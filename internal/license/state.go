package license

import (
	"errors"
	"time"
)

// State is the licensing state of an installation.
type State string

// States.
const (
	StateCommunity State = "community"
	StateInvalid   State = "invalid"
	StatePending   State = "pending"
	StateActive    State = "active"
	StateGrace     State = "grace"
	StateExpired   State = "expired"
	StateRevoked   State = "revoked"
	StateConflict  State = "conflict"
)

// ProEnabled reports whether Pro features may run in this state.
func (s State) ProEnabled() bool {
	return s == StateActive || s == StateGrace
}

// Verdict is the most recent decision reported by the license server.
type Verdict string

// Verdicts.
const (
	VerdictNone        Verdict = ""
	VerdictRevoked     Verdict = "revoked"
	VerdictConflict    Verdict = "conflict"
	VerdictSuspended   Verdict = "suspended"
	VerdictKeyReplaced Verdict = "key_replaced"
	VerdictSuperseded  Verdict = "instance_superseded"
	VerdictExpired     Verdict = "expired"
	VerdictTrialUsed   Verdict = "trial_already_used"
)

// Inputs are everything Evaluate needs; it performs no I/O.
type Inputs struct {
	// Verifier defaults to DefaultVerifier when nil.
	Verifier      *Verifier
	KeyString     string
	Lease         string
	Binding       Binding
	Now           time.Time
	Clock         ClockGuard
	ServerVerdict Verdict
}

// Status is the result of Evaluate.
type Status struct {
	State      State
	Plan       string
	LID        string // masked
	ExpiresAt  time.Time
	GraceUntil time.Time
	Reason     string
}

// MaskLID shows only "lic_…" and the last four characters of a license id.
func MaskLID(lid string) string {
	const prefix = "lic_…"
	if len(lid) <= 4 {
		return prefix
	}
	return prefix + lid[len(lid)-4:]
}

// Evaluate computes the licensing status. Time is taken as the later of Now
// and the clock guard's high-water mark, so rolling the clock back cannot
// extend a license.
func Evaluate(in Inputs) Status {
	if in.KeyString == "" {
		return Status{State: StateCommunity}
	}
	v := in.Verifier
	if v == nil {
		v = DefaultVerifier()
	}
	eff := in.Clock.Effective(in.Now)

	key, err := v.VerifyKey(in.KeyString, eff)
	if err != nil && !errors.Is(err, ErrExpired) {
		return Status{State: StateInvalid, Reason: err.Error()}
	}
	want := in.Binding
	want.LID = key.LID
	lease, leaseErr := verifiedLease(v, in.Lease, want, eff)
	// A verified lease's lic_exp is the server's latest word on the
	// license end, so it wins over the key in both directions: an
	// extension outlives the key and an early end (a shortened or
	// cancelled license) applies offline, without waiting for a verdict.
	end := key.ExpiresAt()
	if lic := lease.LicenseEnd(); leaseErr == nil && !lic.IsZero() {
		end = lic
	}
	st := Status{Plan: key.Plan, LID: MaskLID(key.LID), ExpiresAt: end}
	if !eff.Before(end) {
		st.State, st.Reason = StateExpired, "license expired"
		return st
	}
	switch in.ServerVerdict {
	case VerdictRevoked:
		st.State, st.Reason = StateRevoked, "license revoked"
		return st
	case VerdictConflict:
		st.State, st.Reason = StateConflict, "license active on another cluster"
		return st
	case VerdictSuspended:
		st.State, st.Reason = StateRevoked, "license suspended"
		return st
	case VerdictKeyReplaced:
		st.State, st.Reason = StateInvalid, "license key replaced; use the newest key"
		return st
	case VerdictSuperseded:
		st.State, st.Reason = StateConflict,
			"another instance is running with this license"
		return st
	case VerdictExpired:
		st.State, st.Reason = StateExpired, "license expired"
		return st
	case VerdictTrialUsed:
		st.State, st.Reason = StateInvalid, "license already used on this cluster"
		return st
	}
	return evaluateLease(in, lease, leaseErr, st)
}

// verifiedLease verifies s, or returns errNoLease when there is none.
func verifiedLease(
	v *Verifier, s string, want Binding, now time.Time,
) (Lease, error) {
	if s == "" {
		return Lease{}, errNoLease
	}
	return v.VerifyLease(s, want, now)
}

var errNoLease = errors.New("license: no lease")

func evaluateLease(in Inputs, lease Lease, err error, st Status) Status {
	switch {
	case errors.Is(err, errNoLease):
		st.State, st.Reason = StatePending, "awaiting activation"
		return st
	case errors.Is(err, ErrLeaseNotYetValid):
		st.State, st.Reason = StatePending, err.Error()
		return st
	case errors.Is(err, ErrLeaseExpired):
		st.State, st.Reason = StateExpired, err.Error()
		st.GraceUntil = lease.GraceUntil()
		return st
	case err != nil:
		st.State, st.Reason = StateInvalid, err.Error()
		return st
	}
	st.GraceUntil = lease.GraceUntil()
	switch {
	case lease.InGrace:
		st.State, st.Reason = StateGrace, "lease expired; renew soon"
	case in.Clock.RolledBack(in.Now):
		st.State, st.Reason = StateGrace, "system clock moved backwards"
	default:
		st.State = StateActive
	}
	return st
}
