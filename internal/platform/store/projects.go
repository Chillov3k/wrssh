package store

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/NHAS/reverse_ssh/internal/server/data"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const UnassignedProjectName = "Unassigned"

var (
	ErrProjectExists = errors.New("project already exists")
)

type ProjectRecord struct {
	ID          uint      `json:"id"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	Name        string    `gorm:"uniqueIndex;size:128" json:"name"`
	Description string    `gorm:"size:2048" json:"description"`
	Tags        string    `gorm:"size:1024" json:"tags"`
}

type ArtifactProjectRecord struct {
	ID        uint      `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	URLPath   string    `gorm:"uniqueIndex;size:255" json:"urlPath"`
	Project   string    `gorm:"index;size:128" json:"project"`
}

func DisplayProjectName(name string) string {
	project := strings.TrimSpace(name)
	if project == "" {
		return UnassignedProjectName
	}
	return project
}

func ProjectMatches(recordProject, selectedProject string) bool {
	selected := strings.TrimSpace(selectedProject)
	if selected == "" {
		return true
	}

	record := strings.TrimSpace(recordProject)
	if selected == UnassignedProjectName {
		return record == "" || record == UnassignedProjectName
	}

	return record == selected
}

func (s *Store) bootstrapProjectCatalog() error {
	if err := s.migrateLegacyUnassignedProject(); err != nil {
		return err
	}

	var hostProjects []string
	if err := s.db.Model(&HostRecord{}).
		Where("project <> ''").
		Distinct().
		Pluck("project", &hostProjects).Error; err != nil {
		return err
	}

	var artifactProjects []string
	if err := s.db.Model(&ArtifactProjectRecord{}).
		Where("project <> ''").
		Distinct().
		Pluck("project", &artifactProjects).Error; err != nil {
		return err
	}

	names := make(map[string]struct{}, len(hostProjects)+len(artifactProjects))
	for _, candidate := range append(hostProjects, artifactProjects...) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		names[candidate] = struct{}{}
	}

	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)

	for _, name := range ordered {
		if err := s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&ProjectRecord{
			Name: name,
		}).Error; err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) migrateLegacyUnassignedProject() error {
	var hostCount int64
	if err := s.db.Model(&HostRecord{}).
		Where("project IS NULL OR TRIM(project) = ''").
		Count(&hostCount).Error; err != nil {
		return err
	}

	var missingArtifactPaths []string
	if data.DB() != nil {
		downloads, err := data.ListDownloads("")
		if err != nil {
			return err
		}

		assignments, err := s.ListArtifactProjectAssignments()
		if err != nil {
			return err
		}

		for urlPath := range downloads {
			if strings.TrimSpace(assignments[urlPath]) == "" {
				missingArtifactPaths = append(missingArtifactPaths, urlPath)
			}
		}
		sort.Strings(missingArtifactPaths)
	}

	if hostCount == 0 && len(missingArtifactPaths) == 0 {
		return nil
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&ProjectRecord{
			Name: UnassignedProjectName,
		}).Error; err != nil {
			return err
		}

		if hostCount > 0 {
			if err := tx.Model(&HostRecord{}).
				Where("project IS NULL OR TRIM(project) = ''").
				Update("project", UnassignedProjectName).Error; err != nil {
				return err
			}
		}

		for _, urlPath := range missingArtifactPaths {
			record := ArtifactProjectRecord{
				URLPath: urlPath,
				Project: UnassignedProjectName,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "url_path"}},
				DoUpdates: clause.AssignmentColumns([]string{"project", "updated_at"}),
			}).Create(&record).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *Store) ListProjects() ([]ProjectRecord, error) {
	var projects []ProjectRecord
	err := s.db.Order("name asc").Find(&projects).Error
	return projects, err
}

func (s *Store) GetProject(name string) (ProjectRecord, error) {
	projectName, err := validateProjectName(name, false)
	if err != nil {
		return ProjectRecord{}, err
	}

	var project ProjectRecord
	err = s.db.Where("name = ?", projectName).First(&project).Error
	return project, err
}

