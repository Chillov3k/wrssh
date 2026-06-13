package orchestrator

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/NHAS/reverse_ssh/internal/platform/rssh"
	"github.com/NHAS/reverse_ssh/internal/platform/secrets"
	"github.com/NHAS/reverse_ssh/internal/platform/store"
	"github.com/NHAS/reverse_ssh/internal/server/users"
	"github.com/NHAS/reverse_ssh/internal/server/webserver"
	"golang.org/x/net/websocket"
	"gorm.io/gorm"
)

func (m *Manager) HasProjectRuntime(projectName string) (bool, error) {
	if m == nil {
		return false, nil
	}

	_, err := m.store.GetProjectRuntime(projectName)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return false, err
}

func (m *Manager) RuntimeExternalAddress(projectName string) (string, error) {
	if m == nil {
		return "", fmt.Errorf("project runtime manager is disabled")
	}

	runtime, err := m.store.GetProjectRuntime(projectName)
	if err != nil {
		return "", err
	}

	return m.runtimeExternalAddress(runtime.SSHPublishedPort), nil
}

func (m *Manager) ListClientSnapshots(ctx context.Context, projectName string) ([]users.ClientSnapshot, error) {
	response := struct {
		Items []users.ClientSnapshot `json:"items"`
	}{}
	if err := m.runtimeJSON(ctx, projectName, http.MethodGet, "/internal/clients", nil, &response); err != nil {
		return nil, err
	}
	return response.Items, nil
}

func (m *Manager) ListArtifacts(ctx context.Context, projectName string) ([]rssh.Artifact, error) {
	response := struct {
		Items []rssh.Artifact `json:"items"`
	}{}
	if err := m.runtimeJSON(ctx, projectName, http.MethodGet, "/internal/artifacts", nil, &response); err != nil {
		return nil, err
	}
	return response.Items, nil
}

func (m *Manager) GetArtifact(ctx context.Context, projectName, urlPath string) (rssh.Artifact, error) {
	var artifact rssh.Artifact
	err := m.runtimeJSON(ctx, projectName, http.MethodGet, path.Join("/internal/artifacts", url.PathEscape(urlPath)), nil, &artifact)
	return artifact, err
}

func (m *Manager) CreateBuild(ctx context.Context, projectName string, config webserver.BuildConfig) (rssh.BuildResult, error) {
	var result rssh.BuildResult
	err := m.runtimeJSONWithClient(ctx, m.buildHTTPClient, projectName, http.MethodPost, "/internal/artifacts", config, &result)
	return result, err
}

func (m *Manager) DeleteArtifact(ctx context.Context, projectName, urlPath string) error {
	return m.runtimeJSON(ctx, projectName, http.MethodDelete, path.Join("/internal/artifacts", url.PathEscape(urlPath)), nil, nil)
}

func (m *Manager) KillConnection(ctx context.Context, projectName, connectionID string) error {
	return m.runtimeJSON(ctx, projectName, http.MethodPost, path.Join("/internal/connections", url.PathEscape(connectionID), "kill"), map[string]any{}, nil)
}

func (m *Manager) ExecuteCommand(ctx context.Context, projectName, connectionID, command string) (rssh.CommandExecution, error) {
	response := struct {
		Output   string `json:"output"`
		TimedOut bool   `json:"timedOut"`
		Error    string `json:"error"`
	}{}
	err := m.runtimeJSON(ctx, projectName, http.MethodPost, path.Join("/internal/connections", url.PathEscape(connectionID), "exec"), map[string]any{
		"command": command,
	}, &response)
	if err != nil {
		return rssh.CommandExecution{}, err
	}
	if strings.TrimSpace(response.Error) != "" {
		return rssh.CommandExecution{
			Output:   response.Output,
			TimedOut: response.TimedOut,
		}, errors.New(strings.TrimSpace(response.Error))
	}
	return rssh.CommandExecution{
		Output:   response.Output,
		TimedOut: response.TimedOut,
	}, nil
}

func (m *Manager) ListModules(ctx context.Context, projectName, connectionID string) ([]rssh.ModuleManifest, error) {
	response := struct {
		Items []rssh.ModuleManifest `json:"items"`
	}{}
	err := m.runtimeJSON(ctx, projectName, http.MethodGet, path.Join("/internal/connections", url.PathEscape(connectionID), "modules"), nil, &response)
	if err != nil {
		return nil, err
	}
	return response.Items, nil
}

