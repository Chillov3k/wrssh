//go:build pscan

package engine

import (
	"context"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type scanJob struct {
	host netip.Addr
	port int
}

func Scan(ctx context.Context, cfg Config, emit func(Result) error) error {
	if emit == nil {
		emit = func(Result) error { return nil }
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.Workers <= 0 {
		cfg.Workers = DefaultWorkers
	}
	if cfg.MaxDuration <= 0 {
		cfg.MaxDuration = MaxScanDuration
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.MaxDuration)
	defer cancel()

	limiter := newScanLimiter(cfg.Rate)

	jobs := make(chan scanJob)
	results := make(chan Result)
	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for job := range jobs {
			if limiter != nil {
				if err := limiter.Wait(ctx); err != nil {
					return
				}
			}
			result := scanOne(ctx, cfg.Timeout, job)
			select {
			case results <- result:
			case <-ctx.Done():
				return
			}
		}
	}

	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)
		go worker()
	}

	go func() {
		defer close(jobs)
		for _, host := range cfg.Hosts {
			for _, port := range cfg.Ports {
				select {
				case jobs <- scanJob{host: host, port: port}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	for result := range results {
		if err := emit(result); err != nil {
			cancel()
			return err
		}
	}

	if err := ctx.Err(); err != nil && err != context.Canceled {
		return err
	}
	if ctx.Err() == context.Canceled {
		return ctx.Err()
	}
	return nil
}

func newScanLimiter(ratePerSecond int) *rate.Limiter {
	if ratePerSecond <= 0 {
		return nil
	}
	return rate.NewLimiter(rate.Limit(ratePerSecond), 1)
}

func scanOne(ctx context.Context, timeout time.Duration, job scanJob) Result {
	start := time.Now()
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(job.host.String(), strconv.Itoa(job.port)))
	duration := time.Since(start)
	result := Result{
		IP:         job.host.String(),
		Port:       job.port,
		Open:       err == nil,
		DurationMS: duration.Milliseconds(),
	}
	if conn != nil {
		_ = conn.Close()
	}
	if err != nil && ctx.Err() == nil {
		result.Error = err.Error()
	}
	if result.Open {
		if webInfo, ok := probeWeb(ctx, net.IP(job.host.AsSlice()), job.port, timeout); ok {
			result.Web = &webInfo
		}
	}
	return result
}