func (s *Store) CreateProject(name, description string, tags []string) (ProjectRecord, error) {
	projectName, err := validateProjectName(name, true)
	if err != nil {
		return ProjectRecord{}, err
	}

	var existing ProjectRecord
	result := s.db.Where("name = ?", projectName).Limit(1).Find(&existing)
	if result.Error != nil {
		return ProjectRecord{}, result.Error
	}
	if result.RowsAffected > 0 {
		return ProjectRecord{}, ErrProjectExists
	}

	project := ProjectRecord{
		Name:        projectName,
		Description: strings.TrimSpace(description),
		Tags:        encodeTags(tags),
	}

	if err := s.db.Create(&project).Error; err != nil {
		return ProjectRecord{}, err
	}

	return project, nil
}

func (s *Store) UpdateProjectMetadata(name, description string, tags []string) (ProjectRecord, error) {
	projectName, err := validateProjectName(name, false)
	if err != nil {
		return ProjectRecord{}, err
	}

	if err := s.db.Model(&ProjectRecord{}).
		Where("name = ?", projectName).
		Updates(map[string]any{
			"description": strings.TrimSpace(description),
			"tags":        encodeTags(tags),
		}).Error; err != nil {
		return ProjectRecord{}, err
	}

	return s.GetProject(projectName)
}

func (s *Store) UpdateProject(currentName, newName, description string, tags []string) (ProjectRecord, error) {
	projectName, err := validateProjectName(currentName, false)
	if err != nil {
		return ProjectRecord{}, err
	}

	targetName := strings.TrimSpace(newName)
	if targetName == "" {
		targetName = projectName
	}
	targetName, err = validateProjectName(targetName, false)
	if err != nil {
		return ProjectRecord{}, err
	}

	var updated ProjectRecord
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var project ProjectRecord
		if err := tx.Where("name = ?", projectName).First(&project).Error; err != nil {
			return err
		}

		if targetName != projectName {
			var count int64
			if err := tx.Model(&ProjectRecord{}).
				Where("name = ?", targetName).
				Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return ErrProjectExists
			}
		}

		if err := tx.Model(&ProjectRecord{}).
			Where("name = ?", projectName).
			Updates(map[string]any{
				"name":        targetName,
				"description": strings.TrimSpace(description),
				"tags":        encodeTags(tags),
			}).Error; err != nil {
			return err
		}

		if targetName != projectName {
			if err := tx.Model(&HostRecord{}).
				Where("project = ?", projectName).
				Update("project", targetName).Error; err != nil {
				return err
			}

			if err := tx.Model(&ArtifactProjectRecord{}).
				Where("project = ?", projectName).
				Update("project", targetName).Error; err != nil {
				return err
			}

			if err := tx.Model(&HostProjectHintRecord{}).
				Where("project = ?", projectName).
				Update("project", targetName).Error; err != nil {
				return err
			}

			if err := renameProjectAccessTx(tx, projectName, targetName); err != nil {
				return err
			}
		}

		return tx.Where("name = ?", targetName).First(&updated).Error
	})
	if err != nil {
		return ProjectRecord{}, err
	}

	return updated, nil
}

func (s *Store) EnsureProject(name string) error {
	projectName := strings.TrimSpace(name)
	if projectName == "" {
		return nil
	}

	projectName, err := validateProjectName(projectName, false)
	if err != nil {
		return err
	}

	return s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&ProjectRecord{
		Name: projectName,
	}).Error
}

func (s *Store) AssignArtifactProject(urlPath, project string) error {
	urlPath = strings.TrimSpace(urlPath)
	if urlPath == "" {
		return nil
	}

	projectName := strings.TrimSpace(project)
	if projectName == "" {
		projectName = UnassignedProjectName
	}

	if err := s.EnsureProject(projectName); err != nil {
		return err
	}

	record := ArtifactProjectRecord{
		URLPath: urlPath,
		Project: projectName,
	}

	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "url_path"}},
		DoUpdates: clause.AssignmentColumns([]string{"project", "updated_at"}),
	}).Create(&record).Error
}

