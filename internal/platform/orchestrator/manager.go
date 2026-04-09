package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/NHAS/reverse_ssh/internal"
	platformconfig "github.com/NHAS/reverse_ssh/internal/platform/config"
	"github.com/NHAS/reverse_ssh/internal/platform/secrets"
	"github.com/NHAS/reverse_ssh/internal/platform/store"
	"gorm.io/gorm"
)

const runtimeBuildAPITimeout = 5 * time.Minute

type Manager struct {
	cfg               platformconfig.Config
	store             *store.Store
	docker            *DockerClient
	httpClient        *http.Client
	buildHTTPClient   *http.Client
	platformContainer string
}

func NewManager(ctx context.Context, cfg platformconfig.Config, appStore *store.Store) (*Manager, error) {
	if !cfg.ProjectRuntimesEnabled {
		return nil, nil
	}

	dockerClient, err := NewDockerClient(ctx, cfg.RuntimeDockerSocket, time.Duration(cfg.RuntimeDockerAPITimeoutSec)*time.Second)
	if err != nil {
		return nil, err
	}

	return &Manager{
		cfg:               cfg,
		store:             appStore,
		docker:            dockerClient,
		platformContainer: currentPlatformContainerName(),
		httpClient: &http.Client{
			Timeout: time.Duration(cfg.RuntimeAPITimeout) * time.Second,
		},
		buildHTTPClient: &http.Client{
			Timeout: runtimeBuildAPITimeout,
		},
	}, nil
}

func currentPlatformContainerName() string {
	if value := strings.TrimSpace(os.Getenv("PLATFORM_CONTAINER_NAME")); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv("HOSTNAME")); value != "" {
		return value
	}
	value, err := os.Hostname()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func (m *Manager) EnsureProjectRuntimes(ctx context.Context) error {
	if m == nil {
		return nil
	}

	projects, err := m.store.ListProjects()
	if err != nil {
		return err
	}

	var failures []string
	for _, project := range projects {
		if _, err := m.ProvisionProject(ctx, project.Name); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", project.Name, err))
			continue
		}
	}

	if len(failures) > 0 {
		return fmt.Errorf("runtime bootstrap failures: %s", strings.Join(failures, "; "))
	}

	return nil
}

func (m *Manager) ProvisionProject(ctx context.Context, projectName string) (store.ProjectRuntimeRecord, error) {
	if m == nil {
		return store.ProjectRuntimeRecord{}, fmt.Errorf("project runtime manager is disabled")
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(m.cfg.RuntimeProvisionSec)*time.Second)
	defer cancel()

	runtime, newlyReserved, err := m.ensureRuntimeReservation(ctx, projectName)
	if err != nil {
		return store.ProjectRuntimeRecord{}, err
	}
	if newlyReserved || runtimeNeedsReset(runtime) {
		log.Printf("[runtime:%s] reset stale resources before provisioning", projectName)
		if err := m.resetRuntimeResources(ctx, runtime); err != nil {
			m.markRuntimeFailure(projectName, err)
			return store.ProjectRuntimeRecord{}, err
		}
	}

	secretsRecord, err := m.store.GetProjectRuntimeSecrets(projectName)
	if err != nil {
		return store.ProjectRuntimeRecord{}, err
	}

	agentToken, err := secrets.DecryptString(m.cfg.RuntimeSecretKey, secretsRecord.AgentTokenEncrypted)
	if err != nil {
		return store.ProjectRuntimeRecord{}, err
	}
	dbPassword, err := secrets.DecryptString(m.cfg.RuntimeSecretKey, secretsRecord.DBPasswordEncrypted)
	if err != nil {
		return store.ProjectRuntimeRecord{}, err
	}

	provisioning := store.RuntimeStatusProvisioning
	clearError := ""
	now := time.Now().UTC()
	if _, err := m.store.UpdateProjectRuntimeStatus(projectName, store.ProjectRuntimeStatusPatch{
		Status:            &provisioning,
		LastError:         &clearError,
		LastProvisionedAt: &now,
	}); err != nil {
		return store.ProjectRuntimeRecord{}, err
	}

	if err := m.provisionProjectAttempt(ctx, projectName, runtime, agentToken, dbPassword); err != nil {
		if shouldRetryProvision(runtime, err) {
			log.Printf("[runtime:%s] initial provisioning attempt failed, resetting resources and retrying: %v", projectName, err)
			if resetErr := m.resetRuntimeResources(ctx, runtime); resetErr != nil {
				err = fmt.Errorf("%w; reset failed: %v", err, resetErr)
			} else if retryErr := m.provisionProjectAttempt(ctx, projectName, runtime, agentToken, dbPassword); retryErr != nil {
				err = retryErr
			} else {
				err = nil
			}
		}
		if err != nil {
			m.markRuntimeFailure(projectName, err)
			return store.ProjectRuntimeRecord{}, err
		}
	}

	log.Printf("[runtime:%s] sync project access", projectName)
	if err := m.SyncProjectAccess(ctx, projectName); err != nil {
		m.markRuntimeFailure(projectName, err)
		return store.ProjectRuntimeRecord{}, err
	}

	running := store.RuntimeStatusRunning
	healthyAt := time.Now().UTC()
	updated, err := m.store.UpdateProjectRuntimeStatus(projectName, store.ProjectRuntimeStatusPatch{
		Status:            &running,
		LastError:         &clearError,
		LastHealthyAt:     &healthyAt,
		LastProvisionedAt: &healthyAt,
	})
	if err != nil {
		return store.ProjectRuntimeRecord{}, err
	}

	return updated, nil
}

