package runtimeagent

import "time"

func formatTimePointer(value *time.Time) *string {
	if value == nil || value.IsZero() {
		return nil
	}
	formatted := value.UTC().Format(time.RFC3339)
	return &formatted
}