func (s *Store) DeleteArtifactProject(urlPath string) error {
	urlPath = strings.TrimSpace(urlPath)
	if urlPath == "" {
		return nil
	}

	return s.db.Where("url_path = ?", urlPath).Delete(&ArtifactProjectRecord{}).Error
}

func (s *Store) ListArtifactProjectAssignments() (map[string]string, error) {
	var records []ArtifactProjectRecord
	if err := s.db.Order("url_path asc").Find(&records).Error; err != nil {
		return nil, err
	}

	assignments := make(map[string]string, len(records))
	for _, record := range records {
		assignments[record.URLPath] = strings.TrimSpace(record.Project)
	}

	return assignments, nil
}

func (s *Store) ListHostsForProject(name string) ([]HostRecord, error) {
	projectName, err := validateProjectName(name, false)
	if err != nil {
		return nil, err
	}

	var hosts []HostRecord
	err = s.db.Where("project = ?", projectName).
		Order("connected desc, hostname asc, stable_id asc").
		Find(&hosts).Error
	return hosts, err
}

func (s *Store) ListArtifactPathsForProject(name string) ([]string, error) {
	projectName, err := validateProjectName(name, false)
	if err != nil {
		return nil, err
	}

	var paths []string
	err = s.db.Model(&ArtifactProjectRecord{}).
		Where("project = ?", projectName).
		Order("url_path asc").
		Pluck("url_path", &paths).Error
	return paths, err
}

func (s *Store) DeleteProjectCascade(name string) error {
	projectName, err := validateProjectName(name, false)
	if err != nil {
		return err
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		var project ProjectRecord
		if err := tx.Where("name = ?", projectName).First(&project).Error; err != nil {
			return err
		}

		hostSubquery := tx.Model(&HostRecord{}).
			Select("stable_id").
			Where("project = ?", projectName)

		if err := tx.Where("host_stable_id IN (?)", hostSubquery).
			Delete(&SessionRecord{}).Error; err != nil {
			return err
		}

		if err := tx.Where("project = ?", projectName).
			Delete(&HostRecord{}).Error; err != nil {
			return err
		}

		if err := tx.Where("project = ?", projectName).
			Delete(&ArtifactProjectRecord{}).Error; err != nil {
			return err
		}

		if err := tx.Where("project = ?", projectName).
			Delete(&HostProjectHintRecord{}).Error; err != nil {
			return err
		}

		if err := removeProjectAccessTx(tx, projectName); err != nil {
			return err
		}

		if err := deleteProjectRuntimeByProjectIDTx(tx, project.ID); err != nil {
			return err
		}

		return tx.Where("name = ?", projectName).Delete(&ProjectRecord{}).Error
	})
}

func validateProjectName(name string, allowReserved bool) (string, error) {
	projectName := strings.TrimSpace(name)
	if projectName == "" {
		return "", errors.New("project name is required")
	}
	if strings.Contains(projectName, "/") {
		return "", errors.New("project name cannot contain '/'")
	}
	return projectName, nil
}

func renameProjectAccessTx(tx *gorm.DB, oldProject, newProject string) error {
	var users []WebUser
	if err := tx.Find(&users).Error; err != nil {
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

		if err := tx.Model(&WebUser{}).
			Where("username = ?", user.Username).
			Update("allowed_projects", encodeProjectList(projects)).Error; err != nil {
			return err
		}
	}

	return nil
}

func removeProjectAccessTx(tx *gorm.DB, projectName string) error {
	var users []WebUser
	if err := tx.Find(&users).Error; err != nil {
		return err
	}

	for _, user := range users {
		if user.Role == "admin" {
			continue
		}

		projects := user.AllowedProjectList()
		filtered := projects[:0]
		for _, project := range projects {
			if project != projectName {
				filtered = append(filtered, project)
			}
		}
		if len(filtered) == len(projects) {
			continue
		}

		if err := tx.Model(&WebUser{}).
			Where("username = ?", user.Username).
			Update("allowed_projects", encodeProjectList(filtered)).Error; err != nil {
			return err
		}
	}

	return nil
}
