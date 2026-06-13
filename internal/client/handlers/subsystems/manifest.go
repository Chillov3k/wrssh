package subsystems

type Manifest struct {
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Version     string       `json:"version,omitempty"`
	Usage       string       `json:"usage,omitempty"`
	BuildTags   []string     `json:"buildTags,omitempty"`
	Platforms   []string     `json:"platforms,omitempty"`
	Dangerous   bool         `json:"dangerous,omitempty"`
	Disabled    bool         `json:"disabled,omitempty"`
	Limits      ModuleLimits `json:"limits,omitempty"`
}

type ModuleLimits struct {
	TimeoutSeconds int   `json:"timeoutSeconds,omitempty"`
	OutputBytes    int64 `json:"outputBytes,omitempty"`
	StdinBytes     int64 `json:"stdinBytes,omitempty"`
	MaxArgs        int   `json:"maxArgs,omitempty"`
}
