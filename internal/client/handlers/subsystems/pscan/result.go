//go:build pscan

package pscan

type Result struct {
	IP         string   `json:"ip"`
	Port       int      `json:"port"`
	Open       bool     `json:"open"`
	Error      string   `json:"error,omitempty"`
	DurationMS int64    `json:"durationMs"`
	Web        *WebInfo `json:"web,omitempty"`
}
