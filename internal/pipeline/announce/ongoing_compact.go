package announce

import (
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification/compose"
)

// An ongoing problem is listed in full at most once a day: in the first
// digest of the day that names it, or in a later one when it changed.
// Every other digest says "N more still failing, unchanged (a, b, c +K)"
// instead of repeating the same lines. The audit log still lists them
// all.

// shown is how a problem was last listed in full.
type shown struct {
	// day is the UTC date of the digest, sig what it said (see
	// ongoingSignature).
	day, sig string
}

// ongoingSignature is what a digest says about a problem apart from how
// long it has lasted: whether it flaps, its reason, how many things fail
// and its cause. A change in any of them is news. A problem whose
// findings are all gone is about to resolve: its signature is empty and
// matches whatever it was listed as.
func ongoingSignature(p incident.Incident) string {
	failing := 0
	for _, f := range p.Members {
		if !f.Advisory {
			failing++
		}
	}
	if failing == 0 {
		return ""
	}
	sig := strconv.FormatBool(p.State == incident.Flapping) + "/" +
		ongoingReason(p) + "/" + strconv.Itoa(failing)
	if p.Cause != nil {
		sig += "/" + p.Cause.Root.String() + "/" + p.Cause.Rule + "/" +
			string(p.Cause.Mode)
	}
	return sig
}

func dayOf(now time.Time) string { return now.UTC().Format("2006-01-02") }

// splitOngoing separates the problems to list in full from the ones
// this day's digests already listed and that did not change.
func (c *Collector) splitOngoing(
	items []ongoingItem, now time.Time,
) (changed, unchanged []ongoingItem) {
	for _, item := range items {
		last, ok := c.ongoingShown[item.p.ID]
		sig := ongoingSignature(item.p)
		if ok && last.day == dayOf(now) && (sig == "" || last.sig == sig) {
			unchanged = append(unchanged, item)
			continue
		}
		changed = append(changed, item)
	}
	return changed, unchanged
}

// markShown remembers that a digest listed the problem in full.
func (c *Collector) markShown(p incident.Incident, now time.Time) {
	if c.ongoingShown == nil {
		c.ongoingShown = map[string]shown{}
	}
	c.ongoingShown[p.ID] = shown{day: dayOf(now), sig: ongoingSignature(p)}
}

// ongoingExtras are the digest's ongoing problems: in full, and as the
// summary line.
func ongoingExtras(
	changed, unchanged []ongoingItem,
) (full, same []compose.Ongoing) {
	for _, item := range changed {
		full = append(full, item.Ongoing)
	}
	for _, item := range unchanged {
		same = append(same, item.Ongoing)
	}
	return full, same
}