func (m *Manager) RunModule(ctx context.Context, projectName, connectionID, module string, args []string, stdin []byte, opts rssh.SubsystemExecutionOptions) (rssh.SubsystemExecution, error) {
	response := struct {
		Output    string `json:"output"`
		TimedOut  bool   `json:"timedOut"`
		Truncated bool   `json:"truncated"`
		Error     string `json:"error"`
	}{}
	err := m.runtimeJSON(ctx, projectName, http.MethodPost, path.Join("/internal/connections", url.PathEscape(connectionID), "modules", url.PathEscape(module), "run"), map[string]any{
		"args":             args,
		"stdinBase64":      base64.StdEncoding.EncodeToString(stdin),
		"timeoutSeconds":   int(opts.Timeout.Seconds()),
		"outputLimitBytes": opts.OutputLimitBytes,
	}, &response)
	if err != nil {
		return rssh.SubsystemExecution{}, err
	}
	result := rssh.SubsystemExecution{
		Output:    response.Output,
		TimedOut:  response.TimedOut,
		Truncated: response.Truncated,
	}
	if strings.TrimSpace(response.Error) != "" {
		return result, errors.New(strings.TrimSpace(response.Error))
	}
	return result, nil
}

type RuntimeFileDownload struct {
	Body          io.ReadCloser
	Filename      string
	ContentType   string
	ContentLength int64
}

func (m *Manager) ListFiles(ctx context.Context, projectName, connectionID, remotePath string) (rssh.FileList, error) {
	requestPath := runtimeConnectionFilesystemPath(connectionID, "filesystem")
	query := url.Values{}
	query.Set("path", remotePath)

	var listing rssh.FileList
	response, err := m.runtimeRequest(ctx, projectName, m.httpClient, http.MethodGet, requestPath+"?"+query.Encode(), nil, "")
	if err != nil {
		return rssh.FileList{}, err
	}
	defer response.Body.Close()

	if err := json.NewDecoder(response.Body).Decode(&listing); err != nil && !errors.Is(err, io.EOF) {
		return rssh.FileList{}, err
	}
	return listing, nil
}

func (m *Manager) DownloadFile(ctx context.Context, projectName, connectionID, remotePath string) (RuntimeFileDownload, error) {
	query := url.Values{}
	query.Set("path", remotePath)
	response, err := m.runtimeRequest(ctx, projectName, m.fileHTTPClient, http.MethodGet, runtimeConnectionFilesystemPath(connectionID, "filesystem/download")+"?"+query.Encode(), nil, "")
	if err != nil {
		return RuntimeFileDownload{}, err
	}

	filename := path.Base(rssh.CleanRemotePath(remotePath))
	if _, params, parseErr := mime.ParseMediaType(response.Header.Get("Content-Disposition")); parseErr == nil {
		if value := strings.TrimSpace(params["filename"]); value != "" {
			filename = value
		}
	}

	return RuntimeFileDownload{
		Body:          response.Body,
		Filename:      filename,
		ContentType:   response.Header.Get("Content-Type"),
		ContentLength: response.ContentLength,
	}, nil
}

func (m *Manager) PreviewFile(ctx context.Context, projectName, connectionID, remotePath string) (rssh.FilePreview, error) {
	query := url.Values{}
	query.Set("path", remotePath)
	response, err := m.runtimeRequest(ctx, projectName, m.httpClient, http.MethodGet, runtimeConnectionFilesystemPath(connectionID, "filesystem/preview")+"?"+query.Encode(), nil, "")
	if err != nil {
		return rssh.FilePreview{}, err
	}
	defer response.Body.Close()

	var preview rssh.FilePreview
	if err := json.NewDecoder(response.Body).Decode(&preview); err != nil && !errors.Is(err, io.EOF) {
		return rssh.FilePreview{}, err
	}
	return preview, nil
}

func (m *Manager) UploadFile(ctx context.Context, projectName, connectionID, directory, filename string, reader io.Reader) (rssh.FileUploadResult, error) {
	pr, pw := io.Pipe()
	multipartWriter := multipart.NewWriter(pw)

	go func() {
		var err error
		defer func() {
			if err != nil {
				_ = pw.CloseWithError(err)
				return
			}
			if closeErr := multipartWriter.Close(); closeErr != nil {
				_ = pw.CloseWithError(closeErr)
				return
			}
			_ = pw.Close()
		}()

		part, err := multipartWriter.CreateFormFile("file", filename)
		if err != nil {
			return
		}
		_, err = io.Copy(part, reader)
	}()

	query := url.Values{}
	query.Set("directory", directory)
	requestPath := runtimeConnectionFilesystemPath(connectionID, "filesystem/upload") + "?" + query.Encode()
	response, err := m.runtimeRequest(ctx, projectName, m.fileHTTPClient, http.MethodPost, requestPath, pr, multipartWriter.FormDataContentType())
	if err != nil {
		_ = pr.CloseWithError(err)
		return rssh.FileUploadResult{}, err
	}
	defer response.Body.Close()

	var result rssh.FileUploadResult
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil && !errors.Is(err, io.EOF) {
		return rssh.FileUploadResult{}, err
	}
	return result, nil
}

