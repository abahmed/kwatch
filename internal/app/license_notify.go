package app

import (
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/license"
	"github.com/abahmed/kwatch/internal/notification"
)

// Expiry warning thresholds, in whole days left.
const (
	renewURL         = "https://kwatch.dev/account"
	expiryWarnDays   = 14
	expiryUrgentDays = 3
)

// licenseNotified is what the operator has already been told. It is kept
// in the state file so a restart never repeats a notice.
type licenseNotified struct {
	// Pro is true while Pro is enabled and the operator knows it.
	Pro bool `json:"pro"`
	// Trial14 and Trial3 (persisted JSON keys, unchanged) hold the
	// license the warning was sent for.
	Trial14 string `json:"trial14,omitempty"`
	Trial3  string `json:"trial3,omitempty"`
	// Grace holds the end of the grace window the notice was sent for.
	Grace string `json:"grace,omitempty"`
	// Disabled is the state and reason the last "Pro is disabled" notice
	// was sent for, so a key that never ran Pro, or lost it, is told once
	// per distinct reason.
	Disabled string `json:"disabled,omitempty"`
}

// licenseNotices decides which messages a status change calls for and
// returns the updated record. Each transition yields at most one message:
// Pro enabled, Pro disabled, grace entered, and the two expiry warnings.
func licenseNotices(
	rec licenseNotified, st license.Status, now time.Time,
) ([]string, licenseNotified) {
	var msgs []string
	switch {
	case st.State.ProEnabled() && !rec.Pro:
		rec.Pro, rec.Disabled = true, ""
		msgs = append(msgs, proEnabledNotice(st))
	case st.State.ProEnabled():
		rec.Disabled = ""
	case rec.Pro && proLostState(st.State):
		rec.Pro, rec.Grace = false, ""
		rec.Disabled = unusableMarker(st)
		msgs = append(msgs, proDisabledNotice(st))
	default:
		if note, updated := neverProNotice(rec, st); note != "" {
			rec = updated
			msgs = append(msgs, note)
		}
	}
	if st.State == license.StateGrace {
		marker := fmt.Sprint(st.GraceUntil.Unix())
		if rec.Grace != marker {
			rec.Grace = marker
			msgs = append(msgs, graceNotice(st))
		}
	} else if st.State == license.StateActive {
		rec.Grace = ""
	}
	if note, updated := expiryNotice(rec, st, now); note != "" {
		rec = updated
		msgs = append(msgs, note)
	}
	return msgs, rec
}

// proLostState reports a state that ends Pro for a reason the operator
// should hear about. Pending (no answer yet) does not.
func proLostState(s license.State) bool {
	switch s {
	case license.StateCommunity, license.StateInvalid,
		license.StateExpired, license.StateRevoked, license.StateConflict:
		return true
	}
	return false
}

// unusableMarker identifies a state a key cannot run Pro in; empty for the
// states that are not about the key (no key, no answer yet).
func unusableMarker(st license.Status) string {
	switch st.State {
	case license.StateInvalid, license.StateExpired,
		license.StateRevoked, license.StateConflict:
		return string(st.State) + ": " + st.Reason
	}
	return ""
}

// neverProNotice is the notice for a configured key that cannot run Pro
// although Pro never ran (a mistyped, expired or revoked key at the first
// start), or whose reason changed since the last notice. Without it the
// operator would only see Community and never learn why.
func neverProNotice(
	rec licenseNotified, st license.Status,
) (string, licenseNotified) {
	marker := unusableMarker(st)
	if marker == "" || marker == rec.Disabled {
		return "", rec
	}
	rec.Disabled = marker
	return proDisabledNotice(st), rec
}

func proEnabledNotice(st license.Status) string {
	return notification.MarkerLow + " kwatch Pro is enabled."
}

func proDisabledNotice(st license.Status) string {
	reason := st.Reason
	if st.State == license.StateCommunity {
		reason = "no license key is configured"
	}
	if reason == "" {
		reason = string(st.State)
	}
	return notification.MarkerNotify + " kwatch Pro is disabled: " +
		reason + ". kwatch continues in the Community edition."
}

func graceNotice(st license.Status) string {
	return notification.MarkerNotify + " the kwatch license could not be " +
		"renewed and is in its grace period until " +
		st.GraceUntil.UTC().Format("2006-01-02 15:04") +
		" UTC; Pro features stop then unless it renews."
}

// expiryNotice returns the expiry warning that is due, if any. It applies
// to every plan: the API issues paid licenses as "pro", so gating on a
// plan name would silence the warnings.
func expiryNotice(
	rec licenseNotified, st license.Status, now time.Time,
) (string, licenseNotified) {
	if st.State != license.StateActive || st.ExpiresAt.IsZero() {
		return "", rec
	}
	left := st.ExpiresAt.Sub(now)
	if left <= 0 {
		return "", rec
	}
	days := int((left + 24*time.Hour - 1) / (24 * time.Hour))
	switch {
	case days <= expiryUrgentDays && rec.Trial3 != st.LID:
		rec.Trial3, rec.Trial14 = st.LID, st.LID
	case days <= expiryWarnDays && days > expiryUrgentDays &&
		rec.Trial14 != st.LID:
		rec.Trial14 = st.LID
	default:
		return "", rec
	}
	return notification.MarkerNotify + " Your kwatch Pro license ends in " +
		fmt.Sprintf("%d day(s)", days) + " (" +
		st.ExpiresAt.UTC().Format("2006-01-02") + "). Renew it at " +
		renewURL, rec
}