func (m *Manager) provisionProjectAttempt(ctx context.Context, projectName string, runtime store.ProjectRuntimeRecord, agentToken, dbPassword string) error {
	if err := m.ensureRuntimeResources(ctx, projectName, runtime, agentToken, dbPassword); err != nil {
		return err
	}

	log.Printf("[runtime:%s] wait database healthy", projectName)
	if err := m.waitDatabaseHealthy(ctx, runtime.DatabaseContainer); err != nil {
		return err
	}

	log.Printf("[runtime:%s] wait agent healthy", projectName)
	if err := m.waitAgentHealthy(ctx, runtime, agentToken); err != nil {
		return err
	}

	return nil
}

func (m *Manager) SyncProjectAccess(ctx context.Context, projectName string) error {
	if m == nil {
		return nil
	}

	runtime, err := m.store.GetProjectRuntime(projectName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	secretsRecord, err := m.store.GetProjectRuntimeSecrets(projectName)
	if err != nil {
		return err
	}
	agentToken, err := secrets.DecryptString(m.cfg.RuntimeSecretKey, secretsRecord.AgentTokenEncrypted)
	if err != nil {
		return err
	}

	users, err := m.store.ListProjectAccessUsers(projectName)
	if err != nil {
		return err
	}

	request := map[string]any{
		"users": users,
	}
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPut, strings.TrimRight(runtime.AgentBaseURL, "/")+"/internal/access/users", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Authorization", "Bearer "+agentToken)

	response, err := m.httpClient.Do(httpRequest)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("runtime access sync failed with status %s", response.Status)
	}

	return nil
}

func (m *Manager) DeprovisionProject(ctx context.Context, projectName string) error {
	if m == nil {
		return nil
	}

	runtime, err := m.store.GetProjectRuntime(projectName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(m.cfg.RuntimeProvisionSec)*time.Second)
	defer cancel()

	deleting := store.RuntimeStatusDeleting
	_, _ = m.store.UpdateProjectRuntimeStatus(projectName, store.ProjectRuntimeStatusPatch{
		Status: &deleting,
	})

	_ = m.docker.RemoveContainer(ctx, runtime.RuntimeContainer)
	_ = m.docker.RemoveContainer(ctx, runtime.DatabaseContainer)
	_ = m.docker.RemoveNetwork(ctx, runtime.DatabaseNetwork)
	_ = m.disconnectPlatformFromRuntimeNetwork(ctx, runtime.ControlPlaneNetwork)
	_ = m.docker.RemoveNetwork(ctx, runtime.ControlPlaneNetwork)
	_ = m.docker.RemoveVolume(ctx, runtime.RuntimeVolume)
	_ = m.docker.RemoveVolume(ctx, runtime.DatabaseVolume)

	return nil
}

