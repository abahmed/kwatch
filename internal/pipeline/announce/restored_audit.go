package announce

import (
	"maps"
	"slices"
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
)

// MaxRestoredAuditItems bounds the incidents the audit entry of the
// restored-incidents summary names; Opened says how many there were.
const MaxRestoredAuditItems = 50

// maxItemReasons bounds the member reasons one audit item names.
const maxItemReasons = 5

// restoredListing summarises the restored incidents for the audit log,
// so the summary entry says which incidents it covers: each as
// "id: root tier=T reasons=A,B", the first MaxRestoredAuditItems of them.
func restoredListing(decisions []incident.Decision) *notification.Listed {
	listed := &notification.Listed{Opened: len(decisions)}
	for _, d := range decisions[:min(len(decisions),
		MaxRestoredAuditItems)] {
		p := d.Incident
		listed.Items = append(listed.Items, p.ID+": "+p.Root.String()+
			" tier="+p.Tier.String()+" reasons="+memberReasons(p))
	}
	return listed
}

// memberReasons names the distinct reasons of the incident's findings,
// sorted, at most maxItemReasons of them.
func memberReasons(p incident.Incident) string {
	seen := map[string]bool{}
	for key := range maps.Keys(p.Members) {
		seen[key.Reason] = true
	}
	names := slices.Sorted(maps.Keys(seen))
	if len(names) > maxItemReasons {
		names = append(names[:maxItemReasons], "...")
	}
	return strings.Join(names, ",")
}

// restoredTotals is the key-value list of the restore log line: how
// many incidents came back and how many sit in each tier.
func restoredTotals(decisions []incident.Decision) []any {
	tiers := map[incident.Tier]int{}
	for _, d := range decisions {
		tiers[d.Incident.Tier]++
	}
	out := []any{"incidents", len(decisions)}
	for _, tier := range []incident.Tier{incident.Page, incident.Notify,
		incident.Digest, incident.Silent} {
		out = append(out, tier.String(), tiers[tier])
	}
	return out
}
