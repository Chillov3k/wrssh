//go:build pscan

package engine

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestScanFindsLocalListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen returned error: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	cfg := Config{
		Hosts:       []netip.Addr{netip.MustParseAddr("127.0.0.1")},
		Ports:       []int{port},
		Timeout:     100 * time.Millisecond,
		Workers:     1,
		MaxDuration: time.Second,
	}

	var results []Result
	if err := Scan(context.Background(), cfg, func(result Result) error {
		results = append(results, result)
		return nil
	}); err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("result count = %d, want 1", len(results))
	}
	if !results[0].Open {
		t.Fatalf("expected open result, got %+v", results[0])
	}
	if results[0].Protocol != ProtocolTCP || results[0].State != StateOpen {
		t.Fatalf("result protocol/state = %s/%s, want tcp/open", results[0].Protocol, results[0].State)
	}
}

func TestScanFindsLocalUDPListener(t *testing.T) {
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket returned error: %v", err)
	}
	defer conn.Close()

	go func() {
		buffer := make([]byte, 256)
		for {
			n, addr, err := conn.ReadFrom(buffer)
			if err != nil {
				return
			}
			if n > 0 {
				_, _ = conn.WriteTo([]byte("ok"), addr)
			}
		}
	}()

	port := conn.LocalAddr().(*net.UDPAddr).Port
	cfg := Config{
		Hosts:       []netip.Addr{netip.MustParseAddr("127.0.0.1")},
		Ports:       []int{port},
		Protocols:   []Protocol{ProtocolUDP},
		Timeout:     time.Second,
		Workers:     1,
		MaxDuration: time.Second,
	}

	var results []Result
	if err := Scan(context.Background(), cfg, func(result Result) error {
		results = append(results, result)
		return nil
	}); err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("result count = %d, want 1", len(results))
	}
	if !results[0].Open || results[0].Protocol != ProtocolUDP || results[0].State != StateOpen {
		t.Fatalf("expected udp/open result, got %+v", results[0])
	}
}

func TestScanCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg := Config{
		Hosts:       []netip.Addr{netip.MustParseAddr("127.0.0.1")},
		Ports:       []int{1},
		Timeout:     100 * time.Millisecond,
		Workers:     1,
		MaxDuration: time.Second,
	}
	err := Scan(ctx, cfg, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Scan error = %v, want context.Canceled", err)
	}
}

func TestScanMaxDuration(t *testing.T) {
	cfg := Config{
		Hosts:       []netip.Addr{netip.MustParseAddr("127.0.0.1")},
		Ports:       []int{1},
		Timeout:     100 * time.Millisecond,
		Workers:     1,
		MaxDuration: time.Millisecond,
	}
	err := Scan(context.Background(), cfg, func(Result) error {
		time.Sleep(5 * time.Millisecond)
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "scan exceeded --max-duration") {
		t.Fatalf("Scan error = %v, want max-duration error", err)
	}
}

func TestScanCollectsWebTitle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Server", "pscan-test")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("<html><head><title> Test Portal </title></head></html>"))
	}))
	defer server.Close()

	addr := server.Listener.Addr().(*net.TCPAddr)

	cfg := Config{
		Hosts:       []netip.Addr{netip.MustParseAddr("127.0.0.1")},
		Ports:       []int{addr.Port},
		Timeout:     time.Second,
		Workers:     1,
		MaxDuration: time.Second,
	}

	var results []Result
	if err := Scan(context.Background(), cfg, func(result Result) error {
		results = append(results, result)
		return nil
	}); err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if len(results) != 1 || results[0].Web == nil {
		t.Fatalf("expected one web result, got %+v", results)
	}
	if results[0].Web.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", results[0].Web.StatusCode, http.StatusAccepted)
	}
	if results[0].Web.Title != "Test Portal" {
		t.Fatalf("title = %q, want Test Portal", results[0].Web.Title)
	}
}

func TestScanCollectsWebTitleOnNonPriorityPort(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<title>Odd Port Web</title>"))
	}))
	defer server.Close()

	addr := server.Listener.Addr().(*net.TCPAddr)
	for _, port := range priorityWebPorts {
		if addr.Port == port {
			t.Skipf("test listener unexpectedly got priority web port %d", addr.Port)
		}
	}

	cfg := Config{
		Hosts:       []netip.Addr{netip.MustParseAddr("127.0.0.1")},
		Ports:       []int{addr.Port},
		Timeout:     time.Second,
		Workers:     1,
		MaxDuration: time.Second,
	}

	var results []Result
	if err := Scan(context.Background(), cfg, func(result Result) error {
		results = append(results, result)
		return nil
	}); err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if len(results) != 1 || results[0].Web == nil {
		t.Fatalf("expected web probe on non-priority port, got %+v", results)
	}
	if results[0].Web.Title != "Odd Port Web" {
		t.Fatalf("title = %q, want Odd Port Web", results[0].Web.Title)
	}
}

func TestScanLimiterUsesSingleBurst(t *testing.T) {
	limiter := newScanLimiter(10)
	if limiter == nil {
		t.Fatalf("expected limiter")
	}
	if got, want := limiter.Burst(), 1; got != want {
		t.Fatalf("limiter burst = %d, want %d", got, want)
	}
}

func TestWebProbeTransportDoesNotUseEnvironmentProxy(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")

	transport := newWebProbeTransport(time.Second)
	if transport.Proxy != nil {
		t.Fatalf("expected direct web probe transport without environment proxy")
	}
}