func (m *Manager) ensureRuntimeReservation(ctx context.Context, projectName string) (store.ProjectRuntimeRecord, bool, error) {
	runtime, err := m.store.GetProjectRuntime(projectName)
	if err == nil {
		return runtime, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return store.ProjectRuntimeRecord{}, false, err
	}

	agentToken, err := internal.RandomString(24)
	if err != nil {
		return store.ProjectRuntimeRecord{}, false, err
	}
	dbPassword, err := internal.RandomString(24)
	if err != nil {
		return store.ProjectRuntimeRecord{}, false, err
	}

	encryptedAgentToken, err := secrets.EncryptString(m.cfg.RuntimeSecretKey, agentToken)
	if err != nil {
		return store.ProjectRuntimeRecord{}, false, err
	}
	encryptedDBPassword, err := secrets.EncryptString(m.cfg.RuntimeSecretKey, dbPassword)
	if err != nil {
		return store.ProjectRuntimeRecord{}, false, err
	}

	runtime, err = m.store.ReserveProjectRuntime(projectName, store.ProjectRuntimeReservation{
		MinSSHPort:          m.cfg.RuntimeSSHPortStart,
		MaxSSHPort:          m.cfg.RuntimeSSHPortEnd,
		MinAgentPort:        m.cfg.RuntimeAgentPortStart,
		MaxAgentPort:        m.cfg.RuntimeAgentPortEnd,
		AgentListenPort:     m.cfg.RuntimeAgentPort,
		AgentBaseURLHost:    m.cfg.RuntimeAgentHost,
		AgentTokenEncrypted: encryptedAgentToken,
		DBPasswordEncrypted: encryptedDBPassword,
	})
	if err != nil {
		return store.ProjectRuntimeRecord{}, false, err
	}

	return runtime, true, nil
}

func runtimeNeedsReset(runtime store.ProjectRuntimeRecord) bool {
	if runtime.LastHealthyAt != nil {
		return false
	}

	switch runtime.Status {
	case store.RuntimeStatusPending, store.RuntimeStatusProvisioning, store.RuntimeStatusFailed, store.RuntimeStatusStopped:
		return true
	default:
		return false
	}
}

func shouldRetryProvision(runtime store.ProjectRuntimeRecord, err error) bool {
	if err == nil || runtime.LastHealthyAt != nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	switch runtime.Status {
	case store.RuntimeStatusPending, store.RuntimeStatusProvisioning, store.RuntimeStatusFailed, store.RuntimeStatusStopped:
		return true
	default:
		return false
	}
}

func (m *Manager) resetRuntimeResources(ctx context.Context, runtime store.ProjectRuntimeRecord) error {
	if m == nil {
		return nil
	}

	if err := m.docker.RemoveContainer(ctx, runtime.RuntimeContainer); err != nil {
		return err
	}
	if err := m.docker.RemoveContainer(ctx, runtime.DatabaseContainer); err != nil {
		return err
	}
	if err := m.docker.RemoveNetwork(ctx, runtime.DatabaseNetwork); err != nil {
		return err
	}
	if err := m.disconnectPlatformFromRuntimeNetwork(ctx, runtime.ControlPlaneNetwork); err != nil {
		return err
	}
	if err := m.docker.RemoveNetwork(ctx, runtime.ControlPlaneNetwork); err != nil {
		return err
	}
	if err := m.docker.RemoveVolume(ctx, runtime.RuntimeVolume); err != nil {
		return err
	}
	if err := m.docker.RemoveVolume(ctx, runtime.DatabaseVolume); err != nil {
		return err
	}

	return nil
}

