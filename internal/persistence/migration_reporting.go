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

// RecordMigrationResult lets the application include restore operations in
// the same startup-cycle report as schema migrations. The manager still owns
// copying and metric accounting; callers cannot mutate the report in place.
func (s *Manager) RecordMigrationResult(
	result MigrationResult,
	err error,
) {
	s.recordMigrationResult(result, err)
}

// BeginMigrationReport starts a new startup-cycle report. Application
// composition calls this before restore so reads, migrations, and writes are
// visible in one diagnostic snapshot.
func (s *Manager) BeginMigrationReport() {
	s.resetMigrationReport()
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
	if result.Operation == "" {
		// Keep older internal callers compatible while making every published
		// report entry explicit.
		result.Operation = OperationMigrate
	}
	if err != nil && result.Status != MigrationUnsupported {
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
