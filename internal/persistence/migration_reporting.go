package persistence

import "github.com/abahmed/kwatch/internal/metrics"

// MigrationReport returns all migration outcomes recorded for the current
// startup cycle. The returned slice is independent of manager state.
func (s *Manager) MigrationReport() MigrationReport {
	s.migrationMu.RLock()
	defer s.migrationMu.RUnlock()
	return MigrationReport{
		Operations: append(
			[]MigrationResult(nil), s.migrationReport.Operations...,
		),
		StartedAt:   s.migrationReport.StartedAt,
		CompletedAt: s.migrationReport.CompletedAt,
	}
}

func (s *Manager) resetMigrationReport() {
	s.migrationMu.Lock()
	s.migrationReport = MigrationReport{StartedAt: s.nowTime()}
	s.migrationMu.Unlock()
}

func (s *Manager) recordMigrationResult(
	result MigrationResult,
	err error,
) {
	if err != nil {
		result.Status = MigrationFailed
		if result.Detail == "" {
			result.Detail = "migration operation failed"
		}
	}
	s.migrationMu.Lock()
	if s.migrationReport.StartedAt.IsZero() {
		s.migrationReport.StartedAt = s.nowTime()
	}
	s.migrationReport.Operations = append(
		s.migrationReport.Operations, result,
	)
	s.migrationReport.CompletedAt = s.nowTime()
	s.migrationMu.Unlock()
	metrics.DefaultRegistry().PersistenceMigrations.Add(1)
	if result.Status == MigrationFailed ||
		result.Status == MigrationUnsupported {
		metrics.DefaultRegistry().PersistenceMigrationErr.Add(1)
	}
}
