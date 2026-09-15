package persistence

import "encoding/json"

// StatusJSON exposes the structured migration report through the health
// diagnostics boundary without exposing ConfigMap or client details.
func (s *Manager) StatusJSON() ([]byte, error) {
	return json.Marshal(struct {
		Migrations []MigrationResult `json:"migrations"`
	}{Migrations: s.MigrationReport()})
}