func (m *Manager) DialTerminal(ctx context.Context, projectName, stableID, connectionID string, cols, rows uint32, shell string) (*websocket.Conn, error) {
	if m == nil {
		return nil, fmt.Errorf("project runtime manager is disabled")
	}

	runtime, agentToken, err := m.runtimeCredentials(projectName)
	if err != nil {
		return nil, err
	}

	baseURL, err := url.Parse(runtime.AgentBaseURL)
	if err != nil {
		return nil, err
	}
	switch baseURL.Scheme {
	case "http":
		baseURL.Scheme = "ws"
	case "https":
		baseURL.Scheme = "wss"
	default:
		baseURL.Scheme = "ws"
	}
	baseURL.Path = path.Join("/internal/ws/terminal", stableID)

	query := baseURL.Query()
	if strings.TrimSpace(connectionID) != "" {
		query.Set("connectionId", strings.TrimSpace(connectionID))
	}
	if cols > 0 {
		query.Set("cols", fmt.Sprintf("%d", cols))
	}
	if rows > 0 {
		query.Set("rows", fmt.Sprintf("%d", rows))
	}
	if strings.TrimSpace(shell) != "" {
		query.Set("shell", strings.TrimSpace(shell))
	}
	baseURL.RawQuery = query.Encode()

	config, err := websocket.NewConfig(baseURL.String(), runtime.AgentBaseURL)
	if err != nil {
		return nil, err
	}
	config.Header.Set("Authorization", "Bearer "+agentToken)

	return websocket.DialConfig(config)
}

func (m *Manager) runtimeJSON(ctx context.Context, projectName, method, requestPath string, body any, out any) error {
	return m.runtimeJSONWithClient(ctx, m.httpClient, projectName, method, requestPath, body, out)
}

func (m *Manager) runtimeJSONWithClient(ctx context.Context, client *http.Client, projectName, method, requestPath string, body any, out any) error {
	if m == nil {
		return fmt.Errorf("project runtime manager is disabled")
	}
	if client == nil {
		client = m.httpClient
	}

	runtime, agentToken, err := m.runtimeCredentials(projectName)
	if err != nil {
		return err
	}

	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(runtime.AgentBaseURL, "/")+requestPath, payload)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+agentToken)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode >= 200 && response.StatusCode < 300 {
		if out == nil {
			io.Copy(io.Discard, response.Body)
			return nil
		}
		if err := json.NewDecoder(response.Body).Decode(out); err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		return nil
	}

	message := response.Status
	var payloadError struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payloadError); err == nil && strings.TrimSpace(payloadError.Error) != "" {
		message = strings.TrimSpace(payloadError.Error)
	}

	switch response.StatusCode {
	case http.StatusNotFound:
		return fmt.Errorf("%w: %s", ErrDockerNotFound, message)
	default:
		return fmt.Errorf("runtime API %s %s failed: %s", method, requestPath, message)
	}
}

func (m *Manager) runtimeRequest(ctx context.Context, projectName string, client *http.Client, method, requestPath string, body io.Reader, contentType string) (*http.Response, error) {
	if m == nil {
		return nil, fmt.Errorf("project runtime manager is disabled")
	}
	if client == nil {
		client = m.httpClient
	}

	runtime, agentToken, err := m.runtimeCredentials(projectName)
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(runtime.AgentBaseURL, "/")+requestPath, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+agentToken)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}

	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return response, nil
	}

	defer response.Body.Close()
	message := response.Status
	var payloadError struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payloadError); err == nil && strings.TrimSpace(payloadError.Error) != "" {
		message = strings.TrimSpace(payloadError.Error)
	}

	return nil, fmt.Errorf("runtime API %s %s failed: %s", method, requestPath, message)
}

func runtimeConnectionFilesystemPath(connectionID, suffix string) string {
	return path.Join("/internal/connections", url.PathEscape(connectionID), suffix)
}

func (m *Manager) runtimeCredentials(projectName string) (store.ProjectRuntimeRecord, string, error) {
	runtime, err := m.store.GetProjectRuntime(projectName)
	if err != nil {
		return store.ProjectRuntimeRecord{}, "", err
	}
	secretsRecord, err := m.store.GetProjectRuntimeSecrets(projectName)
	if err != nil {
		return store.ProjectRuntimeRecord{}, "", err
	}
	agentToken, err := secrets.DecryptString(m.cfg.RuntimeSecretKey, secretsRecord.AgentTokenEncrypted)
	if err != nil {
		return store.ProjectRuntimeRecord{}, "", err
	}
	return runtime, agentToken, nil
}