func (m *Manager) ensureRuntimeResources(ctx context.Context, projectName string, runtime store.ProjectRuntimeRecord, agentToken, dbPassword string) error {
	labels := runtimeLabels(projectName, runtime)

	log.Printf("[runtime:%s] ensure volume %s", projectName, runtime.RuntimeVolume)
	if err := m.docker.EnsureVolume(ctx, runtime.RuntimeVolume, labels); err != nil {
		return err
	}
	log.Printf("[runtime:%s] ensure volume %s", projectName, runtime.DatabaseVolume)
	if err := m.docker.EnsureVolume(ctx, runtime.DatabaseVolume, labels); err != nil {
		return err
	}
	log.Printf("[runtime:%s] ensure network %s", projectName, runtime.ControlPlaneNetwork)
	if err := m.docker.EnsureNetwork(ctx, runtime.ControlPlaneNetwork, false, labels); err != nil {
		return err
	}
	if err := m.connectPlatformToRuntimeNetwork(ctx, runtime.ControlPlaneNetwork); err != nil {
		return err
	}
	log.Printf("[runtime:%s] ensure network %s", projectName, runtime.DatabaseNetwork)
	if err := m.docker.EnsureNetwork(ctx, runtime.DatabaseNetwork, true, labels); err != nil {
		return err
	}

	log.Printf("[runtime:%s] ensure db container %s", projectName, runtime.DatabaseContainer)
	if err := m.ensureContainerImageCurrent(ctx, runtime.DatabaseContainer, m.cfg.RuntimePostgresImage); err != nil {
		return err
	}
	if err := m.docker.EnsureContainer(ctx, runtime.DatabaseContainer, map[string]any{
		"Image": m.cfg.RuntimePostgresImage,
		"Env": []string{
			"POSTGRES_DB=" + runtime.DatabaseName,
			"POSTGRES_USER=" + runtime.DatabaseUser,
			"POSTGRES_PASSWORD=" + dbPassword,
		},
		"Labels": labels,
		"Healthcheck": map[string]any{
			"Test":     []string{"CMD-SHELL", "pg_isready -U \"$POSTGRES_USER\" -d \"$POSTGRES_DB\""},
			"Interval": int64(5 * time.Second),
			"Timeout":  int64(5 * time.Second),
			"Retries":  20,
		},
		"HostConfig": map[string]any{
			"RestartPolicy": map[string]any{"Name": "unless-stopped"},
			"Mounts": []map[string]any{
				{
					"Type":   "volume",
					"Source": runtime.DatabaseVolume,
					"Target": "/var/lib/postgresql/data",
				},
			},
			"SecurityOpt": []string{"no-new-privileges:true"},
		},
		"NetworkingConfig": map[string]any{
			"EndpointsConfig": map[string]any{
				runtime.DatabaseNetwork: map[string]any{
					"Aliases": []string{m.cfg.RuntimeDBHostAlias},
				},
			},
		},
	}); err != nil {
		return err
	}
	log.Printf("[runtime:%s] start db container %s", projectName, runtime.DatabaseContainer)
	if err := m.docker.StartContainer(ctx, runtime.DatabaseContainer); err != nil {
		return err
	}

	runtimeEnv := []string{
		"RSSH_DATA_DIR=/data",
		"RSSH_DB_DRIVER=" + m.cfg.DBDriver,
		"RSSH_DB_HOST=" + m.cfg.RuntimeDBHostAlias,
		"RSSH_DB_PORT=" + m.cfg.RuntimeDBPort,
		"RSSH_DB_NAME=" + runtime.DatabaseName,
		"RSSH_DB_USER=" + runtime.DatabaseUser,
		"RSSH_DB_PASSWORD=" + dbPassword,
		"RSSH_DB_SSLMODE=" + m.cfg.RuntimeDBSSLMode,
		"RSSH_DB_TIMEZONE=" + m.cfg.DBTimeZone,
		"RSSH_LISTEN_ADDR=" + m.cfg.RSSHListenAddr,
		"RSSH_EXTERNAL_ADDRESS=" + m.runtimeExternalAddress(runtime.SSHPublishedPort),
		"RSSH_ENABLE_CLIENT_DOWNLOADS=" + strconv.FormatBool(m.cfg.RSSHEnableLinks),
		"RSSH_TIMEOUT=" + strconv.Itoa(m.cfg.RSSHTimeout),
		"RSSH_INSECURE=" + strconv.FormatBool(m.cfg.RSSHInsecure),
		"RSSH_OPEN_PROXY=" + strconv.FormatBool(m.cfg.RSSHOpenProxy),
		"RUNTIME_AGENT_ADDR=:" + strconv.Itoa(runtime.AgentListenPort),
		"RUNTIME_AGENT_TOKEN=" + agentToken,
		"RUNTIME_PROJECT_NAME=" + projectName,
	}
	if strings.TrimSpace(m.cfg.SeedAuthorizedKeys) != "" {
		runtimeEnv = append(runtimeEnv, "SEED_AUTHORIZED_KEYS="+m.cfg.SeedAuthorizedKeys)
	}
	if m.cfg.RSSHTLS {
		runtimeEnv = append(runtimeEnv, "RSSH_TLS=true")
	}
	if strings.TrimSpace(m.cfg.RSSHTLSCertPath) != "" {
		runtimeEnv = append(runtimeEnv, "RSSH_TLS_CERT_PATH="+m.cfg.RSSHTLSCertPath)
	}
	if strings.TrimSpace(m.cfg.RSSHTLSKeyPath) != "" {
		runtimeEnv = append(runtimeEnv, "RSSH_TLS_KEY_PATH="+m.cfg.RSSHTLSKeyPath)
	}

	log.Printf("[runtime:%s] ensure runtime container %s", projectName, runtime.RuntimeContainer)
	if err := m.ensureContainerImageCurrent(ctx, runtime.RuntimeContainer, m.cfg.RuntimeImage); err != nil {
		return err
	}
	if err := m.ensureRuntimeContainerConfigCurrent(ctx, runtime); err != nil {
		return err
	}
	if err := m.docker.EnsureContainer(ctx, runtime.RuntimeContainer, map[string]any{
		"Image":  m.cfg.RuntimeImage,
		"Env":    runtimeEnv,
		"Labels": labels,
		"ExposedPorts": map[string]any{
			fmt.Sprintf("%d/tcp", m.cfg.RSSHListenPort):    map[string]any{},
			strconv.Itoa(runtime.AgentListenPort) + "/tcp": map[string]any{},
		},
		"HostConfig": map[string]any{
			"RestartPolicy": map[string]any{"Name": "unless-stopped"},
			"Mounts": []map[string]any{
				{
					"Type":   "volume",
					"Source": runtime.RuntimeVolume,
					"Target": "/data",
				},
			},
			"PortBindings": map[string]any{
				fmt.Sprintf("%d/tcp", m.cfg.RSSHListenPort): []map[string]string{{
					"HostIP":   m.cfg.RuntimeBindIP,
					"HostPort": strconv.Itoa(runtime.SSHPublishedPort),
				}},
				strconv.Itoa(runtime.AgentListenPort) + "/tcp": []map[string]string{{
					"HostIP":   m.cfg.RuntimeBindIP,
					"HostPort": strconv.Itoa(runtime.AgentPublishedPort),
				}},
			},
			"SecurityOpt": []string{"no-new-privileges:true"},
			"CapDrop":     []string{"ALL"},
		},
		"NetworkingConfig": map[string]any{
			"EndpointsConfig": map[string]any{
				runtime.ControlPlaneNetwork: map[string]any{
					"Aliases": []string{runtime.RuntimeContainer},
				},
				runtime.DatabaseNetwork: map[string]any{
					"Aliases": []string{runtime.RuntimeContainer},
				},
			},
		},
	}); err != nil {
		return err
	}

	log.Printf("[runtime:%s] start runtime container %s", projectName, runtime.RuntimeContainer)
	return m.docker.StartContainer(ctx, runtime.RuntimeContainer)
}

