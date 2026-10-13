package selfcheck

import "time"

// Outcome says what recovery followed.
type Outcome string

// Outcomes of a resolved incident.
const (
	// CauseConfirmed: the cause entity changed or recovered before the
	// members recovered.
	CauseConfirmed Outcome = "cause"
	// OtherChanged: something else near the incident changed, but not
	// the cause.
	OtherChanged Outcome = "other"
	// NothingChanged: no change preceded recovery.
	NothingChanged Outcome = "none"
)

// Evidence is what the model showed about one resolved incident. A zero
// time means it did not happen while the incident was open.
type Evidence struct {
	// MembersRecovered is when the last member stopped failing.
	MembersRecovered time.Time
	// CauseChanged is the latest meaningful change of the cause entity.
	CauseChanged time.Time
	// CauseRecovered is when the cause entity stopped failing.
	CauseRecovered time.Time
	// OtherChanged is the latest meaningful change of any other entity
	// the incident touches.
	OtherChanged time.Time
}

// Classify judges one resolved incident. The cause counts as confirmed
// only when its change or recovery came no later than the members'
// recovery: a change made afterwards cannot have fixed anything.
func Classify(e Evidence) Outcome {
	if before(e.CauseChanged, e.MembersRecovered) ||
		before(e.CauseRecovered, e.MembersRecovered) {
		return CauseConfirmed
	}
	if before(e.OtherChanged, e.MembersRecovered) {
		return OtherChanged
	}
	return NothingChanged
}

// before reports a time that happened, no later than limit.
func before(t, limit time.Time) bool {
	return !t.IsZero() && !t.After(limit)
}
