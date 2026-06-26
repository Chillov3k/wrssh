package subsystems

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
)

const (
	DefaultModuleTimeout     = 60 * time.Second
	MaxModuleTimeout         = 10 * time.Minute
	DefaultModuleOutputBytes = int64(1024 * 1024)
	DefaultModuleStdinBytes  = int64(1024 * 1024)
	DefaultModuleMaxArgs     = 128
)

type RunPolicy struct {
	Timeout      time.Duration
	OutputBytes  int64
	StdinBytes   int64
	MaxArgs      int
	Disabled     bool
	Dangerous    bool
	NoOutputCap  bool
	NoStdinCap   bool
	NoTimeoutCap bool
}

type AuditEvent struct {
	Module       string
	Args         []string
	StartedAt    time.Time
	FinishedAt   time.Time
	Duration     time.Duration
	Success      bool
	Error        string
	TimedOut     bool
	OutputCapped bool
	Dangerous    bool
}

type AuditHook func(AuditEvent)

var auditHook atomic.Value

func SetAuditHook(h AuditHook) {
	auditHook.Store(h)
}

func audit(event AuditEvent) {
	hook, _ := auditHook.Load().(AuditHook)
	if hook != nil {
		hook(event)
	}
}

func policyFor(manifest Manifest) RunPolicy {
	timeout := DefaultModuleTimeout
	if manifest.Limits.TimeoutSeconds > 0 {
		timeout = time.Duration(manifest.Limits.TimeoutSeconds) * time.Second
	} else if manifest.Limits.TimeoutSeconds < 0 {
		timeout = 0
	}
	if timeout > MaxModuleTimeout {
		timeout = MaxModuleTimeout
	}

	outputBytes := DefaultModuleOutputBytes
	noOutputCap := false
	if manifest.Limits.OutputBytes > 0 {
		outputBytes = manifest.Limits.OutputBytes
	} else if manifest.Limits.OutputBytes < 0 {
		outputBytes = 0
		noOutputCap = true
	}

	stdinBytes := DefaultModuleStdinBytes
	noStdinCap := false
	if manifest.Limits.StdinBytes > 0 {
		stdinBytes = manifest.Limits.StdinBytes
	} else if manifest.Limits.StdinBytes < 0 {
		stdinBytes = 0
		noStdinCap = true
	}

	maxArgs := DefaultModuleMaxArgs
	if manifest.Limits.MaxArgs > 0 {
		maxArgs = manifest.Limits.MaxArgs
	}

	return RunPolicy{
		Timeout:      timeout,
		OutputBytes:  outputBytes,
		StdinBytes:   stdinBytes,
		MaxArgs:      maxArgs,
		Disabled:     manifest.Disabled,
		Dangerous:    manifest.Dangerous,
		NoOutputCap:  noOutputCap,
		NoStdinCap:   noStdinCap,
		NoTimeoutCap: timeout == 0,
	}
}

func enforcePolicy(_ context.Context, manifest Manifest, args []string) (RunPolicy, error) {
	policy := policyFor(manifest)
	if policy.Disabled {
		return policy, fmt.Errorf("module %q is disabled", manifest.Name)
	}
	if policy.MaxArgs >= 0 && len(args) > policy.MaxArgs {
		return policy, fmt.Errorf("module %q received %d arguments, maximum is %d", manifest.Name, len(args), policy.MaxArgs)
	}
	return policy, nil
}
