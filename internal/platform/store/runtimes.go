package store

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	RuntimeStatusPending      = "pending"
	RuntimeStatusProvisioning = "provisioning"
	RuntimeStatusRunning      = "running"
	RuntimeStatusDegraded     = "degraded"
	RuntimeStatusStopping     = "stopping"
	RuntimeStatusStopped      = "stopped"
	RuntimeStatusDeleting     = "deleting"
	RuntimeStatusDeleted      = "deleted"
	RuntimeStatusFailed       = "failed"
)

var (
	ErrProjectRuntimeExists   = errors.New("project runtime already exists")
	ErrNoAvailableRuntimePort = errors.New("no available runtime SSH port")
)

type ProjectRuntimeRecord struct {
	ID                  uint       `json:"id"`
	CreatedAt           time.Time  `json:"createdAt"`
	UpdatedAt           time.Time  `json:"updatedAt"`
	ProjectID           uint       `gorm:"uniqueIndex" json:"projectId"`
	RuntimeSlug         string     `gorm:"uniqueIndex;size:96" json:"runtimeSlug"`
	Mode                string     `gorm:"size:32;index" json:"mode"`
	DesiredState        string     `gorm:"size:32;index" json:"desiredState"`
	Status              string     `gorm:"size:32;index" json:"status"`
	SSHPublishedPort    int        `gorm:"uniqueIndex" json:"sshPublishedPort"`
	AgentListenPort     int        `json:"agentListenPort"`
	AgentPublishedPort  int        `gorm:"uniqueIndex" json:"agentPublishedPort"`
	AgentBaseURL        string     `gorm:"size:255" json:"agentBaseUrl"`
	RuntimeContainer    string     `gorm:"uniqueIndex;size:255" json:"runtimeContainer"`
	DatabaseContainer   string     `gorm:"uniqueIndex;size:255" json:"databaseContainer"`
	ControlPlaneNetwork string     `gorm:"uniqueIndex;size:255" json:"controlPlaneNetwork"`
	DatabaseNetwork     string     `gorm:"uniqueIndex;size:255" json:"databaseNetwork"`
	RuntimeVolume       string     `gorm:"size:255" json:"runtimeVolume"`
	DatabaseVolume      string     `gorm:"size:255" json:"databaseVolume"`
	DatabaseName        string     `gorm:"size:128" json:"databaseName"`
	DatabaseUser        string     `gorm:"size:128" json:"databaseUser"`
	LastError           string     `gorm:"size:2048" json:"lastError"`
	LastProvisionedAt   *time.Time `json:"lastProvisionedAt"`
	LastHealthyAt       *time.Time `json:"lastHealthyAt"`
}

