//go:build pscan

package engine

type Protocol string

const (
	ProtocolTCP Protocol = "tcp"
	ProtocolUDP Protocol = "udp"

	StateOpen         = "open"
	StateClosed       = "closed"
	StateOpenFiltered = "open|filtered"
)

type Result struct {
	IP         string   `json:"ip"`
	Port       int      `json:"port"`
	Protocol   Protocol `json:"protocol,omitempty"`
	State      string   `json:"state,omitempty"`
	Open       bool     `json:"open"`
	Error      string   `json:"error,omitempty"`
	DurationMS int64    `json:"durationMs"`
	Web        *WebInfo `json:"web,omitempty"`
	NetBIOS    *NBInfo  `json:"netbios,omitempty"`
}
