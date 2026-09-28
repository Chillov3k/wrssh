// Package networkmap holds the shared network-map data model and the pscan
// output parser used by the control plane and the runtime-agent.
package networkmap

import "strconv"

// Port is a single open transport port observed on a network node.
type Port struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

// Label renders the port in "22/tcp" form.
func (p Port) Label() string {
	protocol := p.Protocol
	if protocol == "" {
		protocol = "tcp"
	}
	return strconv.Itoa(p.Port) + "/" + protocol
}

// Discovery is one IP learned from a pscan run, with everything the scanner
// could observe about it.
type Discovery struct {
	IP       string
	Hostname string
	OS       string
	Ports    []Port
}

// Node is a rendered network-map node after merging stored scan discoveries
// with live captured-host sessions.
type Node struct {
	IP          string  `json:"ip"`
	ExternalIP  string  `json:"externalIp,omitempty"`
	InternalIP  string  `json:"internalIp,omitempty"`
	Hostname    string  `json:"hostname,omitempty"`
	User        string  `json:"user,omitempty"`
	OS          string  `json:"os,omitempty"`
	StableID    string  `json:"stableId,omitempty"`
	Captured    bool    `json:"captured"`
	Online      bool    `json:"online"`
	Source      string  `json:"source"`
	Ports       []Port  `json:"ports,omitempty"`
	FirstSeenAt *string `json:"firstSeenAt,omitempty"`
	LastSeenAt  *string `json:"lastSeenAt,omitempty"`
}

// Edge is a directed "was scanned from" link between two node IPs.
type Edge struct {
	From       string  `json:"from"`
	To         string  `json:"to"`
	LastSeenAt *string `json:"lastSeenAt,omitempty"`
}

// ScanPayload is the runtime-agent request body that records one pscan run.
type ScanPayload struct {
	SourceIP string      `json:"sourceIp"`
	Nodes    []Discovery `json:"nodes"`
}

// ScanResponse reports what a scan upsert changed.
type ScanResponse struct {
	OK    bool `json:"ok"`
	Nodes int  `json:"nodes"`
	Edges int  `json:"edges"`
}

// MapResponse is the stored runtime network map before merging with sessions.
type MapResponse struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
	Notes []Note `json:"notes"`
}

// Note is a free-form operator annotation placed on the map.
type Note struct {
	ID   uint64  `json:"id"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	W    float64 `json:"w"`
	H    float64 `json:"h"`
	Font int     `json:"font"`
	Text string  `json:"text"`
}

// Note limits shared by the platform API and the runtime-agent.
const (
	NoteMinFont      = 10
	NoteMaxFont      = 40
	NoteMinSize      = 80
	NoteMaxSize      = 2000
	NoteMaxTextBytes = 4096
)

// ClampNote sanitizes operator-provided note geometry and text.
func ClampNote(note Note) Note {
	clamp := func(value, min, max float64) float64 {
		if value < min {
			return min
		}
		if value > max {
			return max
		}
		return value
	}
	if note.Font < NoteMinFont {
		note.Font = NoteMinFont
	}
	if note.Font > NoteMaxFont {
		note.Font = NoteMaxFont
	}
	note.X = clamp(note.X, -100000, 100000)
	note.Y = clamp(note.Y, -100000, 100000)
	note.W = clamp(note.W, NoteMinSize, NoteMaxSize)
	note.H = clamp(note.H, NoteMinSize, NoteMaxSize)
	if len(note.Text) > NoteMaxTextBytes {
		note.Text = note.Text[:NoteMaxTextBytes]
	}
	return note
}

const (
	SourceAgent = "agent"
	SourcePscan = "pscan"
)