type ProjectRuntimeSecretRecord struct {
	ID                  uint      `json:"id"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
	ProjectRuntimeID    uint      `gorm:"uniqueIndex" json:"projectRuntimeId"`
	AgentTokenEncrypted string    `gorm:"size:4096" json:"-"`
	DBPasswordEncrypted string    `gorm:"size:4096" json:"-"`
}

type ProjectRuntimeReservation struct {
	MinSSHPort          int
	MaxSSHPort          int
	MinAgentPort        int
	MaxAgentPort        int
	AgentListenPort     int
	AgentBaseURLHost    string
	AgentTokenEncrypted string
	DBPasswordEncrypted string
}

type ProjectRuntimeStatusPatch struct {
	DesiredState      *string
	Status            *string
	AgentBaseURL      *string
	LastError         *string
	LastProvisionedAt *time.Time
	LastHealthyAt     *time.Time
}

func (s *Store) ReserveProjectRuntime(projectName string, reservation ProjectRuntimeReservation) (ProjectRuntimeRecord, error) {
	project, err := s.GetProject(projectName)
	if err != nil {
		return ProjectRuntimeRecord{}, err
	}

	if strings.TrimSpace(reservation.AgentTokenEncrypted) == "" || strings.TrimSpace(reservation.DBPasswordEncrypted) == "" {
		return ProjectRuntimeRecord{}, errors.New("runtime secrets must be encrypted before storing")
	}
	if reservation.AgentListenPort <= 0 {
		return ProjectRuntimeRecord{}, errors.New("runtime agent listen port must be configured")
	}
	if reservation.MinAgentPort <= 0 || reservation.MaxAgentPort <= 0 || reservation.MinAgentPort > reservation.MaxAgentPort {
		return ProjectRuntimeRecord{}, errors.New("invalid runtime agent port range")
	}
	if reservation.MinSSHPort <= 0 || reservation.MaxSSHPort <= 0 || reservation.MinSSHPort > reservation.MaxSSHPort {
		return ProjectRuntimeRecord{}, errors.New("invalid runtime SSH port range")
	}
	agentBaseHost := strings.TrimSpace(reservation.AgentBaseURLHost)
	if agentBaseHost == "" {
		return ProjectRuntimeRecord{}, errors.New("runtime agent base host must be configured")
	}
	var runtime ProjectRuntimeRecord
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var existing ProjectRuntimeRecord
		result := tx.Where("project_id = ?", project.ID).Limit(1).Find(&existing)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected > 0 {
			return ErrProjectRuntimeExists
		}

		port, err := nextAvailableRuntimePortTx(tx, "ssh_published_port", reservation.MinSSHPort, reservation.MaxSSHPort)
		if err != nil {
			return err
		}
		agentPort, err := nextAvailableRuntimePortTx(tx, "agent_published_port", reservation.MinAgentPort, reservation.MaxAgentPort)
		if err != nil {
			return err
		}

		slug := makeRuntimeSlug(project.ID, project.Name)
		runtimeContainer := runtimeContainerName(slug)
		runtime = ProjectRuntimeRecord{
			ProjectID:           project.ID,
			RuntimeSlug:         slug,
			Mode:                "docker",
			DesiredState:        RuntimeStatusRunning,
			Status:              RuntimeStatusPending,
			SSHPublishedPort:    port,
			AgentListenPort:     reservation.AgentListenPort,
			AgentPublishedPort:  agentPort,
			AgentBaseURL:        fmt.Sprintf("http://%s:%d", agentBaseHost, agentPort),
			RuntimeContainer:    runtimeContainer,
			DatabaseContainer:   runtimeDBContainerName(slug),
			ControlPlaneNetwork: runtimeControlPlaneNetworkName(slug),
			DatabaseNetwork:     runtimeDatabaseNetworkName(slug),
			RuntimeVolume:       runtimeDataVolumeName(slug),
			DatabaseVolume:      runtimeDBVolumeName(slug),
			DatabaseName:        runtimeDatabaseName(project.ID),
			DatabaseUser:        runtimeDatabaseUser(project.ID),
		}
		if err := tx.Create(&runtime).Error; err != nil {
			return err
		}

		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "project_runtime_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"agent_token_encrypted", "db_password_encrypted", "updated_at"}),
		}).Create(&ProjectRuntimeSecretRecord{
			ProjectRuntimeID:    runtime.ID,
			AgentTokenEncrypted: strings.TrimSpace(reservation.AgentTokenEncrypted),
			DBPasswordEncrypted: strings.TrimSpace(reservation.DBPasswordEncrypted),
		}).Error
	})
	if err != nil {
		return ProjectRuntimeRecord{}, err
	}

	return runtime, nil
}

func (s *Store) GetProjectRuntime(projectName string) (ProjectRuntimeRecord, error) {
	project, err := s.GetProject(projectName)
	if err != nil {
		return ProjectRuntimeRecord{}, err
	}

	var runtime ProjectRuntimeRecord
	err = s.db.Where("project_id = ?", project.ID).First(&runtime).Error
	return runtime, err
}

func (s *Store) GetProjectRuntimeSecrets(projectName string) (ProjectRuntimeSecretRecord, error) {
	runtime, err := s.GetProjectRuntime(projectName)
	if err != nil {
		return ProjectRuntimeSecretRecord{}, err
	}

	var record ProjectRuntimeSecretRecord
	err = s.db.Where("project_runtime_id = ?", runtime.ID).First(&record).Error
	return record, err
}

func (s *Store) ListProjectRuntimes() ([]ProjectRuntimeRecord, error) {
	var runtimes []ProjectRuntimeRecord
	err := s.db.Order("ssh_published_port asc").Find(&runtimes).Error
	return runtimes, err
}

func (s *Store) UpdateProjectRuntimeStatus(projectName string, patch ProjectRuntimeStatusPatch) (ProjectRuntimeRecord, error) {
	runtime, err := s.GetProjectRuntime(projectName)
	if err != nil {
		return ProjectRuntimeRecord{}, err
	}

	updates := map[string]any{}
	if patch.DesiredState != nil {
		updates["desired_state"] = normalizeRuntimeState(*patch.DesiredState)
	}
	if patch.Status != nil {
		updates["status"] = normalizeRuntimeState(*patch.Status)
	}
	if patch.AgentBaseURL != nil {
		updates["agent_base_url"] = strings.TrimSpace(*patch.AgentBaseURL)
	}
	if patch.LastError != nil {
		updates["last_error"] = strings.TrimSpace(*patch.LastError)
	}
	if patch.LastProvisionedAt != nil {
		updates["last_provisioned_at"] = *patch.LastProvisionedAt
	}
	if patch.LastHealthyAt != nil {
		updates["last_healthy_at"] = *patch.LastHealthyAt
	}
	if len(updates) == 0 {
		return runtime, nil
	}

	if err := s.db.Model(&ProjectRuntimeRecord{}).Where("id = ?", runtime.ID).Updates(updates).Error; err != nil {
		return ProjectRuntimeRecord{}, err
	}

	return s.GetProjectRuntime(projectName)
}

func (s *Store) DeleteProjectRuntime(projectName string) error {
	runtime, err := s.GetProjectRuntime(projectName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		return deleteProjectRuntimeByProjectIDTx(tx, runtime.ProjectID)
	})
}

func (s *Store) deleteProjectRuntimeByProjectID(projectID uint) error {
	if projectID == 0 {
		return nil
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		return deleteProjectRuntimeByProjectIDTx(tx, projectID)
	})
}

func deleteProjectRuntimeByProjectIDTx(tx *gorm.DB, projectID uint) error {
	if projectID == 0 {
		return nil
	}

	var runtime ProjectRuntimeRecord
	result := tx.Where("project_id = ?", projectID).Limit(1).Find(&runtime)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return nil
	}

	if err := tx.Where("project_runtime_id = ?", runtime.ID).Delete(&ProjectRuntimeSecretRecord{}).Error; err != nil {
		return err
	}

	return tx.Where("id = ?", runtime.ID).Delete(&ProjectRuntimeRecord{}).Error
}

func nextAvailableRuntimePortTx(tx *gorm.DB, column string, minPort, maxPort int) (int, error) {
	if strings.TrimSpace(column) == "" {
		column = "ssh_published_port"
	}

	var ports []int
	if err := tx.Model(&ProjectRuntimeRecord{}).
		Where(fmt.Sprintf("%s >= ? AND %s <= ?", column, column), minPort, maxPort).
		Order(column+" asc").
		Pluck(column, &ports).Error; err != nil {
		return 0, err
	}

	sort.Ints(ports)
	next := minPort
	for _, port := range ports {
		if port < next {
			continue
		}
		if port == next {
			next++
			continue
		}
		break
	}

	if next > maxPort {
		return 0, ErrNoAvailableRuntimePort
	}

	return next, nil
}

func normalizeRuntimeState(value string) string {
	state := strings.ToLower(strings.TrimSpace(value))
	switch state {
	case RuntimeStatusPending,
		RuntimeStatusProvisioning,
		RuntimeStatusRunning,
		RuntimeStatusDegraded,
		RuntimeStatusStopping,
		RuntimeStatusStopped,
		RuntimeStatusDeleting,
		RuntimeStatusDeleted,
		RuntimeStatusFailed:
		return state
	default:
		return RuntimeStatusPending
	}
}

func makeRuntimeSlug(projectID uint, projectName string) string {
	base := strings.ToLower(strings.TrimSpace(projectName))
	if base == "" {
		base = "runtime"
	}

	base = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-':
			return r
		default:
			return '-'
		}
	}, base)

	base = strings.Trim(base, "-")
	for strings.Contains(base, "--") {
		base = strings.ReplaceAll(base, "--", "-")
	}
	if base == "" {
		base = "runtime"
	}
	if len(base) > 48 {
		base = strings.Trim(base[:48], "-")
	}

	return fmt.Sprintf("p%d-%s", projectID, base)
}

func runtimeContainerName(slug string) string {
	return "wrssh-runtime-" + slug
}

func runtimeDBContainerName(slug string) string {
	return "wrssh-runtime-db-" + slug
}

func runtimeControlPlaneNetworkName(slug string) string {
	return "wrssh-rt-cp-" + slug
}

func runtimeDatabaseNetworkName(slug string) string {
	return "wrssh-rt-db-" + slug
}

func runtimeDataVolumeName(slug string) string {
	return "wrssh-rt-data-" + slug
}

func runtimeDBVolumeName(slug string) string {
	return "wrssh-rt-dbdata-" + slug
}

func runtimeDatabaseName(projectID uint) string {
	return fmt.Sprintf("rssh_rt_p%d", projectID)
}

func runtimeDatabaseUser(projectID uint) string {
	return fmt.Sprintf("rssh_rt_u_p%d", projectID)
}
