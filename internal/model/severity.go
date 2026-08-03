package model

import "strings"

// Severity is the alert escalation level for an incident.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityWarning  Severity = "warning"
	SeverityNormal   Severity = "normal"
)

// IsValidSeverity reports whether s is a recognized severity level,
// compared case-insensitively. Config maps (SeverityByReason,
// SeverityByOwnerKind) and CRD severityByOwnerKind values are validated
// against this so a typo is rejected instead of silently ranking as normal.
func IsValidSeverity(s string) bool {
	switch Severity(strings.ToLower(strings.TrimSpace(s))) {
	case SeverityCritical, SeverityHigh, SeverityMedium, SeverityWarning, SeverityNormal:
		return true
	default:
		return false
	}
}

// NormalizeSeverity lowercases s (e.g. "High" → "high") so case variants of
// user-supplied config values still rank correctly.
func NormalizeSeverity(s string) Severity {
	return Severity(strings.ToLower(strings.TrimSpace(s)))
}

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
