package detection

// Health says how well an entity is doing its job. The zero value means
// the finding has not been classified yet; Classify fills it in.
type Health uint8

// Health states, from best to worst. Unknown sits last because it means
// kwatch cannot tell, not that the entity is worse than failing.
const (
	// Healthy means the entity works as intended.
	Healthy Health = iota + 1
	// Degraded means the entity works, but less well than it should, or a
	// problem is building up.
	Degraded
	// Failing means the entity does not do its job.
	Failing
	// Unknown means kwatch cannot tell how the entity is doing.
	Unknown
)

// String returns the lower-case name of the health state.
func (h Health) String() string {
	switch h {
	case Healthy:
		return "healthy"
	case Degraded:
		return "degraded"
	case Failing:
		return "failing"
	case Unknown:
		return "unknown"
	default:
		return "unclassified"
	}
}

// healthBySeverity maps a detector's severity to the entity's health. An
// informational finding (a draining node, a suspended schedule) still
// means the entity is not fully serving, so it is Degraded, not Healthy.
var healthBySeverity = map[Severity]Health{
	Info:     Degraded,
	Warning:  Degraded,
	Critical: Failing,
}

// HealthFor returns the health implied by a severity. An unknown severity
// yields Unknown.
func HealthFor(severity Severity) Health {
	if h, ok := healthBySeverity[severity]; ok {
		return h
	}
	return Unknown
}

// ModeFor returns the short, stable failure identity of a reason. Reasons
// without a shorter name are their own mode.
func ModeFor(reason string) Mode {
	if mode, ok := modeByReason[reason]; ok {
		return mode
	}
	return Mode(reason)
}

// Classify fills in Health and Mode when a detector left them unset. It is
// applied to every finding the registry returns, so a new detector cannot
// forget it, and it is idempotent.
func Classify(f Finding) Finding {
	if f.Health == 0 {
		f.Health = HealthFor(f.Severity)
	}
	if f.Mode == "" {
		f.Mode = ModeFor(f.Reason)
	}
	return f
}
