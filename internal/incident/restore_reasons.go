package incident

import (
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// A restart rebuilds the model kind by kind. Until every kind a detector
// reads has synced, a restored incident can be described by a partial
// finding: an autoscaler whose target Deployment is not listed yet raises
// FailedGetScale (notify) instead of HPATargetMissing (digest). Ratcheting
// that tier and telling it as a material change would alert about an
// incident that has not changed, and revise it back at the end of the
// grace. So while the reasons saved with the record are not all back
// inside the restore grace, the tier is provisional: it follows the
// members (never below what people were told), and a rise is held. It
// is told when the saved reasons are back and the rise still holds, or
// when the grace ends with the rise still there. A page is held the same
// way: before the saved reasons are back the evidence for it is the
// same partial view, and the hold lasts at most the grace.

// memberReasons are the sorted reasons of the failing members of p.
func memberReasons(p *Incident) []string {
	var out []string
	for key, s := range p.Members {
		if !s.Advisory {
			out = append(out, key.Reason)
		}
	}
	sort.Strings(out)
	return slices.Compact(out)
}

// awaitedReasons are the reasons a restored live incident waits for: the
// saved member reasons, or the root reasons of an older record.
func awaitedReasons(r Record) []string {
	switch {
	case r.State == Resolved || r.State == Settling || r.Held:
		return nil
	case len(r.Reasons) > 0:
		return slices.Clone(r.Reasons)
	}
	return slices.Clone(r.RootReasons)
}

// provisional reports a restored incident inside the restore grace whose
// saved reasons are not all back yet: its tier rests on a partial view.
func (m *Manager) provisional(p *Incident, now time.Time) bool {
	return len(p.awaited) > 0 && m.inGrace(p, now)
}

// provisionalTier is the tier of a provisional incident: it follows the
// computed tier but never falls below the tier people were told. Once
// the saved reasons are all back, the incident stops being provisional
// and its tier ratchets as usual from the one returned here.
func (m *Manager) provisionalTier(p *Incident, computed Tier) Tier {
	if reasonsBack(p) {
		p.awaited = nil
	}
	return max(computed, toldTier(p))
}

// reasonsBack reports whether every awaited reason has a failing member.
func reasonsBack(p *Incident) bool {
	present := make(map[string]bool, len(p.Members))
	for key, s := range p.Members {
		if !s.Advisory {
			present[key.Reason] = true
		}
	}
	for _, reason := range p.awaited {
		if !present[reason] {
			return false
		}
	}
	return true
}

// provisionalDeadline wakes a provisional incident at the end of the
// grace, when a held rise is judged again.
func (m *Manager) provisionalDeadline(
	p *Incident, now time.Time,
) (time.Time, bool) {
	return m.restoreGrace, m.provisional(p, now)
}

// maxSavedMembers bounds Record.Members: a storm incident needs only a
// few of its entities to be found again after a restart.
const maxSavedMembers = 32

// memberEntities are the sorted entities of the failing members of p,
// at most maxSavedMembers of them.
func memberEntities(p *Incident) []inventory.EntityID {
	var out []inventory.EntityID
	for key, s := range p.Members {
		if !s.Advisory {
			out = append(out, key.Entity)
		}
	}
	slices.SortFunc(out, func(a, b inventory.EntityID) int {
		return strings.Compare(a.String(), b.String())
	})
	out = slices.Compact(out)
	return out[:min(len(out), maxSavedMembers)]
}

// savedReasons are the reasons a record keeps. A restored incident
// that has no live member yet keeps what it still waits for, so a
// second restart inside the restore grace does not forget it.
func savedReasons(p *Incident) []string {
	if len(p.Members) == 0 && p.restored {
		return slices.Clone(p.awaited)
	}
	return memberReasons(p)
}

// savedMembers are the member entities a record keeps. A restored
// incident that has no live member yet keeps its former members, so
// a second restart inside the restore grace can still hand it over.
func savedMembers(p *Incident) []inventory.EntityID {
	if len(p.Members) == 0 && p.restored {
		return slices.Clone(p.formerMembers)
	}
	return memberEntities(p)
}

// tierUnsettled reports a restored incident whose tier still moves with
// a partial view: the sources are not all listed, or the restore grace
// is not over. A target not listed yet reads as missing, a member back
// before its sibling reads alone, and the tier flips up and down within
// seconds of the start. Such moves are neither logged nor recorded in
// the journal; where the tier stands once the grace is over is.
func (m *Manager) tierUnsettled(p *Incident, now time.Time) bool {
	return p.restored && p.State != Resolved && m.inGrace(p, now)
}
