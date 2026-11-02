package dashboard

import (
	"cmp"
	"slices"
	"strings"
	"unicode"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
)

// InsightCause is a kind of failure among the issues kwatch holds (the
// last week): Mode is the failure family the list filters on (?mode=),
// Label its plain name, Count the issues /issues?status=all&mode=
// lists and Active how many of them are still going on.
type InsightCause struct {
	Mode   string `json:"mode"`
	Label  string `json:"label"`
	Count  int    `json:"count"`
	Active int    `json:"active"`
}

// modeLabels name the common failure families in plain words; any
// other family is its name split into words ("RolloutStuck" is
// "Rollout stuck").
var modeLabels = map[string]string{
	"ImagePull":        "Image pull failures",
	"CrashLoop":        "Crash loops",
	"OOMKilled":        "Out of memory",
	"NotReady":         "Not ready",
	"Unschedulable":    "Can't be scheduled",
	"Pending":          "Stuck pending",
	"CreateError":      "Container can't start",
	"Probe":            "Failing health checks",
	"Evicted":          "Evicted",
	"JobFailed":        "Failed jobs",
	"Node":             "Node problems",
	"Condition":        "Failing conditions",
	"MemoryPressure":   "Node memory pressure",
	"DiskPressure":     "Node disk pressure",
	"Restarting":       "Restarting",
	"Error":            "Exited with an error",
	"Exit":             "Exited with an error",
	"RolloutStuck":     "Stuck rollouts",
	"Unavailable":      "Not available",
	"DeadlineExceeded": "Took too long",
}

// modeFamily is the family the list's mode filter and the causes card
// group an incident by; "" when it has no mode.
func modeFamily(p incident.Incident) string {
	return string(p.Mode.Family())
}

// modeLabel is the plain name of a failure family.
func modeLabel(family string) string {
	if l, ok := modeLabels[family]; ok {
		return l
	}
	var b strings.Builder
	for i, r := range family {
		if i > 0 && unicode.IsUpper(r) {
			b.WriteByte(' ')
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}

// validMode reports whether a ?mode= value can name a family: a short
// word of letters and digits.
func validMode(m string) bool {
	if m == "" || len(m) > 64 {
		return false
	}
	for _, r := range m {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return detection.Mode(m).Family() == detection.Mode(m)
}

// topCauses counts the issues held by failure family, as the list
// shows them (quiet issues still active left out), most first.
func topCauses(items []incident.Incident) []InsightCause {
	byMode := map[string]*InsightCause{}
	for _, p := range items {
		family := modeFamily(p)
		if family == "" ||
			(p.Tier == incident.Silent && p.State != incident.Resolved) {
			continue
		}
		c := byMode[family]
		if c == nil {
			c = &InsightCause{Mode: family, Label: modeLabel(family)}
			byMode[family] = c
		}
		c.Count++
		if p.State != incident.Resolved {
			c.Active++
		}
	}
	out := make([]InsightCause, 0, len(byMode))
	for _, c := range byMode {
		out = append(out, *c)
	}
	slices.SortFunc(out, func(a, b InsightCause) int {
		return cmp.Or(b.Count-a.Count, cmp.Compare(a.Mode, b.Mode))
	})
	return out[:min(len(out), insightTop)]
}

// InsightNamespaces is the namespace map: one tile per namespace with
// active issues, More counting the namespaces past the bound.
type InsightNamespaces struct {
	Tiles []NamespaceTile `json:"tiles"`
	More  int             `json:"more"`
}

// NamespaceTile is a namespace, the most urgent severity of its active
// issues and how many it has.
type NamespaceTile struct {
	Namespace string `json:"namespace"`
	Severity  string `json:"severity"`
	Count     int    `json:"count"`
}

// namespaceMap tiles the namespaces of the active issues, most urgent
// first, then the most issues, then A to Z. Cluster-wide issues have
// no tile.
func namespaceMap(live []incident.Incident) InsightNamespaces {
	worst := map[string]incident.Tier{}
	count := map[string]int{}
	for _, p := range live {
		ns := p.Root.Namespace
		if ns == "" {
			continue
		}
		if t, ok := worst[ns]; !ok || p.Tier > t {
			worst[ns] = p.Tier
		}
		count[ns]++
	}
	tiles := make([]NamespaceTile, 0, len(count))
	for ns, n := range count {
		tiles = append(tiles, NamespaceTile{Namespace: ns,
			Severity: worst[ns].String(), Count: n})
	}
	slices.SortFunc(tiles, func(a, b NamespaceTile) int {
		return cmp.Or(cmp.Compare(worst[b.Namespace], worst[a.Namespace]),
			b.Count-a.Count, cmp.Compare(a.Namespace, b.Namespace))
	})
	out := InsightNamespaces{More: max(0, len(tiles)-maxInsightTiles)}
	out.Tiles = tiles[:min(len(tiles), maxInsightTiles)]
	return out
}
