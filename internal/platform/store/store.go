package store

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/NHAS/reverse_ssh/internal/server/observers"
	"github.com/NHAS/reverse_ssh/internal/server/users"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrInvalidCredentials = errors.New("invalid credentials")

type Store struct {
	db *gorm.DB
}

type WebUser struct {
	ID                 uint      `json:"id"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
	Username           string    `gorm:"uniqueIndex;size:128" json:"username"`
	PasswordHash       string    `json:"-"`
	SessionVersion     uint64    `gorm:"default:0" json:"-"`
	MustChangePassword bool      `json:"mustChangePassword"`
	Role               string    `gorm:"size:32" json:"role"`
	RSSHUsername       string    `gorm:"column:r_ssh_username;size:128" json:"rsshUsername"`
	Enabled            bool      `json:"enabled"`
	CanCreateProjects  bool      `json:"canCreateProjects"`
	AllowedProjects    string    `gorm:"size:4096" json:"-"`
	SSHAuthorizedKeys  string    `gorm:"size:65535" json:"-"`
}

type HostRecord struct {
	ID                   uint       `json:"id"`
	CreatedAt            time.Time  `json:"createdAt"`
	UpdatedAt            time.Time  `json:"updatedAt"`
	StableID             string     `gorm:"uniqueIndex;size:128" json:"stableId"`
	PublicKeyFingerprint string     `gorm:"size:128;index" json:"publicKeyFingerprint"`
	ActiveConnectionID   string     `gorm:"index;size:128" json:"activeConnectionId"`
	LastConnectionID     string     `gorm:"size:128" json:"lastConnectionId"`
	DisplayName          string     `gorm:"size:255" json:"displayName"`
	Hostname             string     `gorm:"size:255" json:"hostname"`
	RemoteAddr           string     `gorm:"size:255" json:"remoteAddr"`
	RemoteIP             string     `gorm:"size:128" json:"remoteIp"`
	Comment              string     `gorm:"size:255" json:"comment"`
	Version              string     `gorm:"size:255" json:"version"`
	Owners               string     `gorm:"size:255" json:"owners"`
	IsPublic             bool       `json:"isPublic"`
	Connected            bool       `json:"connected"`
	FirstSeenAt          *time.Time `json:"firstSeenAt"`
	LastSeenAt           *time.Time `json:"lastSeenAt"`
	LastConnectedAt      *time.Time `json:"lastConnectedAt"`
	LastDisconnectedAt   *time.Time `json:"lastDisconnectedAt"`
	LastActivityAt       *time.Time `json:"lastActivityAt"`
	Project              string     `gorm:"size:128" json:"project"`
	Tags                 string     `gorm:"size:1024" json:"tags"`
}

type HostProjectHintRecord struct {
	ID        uint      `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	StableID  string    `gorm:"uniqueIndex;size:128" json:"stableId"`
	Project   string    `gorm:"index;size:128" json:"project"`
}

type HostMetadataPatch struct {
	Project     *string
	Tags        *[]string
	DisplayName *string
}

