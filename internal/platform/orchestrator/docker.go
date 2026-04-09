package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrDockerNotFound = errors.New("docker resource not found")
	ErrDockerConflict = errors.New("docker resource conflict")
)

type DockerClient struct {
	httpClient *http.Client
	baseURL    string
	apiVersion string
}

type dockerVersionResponse struct {
	APIVersion string `json:"ApiVersion"`
}

type dockerErrorResponse struct {
	Message string `json:"message"`
}

type containerInspectResponse struct {
	ID    string `json:"Id"`
	Image string `json:"Image"`
	Name  string `json:"Name"`
	State struct {
		Status   string `json:"Status"`
		Running  bool   `json:"Running"`
		ExitCode int    `json:"ExitCode"`
		Error    string `json:"Error"`
		Health   *struct {
			Status string `json:"Status"`
		} `json:"Health"`
	} `json:"State"`
	NetworkSettings struct {
		Networks map[string]struct{} `json:"Networks"`
		Ports    map[string][]struct {
			HostIP   string `json:"HostIp"`
			HostPort string `json:"HostPort"`
		} `json:"Ports"`
	} `json:"NetworkSettings"`
}

type imageInspectResponse struct {
	ID string `json:"Id"`
}

func NewDockerClient(ctx context.Context, socketPath string, timeout time.Duration) (*DockerClient, error) {
	socketPath = strings.TrimSpace(socketPath)
	if socketPath == "" {
		return nil, fmt.Errorf("docker socket path must not be empty")
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("docker API timeout must be greater than zero")
	}

	transport := &http.Transport{
		DisableCompression: true,
		DisableKeepAlives:  true,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}

	client := &DockerClient{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   timeout,
		},
		baseURL: "http://docker",
	}

	var version dockerVersionResponse
	if err := client.do(ctx, http.MethodGet, "/version", nil, &version, http.StatusOK); err != nil {
		return nil, err
	}
	if strings.TrimSpace(version.APIVersion) == "" {
		return nil, fmt.Errorf("docker API version is empty")
	}
	client.apiVersion = version.APIVersion
	return client, nil
}

func (c *DockerClient) EnsureVolume(ctx context.Context, name string, labels map[string]string) error {
	body := map[string]any{
		"Name":   name,
		"Labels": labels,
	}
	err := c.doVersioned(ctx, http.MethodPost, "/volumes/create", body, nil, http.StatusCreated, http.StatusConflict)
	if errors.Is(err, ErrDockerConflict) {
		return nil
	}
	return err
}

func (c *DockerClient) RemoveVolume(ctx context.Context, name string) error {
	path := "/volumes/" + url.PathEscape(name) + "?force=1"
	err := c.doVersioned(ctx, http.MethodDelete, path, nil, nil, http.StatusNoContent)
	if errors.Is(err, ErrDockerNotFound) {
		return nil
	}
	return err
}

func (c *DockerClient) EnsureNetwork(ctx context.Context, name string, internal bool, labels map[string]string) error {
	body := map[string]any{
		"Name":           name,
		"CheckDuplicate": true,
		"Driver":         "bridge",
		"Internal":       internal,
		"Attachable":     false,
		"Labels":         labels,
	}
	err := c.doVersioned(ctx, http.MethodPost, "/networks/create", body, nil, http.StatusCreated, http.StatusConflict)
	if errors.Is(err, ErrDockerConflict) {
		return nil
	}
	return err
}

func (c *DockerClient) RemoveNetwork(ctx context.Context, name string) error {
	path := "/networks/" + url.PathEscape(name)
	err := c.doVersioned(ctx, http.MethodDelete, path, nil, nil, http.StatusNoContent)
	if errors.Is(err, ErrDockerNotFound) {
		return nil
	}
	return err
}

func (c *DockerClient) EnsureNetworkConnected(ctx context.Context, networkName, containerName string, aliases []string) error {
	inspect, err := c.InspectContainer(ctx, containerName)
	if err != nil {
		return err
	}
	if _, ok := inspect.NetworkSettings.Networks[networkName]; ok {
		return nil
	}

	body := map[string]any{
		"Container": containerName,
	}
	if len(aliases) > 0 {
		body["EndpointConfig"] = map[string]any{
			"Aliases": aliases,
		}
	}

	path := "/networks/" + url.PathEscape(networkName) + "/connect"
	err = c.doVersioned(ctx, http.MethodPost, path, body, nil, http.StatusOK)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "already exists") {
		return nil
	}
	return err
}

