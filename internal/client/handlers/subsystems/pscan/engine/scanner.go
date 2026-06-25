//go:build pscan

package engine

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type scanJob struct {
	host     netip.Addr
	port     int
	protocol Protocol
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
	if len(cfg.Protocols) == 0 {
		cfg.Protocols = []Protocol{ProtocolTCP}
	}

	scanCtx := ctx
	cancel := func() {}
	if cfg.MaxDuration > 0 {
		scanCtx, cancel = context.WithTimeout(ctx, cfg.MaxDuration)
	}
	defer cancel()

	limiter := newScanLimiter(cfg.Rate)

	jobs := make(chan scanJob)
	results := make(chan Result)
	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for job := range jobs {
			if limiter != nil {
				if err := limiter.Wait(scanCtx); err != nil {
					return
				}
			}
			result := scanOne(scanCtx, cfg.Timeout, job)
			select {
			case results <- result:
			case <-scanCtx.Done():
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
				for _, protocol := range cfg.Protocols {
					select {
					case jobs <- scanJob{host: host, port: port, protocol: protocol}:
					case <-scanCtx.Done():
						return
					}
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

	if scanCtx.Err() == context.DeadlineExceeded && cfg.MaxDuration > 0 {
		return fmt.Errorf("scan exceeded --max-duration %s", cfg.MaxDuration)
	}
	if scanCtx.Err() == context.DeadlineExceeded {
		return scanCtx.Err()
	}
	if scanCtx.Err() == context.Canceled {
		return ctx.Err()
	}
	return nil
}

func scanOne(ctx context.Context, timeout time.Duration, job scanJob) Result {
	switch job.protocol {
	case ProtocolUDP:
		return scanUDP(ctx, timeout, job)
	default:
		return scanTCP(ctx, timeout, job)
	}
}

func newScanLimiter(ratePerSecond int) *rate.Limiter {
	if ratePerSecond <= 0 {
		return nil
	}
	return rate.NewLimiter(rate.Limit(ratePerSecond), 1)
}

func scanTCP(ctx context.Context, timeout time.Duration, job scanJob) Result {
	start := time.Now()
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(job.host.String(), strconv.Itoa(job.port)))
	duration := time.Since(start)
	result := Result{
		IP:         job.host.String(),
		Port:       job.port,
		Protocol:   ProtocolTCP,
		State:      StateClosed,
		Open:       err == nil,
		DurationMS: duration.Milliseconds(),
	}
	if result.Open {
		result.State = StateOpen
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
		if job.port == 445 {
			if nbInfo, ok := probeNetBIOS(ctx, net.IP(job.host.AsSlice()), timeout); ok {
				result.NetBIOS = &nbInfo
			}
		}
	}
	return result
}

type udpProbe struct {
	Payload          []byte
	NetBIOSTransID   uint16
	ParseNetBIOS     bool
	ExpectAnyPayload bool
}

func scanUDP(ctx context.Context, timeout time.Duration, job scanJob) Result {
	start := time.Now()
	result := Result{
		IP:       job.host.String(),
		Port:     job.port,
		Protocol: ProtocolUDP,
		State:    StateOpenFiltered,
	}

	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "udp", net.JoinHostPort(job.host.String(), strconv.Itoa(job.port)))
	if err != nil {
		result.DurationMS = time.Since(start).Milliseconds()
		result.State = StateClosed
		result.Error = err.Error()
		return result
	}
	defer conn.Close()

	deadline := time.Now().Add(timeout)
	_ = conn.SetDeadline(deadline)

	probe := udpProbeForPort(job.port)
	if _, err := conn.Write(probe.Payload); err != nil {
		result.DurationMS = time.Since(start).Milliseconds()
		result.State = udpStateFromError(err)
		if result.State != StateOpenFiltered {
			result.Error = err.Error()
		}
		return result
	}

	buffer := make([]byte, 1500)
	n, err := conn.Read(buffer)
	result.DurationMS = time.Since(start).Milliseconds()
	if err != nil {
		result.State = udpStateFromError(err)
		if result.State != StateOpenFiltered {
			result.Error = err.Error()
		}
		return result
	}

	result.Open = true
	result.State = StateOpen
	if probe.ParseNetBIOS {
		if nbInfo, ok := parseNBNSNodeStatusResponse(buffer[:n], probe.NetBIOSTransID); ok {
			result.NetBIOS = &nbInfo
		}
	}
	return result
}

func udpStateFromError(err error) string {
	if err == nil {
		return StateOpen
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return StateOpenFiltered
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return StateOpenFiltered
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "connection refused") || strings.Contains(lower, "port unreachable") {
		return StateClosed
	}
	return StateOpenFiltered
}

func udpProbeForPort(port int) udpProbe {
	switch port {
	case 53:
		return udpProbe{Payload: dnsStatusProbe()}
	case 123:
		return udpProbe{Payload: ntpClientProbe()}
	case 137:
		transactionID := uint16(time.Now().UnixNano())
		return udpProbe{
			Payload:        buildNBNSNodeStatusRequest(transactionID),
			NetBIOSTransID: transactionID,
			ParseNetBIOS:   true,
		}
	case 161:
		return udpProbe{Payload: snmpSysDescrProbe()}
	case 1900:
		return udpProbe{Payload: []byte("M-SEARCH * HTTP/1.1\r\nHOST:239.255.255.250:1900\r\nMAN:\"ssdp:discover\"\r\nMX:1\r\nST:ssdp:all\r\n\r\n")}
	default:
		return udpProbe{Payload: []byte{0}}
	}
}

func dnsStatusProbe() []byte {
	return []byte{
		0x70, 0x53, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x00,
		0x01,
	}
}

func ntpClientProbe() []byte {
	packet := make([]byte, 48)
	packet[0] = 0x1b
	return packet
}

func snmpSysDescrProbe() []byte {
	return []byte{
		0x30, 0x26, 0x02, 0x01, 0x01, 0x04, 0x06, 0x70,
		0x75, 0x62, 0x6c, 0x69, 0x63, 0xa0, 0x19, 0x02,
		0x04, 0x70, 0x73, 0x63, 0x6e, 0x02, 0x01, 0x00,
		0x02, 0x01, 0x00, 0x30, 0x0b, 0x30, 0x09, 0x06,
		0x05, 0x2b, 0x06, 0x01, 0x02, 0x01, 0x05, 0x00,
	}
}
