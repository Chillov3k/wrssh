package store

import (
	"errors"
	"strings"
	"time"

	"github.com/NHAS/reverse_ssh/internal/platform/networkmap"
)

// NetworkNoteRecord is an operator text annotation on the project's network
// map. It is only used for projects without an isolated runtime; runtime
// projects keep map notes in the per-project postgres container.
type NetworkNoteRecord struct {
	ID        uint64 `json:"id"`
	Project   string `gorm:"index;size:128" json:"project"`
	X         float64
	Y         float64
	W         float64
	H         float64
	Font      int
	Text      string `gorm:"size:4096"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// UpsertNetworkNoteForProject creates or updates one map note and returns the
// stored record.
func (s *Store) UpsertNetworkNoteForProject(project string, note networkmap.Note) (networkmap.Note, error) {
	projectName := DisplayProjectName(project)
	note = networkmap.ClampNote(note)
	text := strings.TrimSpace(note.Text)

	if note.ID > 0 {
		result := s.db.Model(&NetworkNoteRecord{}).
			Where("id = ? AND project = ?", note.ID, projectName).
			Updates(map[string]any{
				"x": note.X, "y": note.Y, "w": note.W, "h": note.H,
				"font": note.Font, "text": text,
			})
		if result.Error != nil {
			return networkmap.Note{}, result.Error
		}
		if result.RowsAffected == 0 {
			return networkmap.Note{}, errors.New("note not found")
		}
		return s.networkNoteByID(projectName, note.ID)
	}

	record := NetworkNoteRecord{
		Project: projectName,
		X:       note.X,
		Y:       note.Y,
		W:       note.W,
		H:       note.H,
		Font:    note.Font,
		Text:    text,
	}
	if err := s.db.Create(&record).Error; err != nil {
		return networkmap.Note{}, err
	}
	return networkNoteRecordToShared(record), nil
}

// ListNetworkNotesForProject returns every stored map note of a project.
func (s *Store) ListNetworkNotesForProject(project string) ([]networkmap.Note, error) {
	projectName := DisplayProjectName(project)

	var records []NetworkNoteRecord
	if err := s.db.Where("project = ?", projectName).Order("id asc").Find(&records).Error; err != nil {
		return nil, err
	}

	notes := make([]networkmap.Note, 0, len(records))
	for _, record := range records {
		notes = append(notes, networkNoteRecordToShared(record))
	}
	return notes, nil
}

// DeleteNetworkNoteForProject removes one map note of a project.
func (s *Store) DeleteNetworkNoteForProject(project string, id uint64) error {
	if id == 0 {
		return errors.New("note id is required")
	}
	return s.db.Where("id = ? AND project = ?", id, DisplayProjectName(project)).Delete(&NetworkNoteRecord{}).Error
}

func (s *Store) networkNoteByID(projectName string, id uint64) (networkmap.Note, error) {
	var record NetworkNoteRecord
	if err := s.db.Where("id = ? AND project = ?", id, projectName).First(&record).Error; err != nil {
		return networkmap.Note{}, err
	}
	return networkNoteRecordToShared(record), nil
}

func networkNoteRecordToShared(record NetworkNoteRecord) networkmap.Note {
	return networkmap.Note{
		ID:   record.ID,
		X:    record.X,
		Y:    record.Y,
		W:    record.W,
		H:    record.H,
		Font: record.Font,
		Text: record.Text,
	}
}