func (m *Manager) ensureRuntimeContainerConfigCurrent(ctx context.Context, runtime store.ProjectRuntimeRecord) error {
	if m == nil {
		return nil
	}

	inspect, err := m.docker.InspectContainer(ctx, runtime.RuntimeContainer)
	if err != nil {
		if errors.Is(err, ErrDockerNotFound) {
			return nil
		}
		return err
	}

	sshPortKey := fmt.Sprintf("%d/tcp", m.cfg.RSSHListenPort)
	if !hasPublishedPortBinding(inspect, sshPortKey, m.cfg.RuntimeBindIP, runtime.SSHPublishedPort) {
		log.Printf("[runtime] recreate %s because SSH port binding drift detected", runtime.RuntimeContainer)
		return m.docker.RemoveContainer(ctx, runtime.RuntimeContainer)
	}

	agentPortKey := fmt.Sprintf("%d/tcp", runtime.AgentListenPort)
	if !hasPublishedPortBinding(inspect, agentPortKey, m.cfg.RuntimeBindIP, runtime.AgentPublishedPort) {
		log.Printf("[runtime] recreate %s because runtime-agent port binding drift detected", runtime.RuntimeContainer)
		return m.docker.RemoveContainer(ctx, runtime.RuntimeContainer)
	}

	return nil
}