func (c *DockerClient) DisconnectContainerFromNetwork(ctx context.Context, networkName, containerName string) error {
	inspect, err := c.InspectContainer(ctx, containerName)
	if err != nil {
		if errors.Is(err, ErrDockerNotFound) {
			return nil
		}
		return err
	}
	if _, ok := inspect.NetworkSettings.Networks[networkName]; !ok {
		return nil
	}

	body := map[string]any{
		"Container": containerName,
		"Force":     true,
	}

	path := "/networks/" + url.PathEscape(networkName) + "/disconnect"
	err = c.doVersioned(ctx, http.MethodPost, path, body, nil, http.StatusOK)
	if errors.Is(err, ErrDockerNotFound) {
		return nil
	}
	return err
}

func (c *DockerClient) EnsureContainer(ctx context.Context, name string, body map[string]any) error {
	if _, err := c.InspectContainer(ctx, name); err == nil {
		return nil
	} else if !errors.Is(err, ErrDockerNotFound) {
		return err
	}

	path := "/containers/create?name=" + url.QueryEscape(name)
	err := c.doVersioned(ctx, http.MethodPost, path, body, nil, http.StatusCreated, http.StatusConflict)
	if errors.Is(err, ErrDockerConflict) {
		return nil
	}
	return err
}

func (c *DockerClient) StartContainer(ctx context.Context, name string) error {
	path := "/containers/" + url.PathEscape(name) + "/start"
	err := c.doVersioned(ctx, http.MethodPost, path, nil, nil, http.StatusNoContent, http.StatusNotModified)
	if errors.Is(err, ErrDockerNotFound) {
		return err
	}
	return nil
}

func (c *DockerClient) RemoveContainer(ctx context.Context, name string) error {
	path := "/containers/" + url.PathEscape(name) + "?force=1&v=1"
	err := c.doVersioned(ctx, http.MethodDelete, path, nil, nil, http.StatusNoContent)
	if errors.Is(err, ErrDockerNotFound) {
		return nil
	}
	return err
}

func (c *DockerClient) InspectContainer(ctx context.Context, name string) (containerInspectResponse, error) {
	var response containerInspectResponse
	path := "/containers/" + url.PathEscape(name) + "/json"
	err := c.doVersioned(ctx, http.MethodGet, path, nil, &response, http.StatusOK)
	if err != nil {
		return containerInspectResponse{}, err
	}
	return response, nil
}

func (c *DockerClient) InspectImage(ctx context.Context, ref string) (imageInspectResponse, error) {
	var response imageInspectResponse
	path := "/images/" + url.PathEscape(ref) + "/json"
	err := c.doVersioned(ctx, http.MethodGet, path, nil, &response, http.StatusOK)
	if err != nil {
		return imageInspectResponse{}, err
	}
	return response, nil
}

func (c *DockerClient) doVersioned(ctx context.Context, method, path string, body any, out any, okStatuses ...int) error {
	versionedPath := fmt.Sprintf("/v%s%s", c.apiVersion, path)
	return c.do(ctx, method, versionedPath, body, out, okStatuses...)
}

func (c *DockerClient) do(ctx context.Context, method, path string, body any, out any, okStatuses ...int) error {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, payload)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	for _, code := range okStatuses {
		if resp.StatusCode == code {
			if out == nil {
				io.Copy(io.Discard, resp.Body)
				return nil
			}
			if err := json.NewDecoder(resp.Body).Decode(out); err != nil && !errors.Is(err, io.EOF) {
				return err
			}
			return nil
		}
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	message := strings.TrimSpace(string(bodyBytes))
	if len(bodyBytes) > 0 {
		var dockerErr dockerErrorResponse
		if err := json.Unmarshal(bodyBytes, &dockerErr); err == nil && strings.TrimSpace(dockerErr.Message) != "" {
			message = strings.TrimSpace(dockerErr.Message)
		}
	}

	switch resp.StatusCode {
	case http.StatusNotFound:
		if message == "" {
			message = ErrDockerNotFound.Error()
		}
		return fmt.Errorf("%w: %s", ErrDockerNotFound, message)
	case http.StatusConflict:
		if message == "" {
			message = ErrDockerConflict.Error()
		}
		return fmt.Errorf("%w: %s", ErrDockerConflict, message)
	default:
		if message == "" {
			message = resp.Status
		}
		return fmt.Errorf("docker API %s %s failed: %s", method, path, message)
	}
}
