package mux

import (
	"net"
	"testing"

	"github.com/NHAS/reverse_ssh/pkg/mux/protocols"
)

func TestDetermineProtocolWebsocketPath(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		want   protocols.Type
	}{
		{name: "websocket endpoint", prefix: "GET /ws HTTP/1.1\r\n", want: protocols.Websockets},
		{name: "websocket endpoint with query", prefix: "GET /ws?id=1 HTTP/1.1\r\n", want: protocols.Websockets},
		{name: "download path starting with ws", prefix: "GET /wsslove HTTP/1.1\r\n", want: protocols.HTTPDownload},
		{name: "download path starting with wsfoo", prefix: "GET /wsfoo HTTP/1.1\r\n", want: protocols.HTTPDownload},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, server := net.Pipe()
			defer client.Close()
			defer server.Close()

			go func() {
				_, _ = client.Write([]byte(tt.prefix[:14]))
			}()

			m := &Multiplexer{}
			conn, got, err := m.determineProtocol(server)
			if err != nil {
				t.Fatalf("determineProtocol returned error: %v", err)
			}
			_ = conn.Close()

			if got != tt.want {
				t.Fatalf("determineProtocol got %q, want %q", got, tt.want)
			}
		})
	}
}