func hasPublishedPortBinding(inspect containerInspectResponse, portKey, bindIP string, hostPort int) bool {
	if hostPort <= 0 {
		return false
	}

	bindings, ok := inspect.NetworkSettings.Ports[portKey]
	if !ok || len(bindings) == 0 {
		return false
	}

	expectedPort := strconv.Itoa(hostPort)
	expectedIP := strings.TrimSpace(bindIP)
	for _, binding := range bindings {
		if strings.TrimSpace(binding.HostPort) != expectedPort {
			continue
		}
		actualIP := strings.TrimSpace(binding.HostIP)
		if expectedIP == "" || expectedIP == actualIP {
			return true
		}
		if expectedIP == "0.0.0.0" && actualIP == "" {
			return true
		}
	}

	return false
}

func (m *Manager) connectPlatformToRuntimeNetwork(ctx context.Context, networkName string) error {
	if m == nil || m.platformContainer == "" || strings.TrimSpace(networkName) == "" {
		return nil
	}
	return m.docker.EnsureNetworkConnected(ctx, networkName, m.platformContainer, nil)
}

func (m *Manager) disconnectPlatformFromRuntimeNetwork(ctx context.Context, networkName string) error {
	if m == nil || m.platformContainer == "" || strings.TrimSpace(networkName) == "" {
		return nil
	}
	return m.docker.DisconnectContainerFromNetwork(ctx, networkName, m.platformContainer)
}

func (m *Manager) ensureContainerImageCurrent(ctx context.Context, containerName, imageRef string) error {
	imageRef = strings.TrimSpace(imageRef)
	if m == nil || imageRef == "" {
		return nil
	}

	container, err := m.docker.InspectContainer(ctx, containerName)
	if err != nil {
		if errors.Is(err, ErrDockerNotFound) {
			return nil
		}
		return err
	}

	image, err := m.docker.InspectImage(ctx, imageRef)
	if err != nil {
		if errors.Is(err, ErrDockerNotFound) {
			return nil
		}
		return err
	}

	if strings.TrimSpace(container.Image) == "" || strings.EqualFold(strings.TrimSpace(container.Image), strings.TrimSpace(image.ID)) {
		return nil
	}

	log.Printf("[runtime] recreate %s because image drift detected (%s -> %s)", containerName, container.Image, image.ID)
	return m.docker.RemoveContainer(ctx, containerName)
}

