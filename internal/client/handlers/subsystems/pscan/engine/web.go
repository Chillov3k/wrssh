//go:build pscan

package engine

import (
	"context"
	"crypto/tls"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const maxWebProbeBody = 128 * 1024

var (
	titlePattern     = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	priorityWebPorts = []int{80, 443, 8080, 8443, 81, 7001, 8000, 8001, 8008, 8081, 8088, 8089, 8888, 9000, 9200, 9443, 10000}
)

type WebInfo struct {
	URL        string `json:"url,omitempty"`
	Scheme     string `json:"scheme,omitempty"`
	StatusCode int    `json:"statusCode,omitempty"`
	Title      string `json:"title,omitempty"`
	Server     string `json:"server,omitempty"`
}

func probeWeb(ctx context.Context, host net.IP, port int, timeout time.Duration) (WebInfo, bool) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	for _, scheme := range webProbeSchemes(port) {
		info, ok := probeWebScheme(ctx, host, port, scheme, timeout)
		if ok {
			return info, true
		}
	}
	return WebInfo{}, false
}

func webProbeSchemes(port int) []string {
	switch port {
	case 443, 8443, 9443:
		return []string{"https", "http"}
	default:
		return []string{"http", "https"}
	}
}

func probeWebScheme(ctx context.Context, host net.IP, port int, scheme string, timeout time.Duration) (WebInfo, bool) {
	target := net.JoinHostPort(host.String(), strconv.Itoa(port))
	url := fmt.Sprintf("%s://%s/", scheme, target)

	transport := newWebProbeTransport(timeout)
	defer transport.CloseIdleConnections()

	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return WebInfo{}, false
	}
	req.Header.Set("User-Agent", "wrssh-pscan/1")

	resp, err := client.Do(req)
	if err != nil {
		return WebInfo{}, false
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxWebProbeBody))
	return WebInfo{
		URL:        url,
		Scheme:     scheme,
		StatusCode: resp.StatusCode,
		Title:      extractTitle(string(body)),
		Server:     strings.TrimSpace(resp.Header.Get("Server")),
	}, true
}

func newWebProbeTransport(timeout time.Duration) *http.Transport {
	return &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: timeout,
		}).DialContext,
		TLSHandshakeTimeout: timeout,
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
	}
}

func extractTitle(body string) string {
	match := titlePattern.FindStringSubmatch(body)
	if len(match) < 2 {
		return ""
	}
	title := html.UnescapeString(match[1])
	title = strings.Join(strings.Fields(title), " ")
	if len(title) > 160 {
		title = title[:160]
	}
	return title
}
