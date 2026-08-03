package model

// Severity is the alert escalation level for an incident.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityWarning  Severity = "warning"
	SeverityNormal   Severity = "normal"
)

// Rank orders severities for escalation decisions; higher is more severe.
// medium and warning share a rank; unknown or empty values rank as normal.
func (s Severity) Rank() int {
	switch s {
	case SeverityCritical:
		return 3
	case SeverityHigh:
		return 2
	case SeverityMedium, SeverityWarning:
		return 1
	default:
		return 0
	}
}

// SeverityFromString converts a user-supplied string to a Severity.
// Config maps (SeverityByReason, SeverityByOwnerKind) are user strings, so
// they are converted at the boundary when compared to typed severities.
func SeverityFromString(s string) Severity { return Severity(s) }
