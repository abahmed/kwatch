package release

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Kinds are the workloads whose rollouts are releases.
var Kinds = []inventory.Kind{
	kube.KindDeployment, kube.KindStatefulSet, kube.KindDaemonSet,
}

// View is what one look at the releases reads.
type View struct {
	Model     inventory.HistoryReader
	Incidents []incident.Incident
	// InScope reports a namespace kwatch watches; nil watches all.
	InScope func(namespace string) bool
	// Own is kwatch's own namespace and Deployment: its rollouts are
	// kwatch restarting, not a release to judge.
	OwnNamespace, OwnWorkload string
	// Detail adds the health comparison and the error line (Pro).
	Detail bool
}

// LateAfter is how long after it happened a release first seen counts as
// seen after the fact: kwatch did not watch it roll out.
const LateAfter = 30 * time.Minute

// Observe finds the releases in v's change journal that b does not hold
// yet and judges every release that has no final verdict. It reports
// whether b changed.
func Observe(b *Book, v View, now time.Time) bool {
	changed := b.prune(now)
	for _, kind := range Kinds {
		for _, id := range v.Model.Entities(kind) {
			if !v.watched(id) {
				continue
			}
			for _, c := range v.Model.Changes(id, now.Add(-KeepFor)) {
				if !isRelease(c) || b.find(recordID(id, c.At)) != nil ||
					b.tooOld(c.At) {
					continue
				}
				r := newRecord(id, c)
				if now.Sub(c.At) > LateAfter {
					r.Verdict, r.Final = Unknown, true
				}
				b.add(r)
				changed = true
			}
		}
	}
	for i := range b.Records {
		r := &b.Records[i]
		if r.Final {
			continue
		}
		before := *r
		judge(r, v, b.newer(*r), now)
		changed = changed || !sameJudgement(before, *r)
	}
	return changed
}

// watched reports a workload kwatch judges releases of.
func (v View) watched(id inventory.EntityID) bool {
	if v.InScope != nil && !v.InScope(id.Namespace) {
		return false
	}
	return v.OwnWorkload == "" || id.Namespace != v.OwnNamespace ||
		id.Name != v.OwnWorkload
}

// newer is the start of the next release of r's workload, or zero.
func (b *Book) newer(r Record) time.Time {
	var next time.Time
	for _, o := range b.Records {
		if o.Kind == r.Kind && o.Namespace == r.Namespace &&
			o.Name == r.Name && o.At.After(r.At) &&
			(next.IsZero() || o.At.Before(next)) {
			next = o.At
		}
	}
	return next
}

// isRelease reports a change of the pod template: a rollout. A replica,
// pause or label change rolls nothing out.
func isRelease(c inventory.Change) bool {
	if c.Created || c.Deleted {
		return false
	}
	for _, f := range c.Fields {
		if templatePath(f.Path) {
			return true
		}
	}
	return false
}

// workloadPaths are the changes of a workload that leave its pods alone.
var workloadPaths = []string{"spec.replicas", "spec.paused",
	"spec.suspend", "metadata."}

func templatePath(path string) bool {
	for _, p := range workloadPaths {
		if strings.HasPrefix(path, p) {
			return false
		}
	}
	return path != ""
}

func recordID(id inventory.EntityID, at time.Time) string {
	return string(id.Kind) + "/" + id.Namespace + "/" + id.Name + "@" +
		strconv.FormatInt(at.Unix(), 10)
}

// newRecord is the release c made of the workload id.
func newRecord(id inventory.EntityID, c inventory.Change) Record {
	r := Record{ID: recordID(id, c.At), Kind: string(id.Kind),
		Namespace: id.Namespace, Name: id.Name, At: c.At,
		Actor: c.Actor, App: c.App, Cause: c.Cause, Verdict: Rolling}
	seen := map[string]bool{}
	for _, f := range c.Fields {
		if container, ok := imagePath(f.Path); ok {
			r.Images = append(r.Images, Image{Container: container,
				Before: f.Before, After: f.After})
			continue
		}
		if what := changedWhat(f.Path); what != "" && !seen[what] {
			seen[what] = true
			r.Changed = append(r.Changed, what)
		}
	}
	sort.Strings(r.Changed)
	return r
}

// imagePath reports "containers[name].image" and its container name.
func imagePath(path string) (string, bool) {
	body, ok := strings.CutPrefix(path, "containers[")
	if !ok {
		return "", false
	}
	name, rest, ok := strings.Cut(body, "]")
	return name, ok && rest == ".image"
}

// changedWhat names what a field change of the template changed.
func changedWhat(path string) string {
	switch {
	case !templatePath(path):
		return ""
	case strings.Contains(path, ".resources"):
		return "resources"
	case strings.Contains(path, ".env"):
		return "environment"
	case strings.Contains(path, "Probe") || strings.Contains(path, "probe"):
		return "probes"
	case strings.HasPrefix(path, "template.references"):
		return "config"
	case strings.HasPrefix(path, "template.restartedAt"):
		return "restart"
	case strings.Contains(path, "volume"):
		return "volumes"
	}
	return "pod template"
}
