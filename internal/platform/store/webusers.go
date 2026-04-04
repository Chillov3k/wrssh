package store

import (
	"errors"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrUserExists        = errors.New("user already exists")
	ErrInvalidUsername   = errors.New("username must contain only letters, numbers, dot, dash or underscore")
	ErrCannotDeleteAdmin = errors.New("cannot delete the last admin user")
	usernamePattern      = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

type WebUserPatch struct {
	Password           *string
	MustChangePassword *bool
	CanCreateProjects  *bool
	AllowedProjects    *[]string
	SSHAuthorizedKeys  *string
	Enabled            *bool
}

type ProjectAccessUser struct {
	Username       string `json:"username"`
	AuthorizedKeys string `json:"authorizedKeys"`
}

func (u WebUser) AllowedProjectList() []string {
	if u.Role == "admin" {
		return nil
	}
	return splitCSV(u.AllowedProjects)
}

func UserCanCreateProjects(user WebUser) bool {
	return user.Role == "admin" || user.CanCreateProjects
}

func UserCanAccessProject(user WebUser, project string) bool {
	if user.Role == "admin" {
		return true
	}

	projectName := DisplayProjectName(project)
	for _, allowed := range user.AllowedProjectList() {
		if allowed == projectName {
			return true
		}
	}

	return false
}

func (s *Store) ListWebUsers() ([]WebUser, error) {
	var users []WebUser
	err := s.db.Order("role asc, username asc").Find(&users).Error
	return users, err
}

func (s *Store) ListProjectAccessUsers(project string) ([]ProjectAccessUser, error) {
	projectName := DisplayProjectName(project)

	users, err := s.ListWebUsers()
	if err != nil {
		return nil, err
	}

	merged := make(map[string][]string)
	for _, user := range users {
		if !user.Enabled {
			continue
		}
		if strings.TrimSpace(user.SSHAuthorizedKeys) == "" {
			continue
		}
		if user.Role != "admin" && !UserCanAccessProject(user, projectName) {
			continue
		}

		username := strings.TrimSpace(user.RSSHUsername)
		if username == "" {
			username = user.Username
		}
		if username == "" {
			continue
		}

		merged[username] = append(merged[username], strings.TrimSpace(user.SSHAuthorizedKeys))
	}

	usernames := make([]string, 0, len(merged))
	for username := range merged {
		usernames = append(usernames, username)
	}
	sort.Strings(usernames)

	result := make([]ProjectAccessUser, 0, len(usernames))
	for _, username := range usernames {
		result = append(result, ProjectAccessUser{
			Username:       username,
			AuthorizedKeys: strings.Join(merged[username], "\n"),
		})
	}

	return result, nil
}

func (s *Store) GetUserByUsername(username string) (WebUser, error) {
	var user WebUser
	err := s.db.Where("username = ?", strings.TrimSpace(username)).First(&user).Error
	return user, err
}

func (s *Store) CreateManagedUser(username, password string, canCreateProjects bool, allowedProjects []string, sshAuthorizedKeys string) (WebUser, error) {
	username, err := validateManagedUsername(username)
	if err != nil {
		return WebUser{}, err
	}
	if err := s.validateAssignedProjects(allowedProjects); err != nil {
		return WebUser{}, err
	}

	var existing WebUser
	result := s.db.Where("username = ?", username).Limit(1).Find(&existing)
	if result.Error != nil {
		return WebUser{}, result.Error
	}
	if result.RowsAffected > 0 {
		return WebUser{}, ErrUserExists
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return WebUser{}, err
	}

	user := WebUser{
		Username:           username,
		PasswordHash:       string(hash),
		MustChangePassword: true,
		Role:               "user",
		RSSHUsername:       username,
		Enabled:            true,
		CanCreateProjects:  canCreateProjects,
		AllowedProjects:    encodeProjectList(allowedProjects),
		SSHAuthorizedKeys:  strings.TrimSpace(sshAuthorizedKeys),
	}

	if err := s.db.Create(&user).Error; err != nil {
		return WebUser{}, err
	}

	return user, nil
}

func (s *Store) UpdateManagedUser(username string, patch WebUserPatch) (WebUser, error) {
	username, err := validateManagedUsername(username)
	if err != nil {
		return WebUser{}, err
	}

	updates := map[string]any{}
	if patch.Password != nil {
		hash, err := bcrypt.GenerateFromPassword([]byte(*patch.Password), bcrypt.DefaultCost)
		if err != nil {
			return WebUser{}, err
		}
		updates["password_hash"] = string(hash)
	}

	if patch.MustChangePassword != nil {
		updates["must_change_password"] = *patch.MustChangePassword
	}

	if patch.CanCreateProjects != nil {
		updates["can_create_projects"] = *patch.CanCreateProjects
	}

	if patch.AllowedProjects != nil {
		if err := s.validateAssignedProjects(*patch.AllowedProjects); err != nil {
			return WebUser{}, err
		}
		updates["allowed_projects"] = encodeProjectList(*patch.AllowedProjects)
	}

	if patch.SSHAuthorizedKeys != nil {
		updates["ssh_authorized_keys"] = strings.TrimSpace(*patch.SSHAuthorizedKeys)
	}

	if patch.Enabled != nil {
		updates["enabled"] = *patch.Enabled
	}

	if len(updates) == 0 {
		return s.GetUserByUsername(username)
	}

	if err := s.db.Model(&WebUser{}).Where("username = ?", username).Updates(updates).Error; err != nil {
		return WebUser{}, err
	}

	return s.GetUserByUsername(username)
}

func (s *Store) DeleteManagedUser(username string) error {
	username, err := validateManagedUsername(username)
	if err != nil {
		return err
	}

	user, err := s.GetUserByUsername(username)
	if err != nil {
		return err
	}

	if user.Role == "admin" {
		var admins int64
		if err := s.db.Model(&WebUser{}).Where("role = ? AND enabled = ?", "admin", true).Count(&admins).Error; err != nil {
			return err
		}
		if admins <= 1 {
			return ErrCannotDeleteAdmin
		}
	}

	return s.db.Where("username = ?", username).Delete(&WebUser{}).Error
}

func (s *Store) GrantUserProjectAccess(username, project string) error {
	user, err := s.GetUserByUsername(username)
	if err != nil {
		return err
	}
	if user.Role == "admin" {
		return nil
	}

	projects := append(user.AllowedProjectList(), DisplayProjectName(project))
	return s.db.Model(&WebUser{}).
		Where("username = ?", username).
		Update("allowed_projects", encodeProjectList(projects)).Error
}

func (s *Store) RenameProjectAccess(oldProject, newProject string) error {
	oldProject = DisplayProjectName(oldProject)
	newProject = DisplayProjectName(newProject)
	if oldProject == newProject {
		return nil
	}

	users, err := s.ListWebUsers()
	if err != nil {
		return err
	}

	for _, user := range users {
		if user.Role == "admin" {
			continue
		}
		projects := user.AllowedProjectList()
		changed := false
		for index, project := range projects {
			if project == oldProject {
				projects[index] = newProject
				changed = true
			}
		}
		if !changed {
			continue
		}
		if err := s.db.Model(&WebUser{}).
			Where("username = ?", user.Username).
			Update("allowed_projects", encodeProjectList(projects)).Error; err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) RemoveProjectAccess(project string) error {
	project = DisplayProjectName(project)

	users, err := s.ListWebUsers()
	if err != nil {
		return err
	}

	for _, user := range users {
		if user.Role == "admin" {
			continue
		}

		projects := user.AllowedProjectList()
		filtered := projects[:0]
		for _, candidate := range projects {
			if candidate != project {
				filtered = append(filtered, candidate)
			}
		}
		if len(filtered) == len(projects) {
			continue
		}
		if err := s.db.Model(&WebUser{}).
			Where("username = ?", user.Username).
			Update("allowed_projects", encodeProjectList(filtered)).Error; err != nil {
			return err
		}
	}

	return nil
}

func validateManagedUsername(username string) (string, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return "", errors.New("username is required")
	}
	if !usernamePattern.MatchString(username) {
		return "", ErrInvalidUsername
	}
	return username, nil
}

func (s *Store) validateAssignedProjects(projects []string) error {
	seen := make(map[string]struct{}, len(projects))
	for _, project := range projects {
		projectName := DisplayProjectName(project)
		if _, ok := seen[projectName]; ok {
			continue
		}
		seen[projectName] = struct{}{}
		if _, err := s.GetProject(projectName); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("assigned project does not exist: " + projectName)
			}
			return err
		}
	}
	return nil
}

func encodeProjectList(projects []string) string {
	clean := make([]string, 0, len(projects))
	seen := make(map[string]struct{}, len(projects))
	for _, project := range projects {
		projectName := DisplayProjectName(project)
		if projectName == "" {
			continue
		}
		if _, ok := seen[projectName]; ok {
			continue
		}
		seen[projectName] = struct{}{}
		clean = append(clean, projectName)
	}
	sort.Strings(clean)
	return strings.Join(clean, ",")
}
