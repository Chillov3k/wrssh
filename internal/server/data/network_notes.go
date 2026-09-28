package data

import (
	"errors"
	"strings"
	"time"

	"github.com/NHAS/reverse_ssh/internal/platform/networkmap"
	"gorm.io/gorm"
)

// NetworkNote is an operator text annotation placed on the project's network
// map. Notes are stored per runtime next to the scan discoveries.
type NetworkNote struct {
	ID        uint64    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	X         float64   `json:"x"`
	Y         float64   `json:"y"`
	W         float64   `json:"w"`
	H         float64   `json:"h"`
	Font      int       `json:"font"`
	Text      string    `gorm:"size:4096" json:"text"`
}

// UpsertNetworkNote creates or updates one note and returns the stored record.
func UpsertNetworkNote(note networkmap.Note) (networkmap.Note, error) {
	note = networkmap.ClampNote(note)

	db := DB()
	if db == nil {
		return networkmap.Note{}, errors.New("database is not loaded")
	}

	record := NetworkNote{
		X:    note.X,
		Y:    note.Y,
		W:    note.W,
		H:    note.H,
		Font: note.Font,
		Text: strings.TrimSpace(note.Text),
	}

	if note.ID > 0 {
		result := db.Model(&NetworkNote{}).Where("id = ?", note.ID).Updates(map[string]any{
			"x": record.X, "y": record.Y, "w": record.W, "h": record.H,
			"font": record.Font, "text": record.Text,
		})
		if result.Error != nil {
			return networkmap.Note{}, result.Error
		}
		if result.RowsAffected == 0 {
			return networkmap.Note{}, errors.New("note not found")
		}
		return readNetworkNote(db, note.ID)
	}

	if err := db.Create(&record).Error; err != nil {
		return networkmap.Note{}, err
	}
	return networkNoteToShared(record), nil
}

// ListNetworkNotes returns every stored note.
func ListNetworkNotes() ([]networkmap.Note, error) {
	var records []NetworkNote
	if err := DB().Order("id asc").Find(&records).Error; err != nil {
		return nil, err
	}

	notes := make([]networkmap.Note, 0, len(records))
	for _, record := range records {
		notes = append(notes, networkNoteToShared(record))
	}
	return notes, nil
}

// DeleteNetworkNote removes one note by id.
func DeleteNetworkNote(id uint64) error {
	if id == 0 {
		return errors.New("note id is required")
	}
	return DB().Where("id = ?", id).Delete(&NetworkNote{}).Error
}

func readNetworkNote(db *gorm.DB, id uint64) (networkmap.Note, error) {
	var record NetworkNote
	if err := db.Where("id = ?", id).First(&record).Error; err != nil {
		return networkmap.Note{}, err
	}
	return networkNoteToShared(record), nil
}

func networkNoteToShared(record NetworkNote) networkmap.Note {
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