func (m *Manager) waitDatabaseHealthy(ctx context.Context, container string) error {
	ticker := time.NewTicker(time.Duration(m.cfg.RuntimeHealthPollSec) * time.Second)
	defer ticker.Stop()
	var runningSince time.Time

	for {
		inspect, err := m.docker.InspectContainer(ctx, container)
		if err != nil {
			return err
		}
		if inspect.State.Health != nil && inspect.State.Health.Status == "healthy" {
			return nil
		}
		if !inspect.State.Running && strings.TrimSpace(inspect.State.Status) != "" && strings.TrimSpace(inspect.State.Status) != "created" {
			message := strings.TrimSpace(inspect.State.Error)
			if message == "" {
				message = "container exited before becoming healthy"
			}
			return fmt.Errorf("runtime database %s is %s (exit code %d): %s", container, inspect.State.Status, inspect.State.ExitCode, message)
		}
		if inspect.State.Running {
			if runningSince.IsZero() {
				runningSince = time.Now()
			}
		} else {
			runningSince = time.Time{}
		}
		if inspect.State.Health == nil && inspect.State.Running {
			return nil
		}
		// Some Docker/healthcheck combinations keep Postgres in "starting"/"unhealthy"
		// even though the container is already running and the runtime agent can handle
		// a short DB warm-up with its own retry loop.
		if inspect.State.Running && !runningSince.IsZero() && time.Since(runningSince) >= time.Duration(m.cfg.RuntimeDBGraceSec)*time.Second {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for runtime database %s to become healthy", container)
		case <-ticker.C:
		}
	}
}

func (m *Manager) waitAgentHealthy(ctx context.Context, runtime store.ProjectRuntimeRecord, agentToken string) error {
	ticker := time.NewTicker(time.Duration(m.cfg.RuntimeHealthPollSec) * time.Second)
	defer ticker.Stop()

	healthURL := strings.TrimRight(runtime.AgentBaseURL, "/") + "/internal/health"
	for {
		inspect, inspectErr := m.docker.InspectContainer(ctx, runtime.RuntimeContainer)
		if inspectErr != nil {
			return inspectErr
		}
		if !inspect.State.Running && strings.TrimSpace(inspect.State.Status) != "" && strings.TrimSpace(inspect.State.Status) != "created" {
			message := strings.TrimSpace(inspect.State.Error)
			if message == "" {
				message = "container exited before runtime agent became healthy"
			}
			return fmt.Errorf("runtime container %s is %s (exit code %d): %s", runtime.RuntimeContainer, inspect.State.Status, inspect.State.ExitCode, message)
		}

		request, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if err != nil {
			return err
		}
		request.Header.Set("Authorization", "Bearer "+agentToken)

		response, err := m.httpClient.Do(request)
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for runtime agent %s to become healthy", runtime.RuntimeContainer)
		case <-ticker.C:
		}
	}
}

func (m *Manager) runtimeExternalAddress(sshPort int) string {
	return fmt.Sprintf("%s:%d", strings.TrimSpace(m.cfg.RuntimePublishedHost), sshPort)
}

func (m *Manager) markRuntimeFailure(projectName string, cause error) {
	if m == nil || cause == nil {
		return
	}

	failed := store.RuntimeStatusFailed
	message := cause.Error()
	_, _ = m.store.UpdateProjectRuntimeStatus(projectName, store.ProjectRuntimeStatusPatch{
		Status:    &failed,
		LastError: &message,
	})
}

func runtimeLabels(projectName string, runtime store.ProjectRuntimeRecord) map[string]string {
	return map[string]string{
		"wrssh.managed":      "true",
		"wrssh.project":      projectName,
		"wrssh.project_id":   strconv.FormatUint(uint64(runtime.ProjectID), 10),
		"wrssh.runtime_slug": runtime.RuntimeSlug,
	}
}
