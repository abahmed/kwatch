package persistence

import (
	"encoding/json"
	"time"
)

// StatusJSON exposes the structured migration report through the health
// diagnostics boundary without exposing ConfigMap or client details.
func (s *Manager) StatusJSON() ([]byte, error) {
	report := s.MigrationReport()
	return json.Marshal(struct {
		Migrations  []MigrationResult `json:"migrations"`
		StartedAt   time.Time         `json:"startedAt,omitempty"`
		CompletedAt time.Time         `json:"completedAt,omitempty"`
	}{
		Migrations:  report.Operations,
		StartedAt:   report.StartedAt,
		CompletedAt: report.CompletedAt,
	})
}