type SessionRecord struct {
	ID               uint       `json:"id"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	SessionUID       string     `gorm:"uniqueIndex;size:128" json:"sessionUid"`
	Type             string     `gorm:"size:64" json:"type"`
	Status           string     `gorm:"size:32" json:"status"`
	Source           string     `gorm:"size:32" json:"source"`
	Username         string     `gorm:"size:128" json:"username"`
	Role             string     `gorm:"size:32" json:"role"`
	HostStableID     string     `gorm:"index;size:128" json:"hostStableId"`
	HostConnectionID string     `gorm:"size:128" json:"hostConnectionId"`
	Hostname         string     `gorm:"size:255" json:"hostname"`
	RemoteAddr       string     `gorm:"size:255" json:"remoteAddr"`
	Command          string     `gorm:"size:2048" json:"command"`
	ErrorText        string     `gorm:"size:2048" json:"errorText"`
	StartedAt        time.Time  `json:"startedAt"`
	EndedAt          *time.Time `json:"endedAt"`
}

type DashboardSummary struct {
	ConnectedHosts   int64 `json:"connectedHosts"`
	OfflineHosts     int64 `json:"offlineHosts"`
	ActiveSessions   int64 `json:"activeSessions"`
	CompletedToday   int64 `json:"completedToday"`
	AvailableBuilds  int64 `json:"availableBuilds"`
	ConfiguredHooks  int64 `json:"configuredHooks"`
	ProjectsCount    int64 `json:"projectsCount"`
	TaggedHostsCount int64 `json:"taggedHostsCount"`
}

func New(db *gorm.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("nil db")
	}

	if err := db.AutoMigrate(&WebUser{}, &HostRecord{}, &SessionRecord{}, &ProjectRecord{}, &ArtifactProjectRecord{}, &HostProjectHintRecord{}, &ProjectRuntimeRecord{}, &ProjectRuntimeSecretRecord{}); err != nil {
		return nil, err
	}

	store := &Store{db: db}
	if err := store.bootstrapProjectCatalog(); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *Store) EnsureUser(username, password, role, rsshUsername string) error {
	var existing WebUser
	result := s.db.Where("username = ?", username).Limit(1).Find(&existing)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		updates := map[string]any{
			"role":                 role,
			"r_ssh_username":       rsshUsername,
			"enabled":              true,
			"can_create_projects":  role == "admin",
			"must_change_password": false,
		}

		if password != "" {
			hash, hashErr := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if hashErr != nil {
				return hashErr
			}
			updates["password_hash"] = string(hash)
		}

		return s.db.Model(&existing).Updates(updates).Error
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	return s.db.Create(&WebUser{
		Username:           username,
		PasswordHash:       string(hash),
		SessionVersion:     0,
		MustChangePassword: false,
		Role:               role,
		RSSHUsername:       rsshUsername,
		Enabled:            true,
		CanCreateProjects:  role == "admin",
	}).Error
}

func (s *Store) Authenticate(username, password string) (WebUser, error) {
	var user WebUser
	result := s.db.Where("username = ? AND enabled = ?", username, true).Limit(1).Find(&user)
	if result.Error != nil {
		return WebUser{}, result.Error
	}
	if result.RowsAffected == 0 {
		return WebUser{}, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return WebUser{}, ErrInvalidCredentials
	}

	return user, nil
}

func (s *Store) GetUserByID(id uint) (WebUser, error) {
	var user WebUser
	err := s.db.First(&user, id).Error
	return user, err
}

func (s *Store) RotateUserSessionVersionByID(id uint) (WebUser, error) {
	if id == 0 {
		return WebUser{}, errors.New("user id is required")
	}
	if err := s.db.Model(&WebUser{}).
		Where("id = ?", id).
		UpdateColumn("session_version", gorm.Expr("session_version + 1")).Error; err != nil {
		return WebUser{}, err
	}
	return s.GetUserByID(id)
}

func (s *Store) RotateUserSessionVersion(username string) (WebUser, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return WebUser{}, errors.New("username is required")
	}
	if err := s.db.Model(&WebUser{}).
		Where("username = ?", username).
		UpdateColumn("session_version", gorm.Expr("session_version + 1")).Error; err != nil {
		return WebUser{}, err
	}
	return s.GetUserByUsername(username)
}

func (s *Store) UpsertHostFromSnapshot(snapshot users.ClientSnapshot, seenAt time.Time) (HostRecord, error) {
	var host HostRecord
	result := s.db.Where("stable_id = ?", snapshot.StableID).Limit(1).Find(&host)
	if result.Error != nil {
		return HostRecord{}, result.Error
	}

	projectHintKey := strings.TrimSpace(snapshot.PublicKeyFingerprint)
	if projectHintKey == "" {
		projectHintKey = strings.TrimSpace(snapshot.StableID)
	}
	projectHint, err := s.projectHintForStableID(projectHintKey)
	if err != nil {
		return HostRecord{}, err
	}

	if result.RowsAffected == 0 {
		host = HostRecord{
			StableID:    snapshot.StableID,
			FirstSeenAt: ptrTime(seenAt),
		}
	}

	host.ActiveConnectionID = snapshot.ConnectionID
	host.LastConnectionID = snapshot.ConnectionID
	host.PublicKeyFingerprint = strings.TrimSpace(snapshot.PublicKeyFingerprint)
	host.Hostname = snapshot.Hostname
	host.RemoteAddr = snapshot.RemoteAddr
	host.RemoteIP = snapshot.RemoteIP
	host.Comment = snapshot.Comment
	host.Version = snapshot.Version
	host.Owners = strings.Join(snapshot.Owners, ",")
	host.IsPublic = snapshot.IsPublic
	host.Connected = true
	switch strings.TrimSpace(host.Project) {
	case "":
		if projectHint != "" {
			host.Project = projectHint
		} else {
			host.Project = UnassignedProjectName
		}
	case UnassignedProjectName:
		if projectHint != "" {
			host.Project = projectHint
		}
	}
	host.LastSeenAt = ptrTime(seenAt)
	host.LastConnectedAt = ptrTime(seenAt)
	host.LastActivityAt = ptrTime(seenAt)

	if err := s.EnsureProject(host.Project); err != nil {
		return HostRecord{}, err
	}

	err = s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "stable_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"public_key_fingerprint", "active_connection_id", "last_connection_id", "hostname", "remote_addr", "remote_ip", "comment", "version", "owners", "is_public", "connected", "project", "last_seen_at", "last_connected_at", "last_activity_at", "updated_at"}),
	}).Create(&host).Error
	if err != nil {
		return HostRecord{}, err
	}

	err = s.db.Where("stable_id = ?", snapshot.StableID).First(&host).Error
	return host, err
}

func (s *Store) MarkHostDisconnected(stableID, connectionID string, at time.Time) error {
	if stableID == "" {
		return nil
	}

	return s.db.Model(&HostRecord{}).
		Where("stable_id = ?", stableID).
		Updates(map[string]any{
			"connected":            false,
			"active_connection_id": "",
			"last_connection_id":   connectionID,
			"last_seen_at":         at,
			"last_disconnected_at": at,
			"last_activity_at":     at,
		}).Error
}

func (s *Store) ReconcileConnectedHosts(liveStableIDs []string) error {
	seen := make(map[string]struct{}, len(liveStableIDs))
	filtered := make([]string, 0, len(liveStableIDs))
	for _, stableID := range liveStableIDs {
		stableID = strings.TrimSpace(stableID)
		if stableID == "" {
			continue
		}
		if _, ok := seen[stableID]; ok {
			continue
		}
		seen[stableID] = struct{}{}
		filtered = append(filtered, stableID)
	}

	query := s.db.Model(&HostRecord{}).
		Where("connected = ? OR active_connection_id <> ''", true)
	if len(filtered) > 0 {
		query = query.Where("stable_id NOT IN ?", filtered)
	}

	return query.Updates(map[string]any{
		"connected":            false,
		"active_connection_id": "",
	}).Error
}

func (s *Store) ReconcileProjectConnectedHosts(project string, liveStableIDs []string) error {
	projectName := DisplayProjectName(project)
	seen := make(map[string]struct{}, len(liveStableIDs))
	filtered := make([]string, 0, len(liveStableIDs))
	for _, stableID := range liveStableIDs {
		stableID = strings.TrimSpace(stableID)
		if stableID == "" {
			continue
		}
		if _, ok := seen[stableID]; ok {
			continue
		}
		seen[stableID] = struct{}{}
		filtered = append(filtered, stableID)
	}

	query := s.db.Model(&HostRecord{}).
		Where("project = ?", projectName).
		Where("connected = ? OR active_connection_id <> ''", true)
	if len(filtered) > 0 {
		query = query.Where("stable_id NOT IN ?", filtered)
	}

	return query.Updates(map[string]any{
		"connected":            false,
		"active_connection_id": "",
	}).Error
}

func (s *Store) TouchHostActivity(stableID string, at time.Time) error {
	if stableID == "" {
		return nil
	}

	return s.db.Model(&HostRecord{}).
		Where("stable_id = ?", stableID).
		Updates(map[string]any{
			"last_activity_at": at,
			"last_seen_at":     at,
		}).Error
}

func (s *Store) UpsertHostFromSnapshotForProject(snapshot users.ClientSnapshot, seenAt time.Time, project string) (HostRecord, error) {
	projectName := DisplayProjectName(project)
	projectHintKey := strings.TrimSpace(snapshot.PublicKeyFingerprint)
	if projectHintKey == "" {
		projectHintKey = strings.TrimSpace(snapshot.StableID)
	}
	if err := s.AssignHostProjectHint(projectHintKey, projectName); err != nil {
		return HostRecord{}, err
	}

	host, err := s.UpsertHostFromSnapshot(snapshot, seenAt)
	if err != nil {
		return HostRecord{}, err
	}

	if DisplayProjectName(host.Project) == projectName {
		return host, nil
	}

	return s.PatchHostMetadata(snapshot.StableID, HostMetadataPatch{
		Project: &projectName,
	})
}

func (s *Store) ListHosts() ([]HostRecord, error) {
	var hosts []HostRecord
	err := s.db.Order("connected desc, hostname asc, stable_id asc").Find(&hosts).Error
	return hosts, err
}

func (s *Store) GetHostByStableID(stableID string) (HostRecord, error) {
	var host HostRecord
	err := s.db.Where("stable_id = ?", stableID).First(&host).Error
	return host, err
}

func (s *Store) PatchHostMetadata(stableID string, patch HostMetadataPatch) (HostRecord, error) {
	host, err := s.GetHostByStableID(stableID)
	if err != nil {
		return HostRecord{}, err
	}

	updates := map[string]any{}
	var projectName string
	projectUpdated := false

	if patch.Project != nil {
		projectName = DisplayProjectName(*patch.Project)
		if err := s.EnsureProject(projectName); err != nil {
			return HostRecord{}, err
		}
		updates["project"] = projectName
		projectUpdated = true
	}

	if patch.Tags != nil {
		updates["tags"] = encodeTags(*patch.Tags)
	}

	if patch.DisplayName != nil {
		updates["display_name"] = strings.TrimSpace(*patch.DisplayName)
	}

	if len(updates) == 0 {
		return s.GetHostByStableID(stableID)
	}

	if err := s.db.Model(&HostRecord{}).
		Where("stable_id = ?", stableID).
		Updates(updates).Error; err != nil {
		return HostRecord{}, err
	}

	if projectUpdated {
		projectHintKey := strings.TrimSpace(host.PublicKeyFingerprint)
		if projectHintKey == "" {
			projectHintKey = strings.TrimSpace(stableID)
		}
		if err := s.AssignHostProjectHint(projectHintKey, projectName); err != nil {
			return HostRecord{}, err
		}
	}

	return s.GetHostByStableID(stableID)
}

func (s *Store) DeleteHost(stableID string) error {
	if stableID == "" {
		return nil
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("host_stable_id = ?", stableID).Delete(&SessionRecord{}).Error; err != nil {
			return err
		}

		if err := tx.Where("stable_id = ?", stableID).Delete(&HostProjectHintRecord{}).Error; err != nil {
			return err
		}

		return tx.Where("stable_id = ?", stableID).Delete(&HostRecord{}).Error
	})
}

func (s *Store) DeleteHosts(stableIDs []string) (int64, error) {
	filtered := make([]string, 0, len(stableIDs))
	seen := make(map[string]struct{}, len(stableIDs))
	for _, stableID := range stableIDs {
		stableID = strings.TrimSpace(stableID)
		if stableID == "" {
			continue
		}
		if _, ok := seen[stableID]; ok {
			continue
		}
		seen[stableID] = struct{}{}
		filtered = append(filtered, stableID)
	}
	if len(filtered) == 0 {
		return 0, nil
	}

	var deleted int64
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("host_stable_id IN ?", filtered).Delete(&SessionRecord{}).Error; err != nil {
			return err
		}

		if err := tx.Where("stable_id IN ?", filtered).Delete(&HostProjectHintRecord{}).Error; err != nil {
			return err
		}

		result := tx.Where("stable_id IN ?", filtered).Delete(&HostRecord{})
		if result.Error != nil {
			return result.Error
		}
		deleted = result.RowsAffected
		return nil
	})
	return deleted, err
}

func (s *Store) AssignHostProjectHint(stableID, project string) error {
	stableID = strings.TrimSpace(stableID)
	if stableID == "" {
		return nil
	}

	projectName := DisplayProjectName(project)
	if err := s.EnsureProject(projectName); err != nil {
		return err
	}

	record := HostProjectHintRecord{
		StableID: stableID,
		Project:  projectName,
	}

	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "stable_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"project", "updated_at"}),
	}).Create(&record).Error
}

func (s *Store) projectHintForStableID(stableID string) (string, error) {
	stableID = strings.TrimSpace(stableID)
	if stableID == "" {
		return "", nil
	}

	var record HostProjectHintRecord
	err := s.db.Where("stable_id = ?", stableID).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(record.Project), nil
}

func (s *Store) ListSessions(limit int) ([]SessionRecord, error) {
	if limit <= 0 {
		limit = 100
	}

	var sessions []SessionRecord
	err := s.db.Order("started_at desc").Limit(limit).Find(&sessions).Error
	return sessions, err
}

func (s *Store) StartSession(session SessionRecord) (SessionRecord, error) {
	if session.StartedAt.IsZero() {
		session.StartedAt = time.Now()
	}
	if session.Status == "" {
		session.Status = "active"
	}
	if err := s.db.Create(&session).Error; err != nil {
		return SessionRecord{}, err
	}
	return session, nil
}

func (s *Store) FinishSession(sessionUID, status, errText string, endedAt time.Time) error {
	updates := map[string]any{
		"status":     status,
		"ended_at":   endedAt,
		"error_text": errText,
	}
	return s.db.Model(&SessionRecord{}).Where("session_uid = ?", sessionUID).Updates(updates).Error
}

func (s *Store) CountCompletedSessionsForHostsSince(hostStableIDs []string, since time.Time) (int64, error) {
	filtered := make([]string, 0, len(hostStableIDs))
	seen := make(map[string]struct{}, len(hostStableIDs))
	for _, stableID := range hostStableIDs {
		stableID = strings.TrimSpace(stableID)
		if stableID == "" {
			continue
		}
		if _, ok := seen[stableID]; ok {
			continue
		}
		seen[stableID] = struct{}{}
		filtered = append(filtered, stableID)
	}
	if len(filtered) == 0 {
		return 0, nil
	}

	var count int64
	err := s.db.Model(&SessionRecord{}).
		Where("ended_at >= ?", since).
		Where("host_stable_id IN ?", filtered).
		Count(&count).Error
	return count, err
}

func (s *Store) UpsertAdminConsoleSession(event observers.AdminSessionState) error {
	var existing SessionRecord
	err := s.db.Where("session_uid = ?", event.SessionID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		record := SessionRecord{
			SessionUID: event.SessionID,
			Type:       "admin-console",
			Status:     normalizeSessionStatus(event.Status),
			Source:     event.Source,
			Username:   event.Username,
			Role:       event.Privilege,
			RemoteAddr: event.RemoteAddr,
			StartedAt:  event.Timestamp,
		}
		if record.Status != "active" {
			record.EndedAt = ptrTime(event.Timestamp)
		}
		return s.db.Create(&record).Error
	}
	if err != nil {
		return err
	}

	updates := map[string]any{
		"status":      normalizeSessionStatus(event.Status),
		"remote_addr": event.RemoteAddr,
		"role":        event.Privilege,
	}
	if event.Status == "disconnected" {
		updates["ended_at"] = event.Timestamp
	}

	return s.db.Model(&existing).Updates(updates).Error
}

func (s *Store) Summary() (DashboardSummary, error) {
	var summary DashboardSummary

	if err := s.db.Model(&SessionRecord{}).Where("status = ?", "active").Count(&summary.ActiveSessions).Error; err != nil {
		return DashboardSummary{}, err
	}

	startOfDay := time.Now().Truncate(24 * time.Hour)
	if err := s.db.Model(&SessionRecord{}).Where("ended_at >= ?", startOfDay).Count(&summary.CompletedToday).Error; err != nil {
		return DashboardSummary{}, err
	}

	type countRow struct {
		Count int64
	}
	var row countRow
	if err := s.db.Raw("SELECT COUNT(*) as count FROM downloads").Scan(&row).Error; err == nil {
		summary.AvailableBuilds = row.Count
	}
	if err := s.db.Raw("SELECT COUNT(*) as count FROM webhooks").Scan(&row).Error; err == nil {
		summary.ConfiguredHooks = row.Count
	}
	if err := s.db.Model(&HostRecord{}).Where("project <> ''").Distinct("project").Count(&summary.ProjectsCount).Error; err != nil {
		return DashboardSummary{}, err
	}
	if err := s.db.Model(&HostRecord{}).Where("tags <> ''").Count(&summary.TaggedHostsCount).Error; err != nil {
		return DashboardSummary{}, err
	}

	return summary, nil
}

func FilterHostsForUser(hosts []HostRecord, user WebUser) []HostRecord {
	if user.Role == "admin" {
		return hosts
	}

	filtered := make([]HostRecord, 0, len(hosts))
	for _, host := range hosts {
		if host.IsPublic {
			filtered = append(filtered, host)
			continue
		}

		for _, owner := range splitCSV(host.Owners) {
			if owner == user.RSSHUsername {
				filtered = append(filtered, host)
				break
			}
		}
	}

	return filtered
}

func DecodeTags(tags string) []string {
	return splitCSV(tags)
}

func encodeTags(tags []string) string {
	clean := make([]string, 0, len(tags))
	seen := make(map[string]bool, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		clean = append(clean, tag)
	}
	sort.Strings(clean)
	return strings.Join(clean, ",")
}

func splitCSV(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		result = append(result, part)
	}
	return result
}

func normalizeSessionStatus(status string) string {
	switch status {
	case "connected", "active":
		return "active"
	case "completed", "disconnected":
		return "completed"
	default:
		return status
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
