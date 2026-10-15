package app

import (
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/license"
	"github.com/abahmed/kwatch/internal/notification"
)

// Trial warning thresholds, in whole days left.
const (
	trialWarnDays   = 14
	trialUrgentDays = 3
)

// licenseNotified is what the operator has already been told. It is kept
// in the state file so a restart never repeats a notice.
type licenseNotified struct {
	// Pro is true while Pro is enabled and the operator knows it.
	Pro bool `json:"pro"`
	// Trial14 and Trial3 hold the license the warning was sent for.
	Trial14 string `json:"trial14,omitempty"`
	Trial3  string `json:"trial3,omitempty"`
	// Grace holds the end of the grace window the notice was sent for.
	Grace string `json:"grace,omitempty"`
}

// licenseNotices decides which messages a status change calls for and
// returns the updated record. Each transition yields at most one message:
// Pro enabled, Pro disabled, grace entered, and the two trial warnings.
func licenseNotices(
	rec licenseNotified, st license.Status, now time.Time,
) ([]string, licenseNotified) {
	var msgs []string
	switch {
	case st.State.ProEnabled() && !rec.Pro:
		rec.Pro = true
		msgs = append(msgs, proEnabledNotice(st))
	case !st.State.ProEnabled() && rec.Pro && proLostState(st.State):
		rec.Pro, rec.Grace = false, ""
		msgs = append(msgs, proDisabledNotice(st))
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
	if note, updated := trialNotice(rec, st, now); note != "" {
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

func proEnabledNotice(st license.Status) string {
	text := "kwatch Pro is enabled"
	if st.Plan != "" {
		text += " (" + st.Plan + " plan)"
	}
	return notification.MarkerLow + " " + text + "."
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

// trialNotice returns the trial warning that is due, if any.
func trialNotice(
	rec licenseNotified, st license.Status, now time.Time,
) (string, licenseNotified) {
	if st.Plan != license.PlanTrial || st.State != license.StateActive ||
		st.ExpiresAt.IsZero() {
		return "", rec
	}
	left := st.ExpiresAt.Sub(now)
	if left <= 0 {
		return "", rec
	}
	days := int((left + 24*time.Hour - 1) / (24 * time.Hour))
	switch {
	case days <= trialUrgentDays && rec.Trial3 != st.LID:
		rec.Trial3, rec.Trial14 = st.LID, st.LID
	case days <= trialWarnDays && days > trialUrgentDays &&
		rec.Trial14 != st.LID:
		rec.Trial14 = st.LID
	default:
		return "", rec
	}
	return notification.MarkerNotify + " the kwatch Pro trial ends in " +
		fmt.Sprintf("%d day(s)", days) + " (" +
		st.ExpiresAt.UTC().Format("2006-01-02") + " UTC).", rec
}
