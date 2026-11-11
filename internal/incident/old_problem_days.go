package incident

import "time"

// OldProblemDays is the configured horizon of an old problem in whole
// days (Config.OldProblem); zero when old problems are off. The
// dashboard words its "already failing for N+ days" line from it.
func (m *Manager) OldProblemDays() int {
	return int(m.cfg.OldProblem / (24 * time.Hour))
}
