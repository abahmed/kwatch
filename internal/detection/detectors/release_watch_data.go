package detectors

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// release is a Deployment's newest revision and what it is compared with.
type release struct {
	revision, previous   string
	image, previousImage string
	pods                 []inventory.Entity
	podContainers        [][]inventory.Entity
	revisionAge          time.Duration
	// born is when the newest revision began (see revisionBorn).
	born time.Time
	// edits are what the new revision changed in the pod template.
	edits []inventory.FieldChange
}

// started is when the revision began: its ReplicaSet's creation, or
// the revision change kwatch saw on it (a rollback reuses an old
// ReplicaSet). Only when neither is known is it the revision's first
// pod: pods that a node rotation or an eviction recreated are new pods
// of an old revision, not a rollout.
func (r release) started() time.Time {
	if !r.born.IsZero() {
		return r.born
	}
	var first time.Time
	for _, pod := range r.pods {
		created := timestamp(pod, kube.AttrCreated)
		if !created.IsZero() && (first.IsZero() || created.Before(first)) {
			first = created
		}
	}
	return first
}

// age is the whole minutes the revision has run, for the summary.
func (r release) age() string {
	return strconv.Itoa(int(r.revisionAge.Minutes()))
}

func (r release) containers() []inventory.Entity {
	var out []inventory.Entity
	for _, list := range r.podContainers {
		out = append(out, list...)
	}
	return out
}

// lastRestart is when any container of the revision last restarted.
func (r release) lastRestart() time.Time {
	var last time.Time
	for _, c := range r.containers() {
		if at := timestamp(c, kube.AttrLastFinished); at.After(last) {
			last = at
		}
	}
	return last
}

// restartRate returns the restarts of the revision's containers and the
// pod-hours they ran.
func (r release) restartRate(now time.Time) (int, float64) {
	restarts := 0
	for _, c := range r.containers() {
		count, _ := number(c, kube.AttrRestarts)
		restarts += int(count)
	}
	hours := 0.0
	for _, pod := range r.pods {
		age := now.Sub(timestamp(pod, kube.AttrCreated))
		hours += max(age, releaseMinPodAge).Hours()
	}
	return restarts, hours
}

// releaseChangeLead is how long before its first pod a rollout's
// template change may have been recorded.
const releaseChangeLead = 10 * time.Minute

type revisionSet struct {
	id       inventory.EntityID
	revision int
	entity   inventory.Entity
}

// latestRelease finds the Deployment's newest ReplicaSet and the one
// before it. A Deployment with a single known revision is a first
// deploy: there is nothing to compare with.
func latestRelease(
	model inventory.Reader, deployment inventory.EntityID,
) (release, bool) {
	var sets []revisionSet
	for _, id := range model.Related(deployment, inventory.OwnedBy,
		inventory.Incoming) {
		rs, ok := model.Entity(id)
		if !ok || id.Kind != kube.KindReplicaSet {
			continue
		}
		n, err := strconv.Atoi(text(rs, kube.AttrRevision))
		if err == nil && n > 0 {
			sets = append(sets, revisionSet{id, n, rs})
		}
	}
	if len(sets) < 2 {
		return release{}, false
	}
	sort.Slice(sets, func(i, j int) bool {
		return sets[i].revision > sets[j].revision
	})
	newest, older := sets[0], sets[1]
	out := release{
		revision: strconv.Itoa(newest.revision),
		previous: strconv.Itoa(older.revision),
		born:     revisionBorn(newest.entity),
	}
	for _, id := range model.Related(newest.id, inventory.OwnedBy,
		inventory.Incoming) {
		if pod, ok := model.Entity(id); ok && id.Kind == kube.KindPod &&
			!podFinished(pod) {
			out.pods = append(out.pods, pod)
			out.podContainers = append(out.podContainers,
				podContainers(model, id))
		}
	}
	since := out.started().Add(-releaseChangeLead)
	out.previousImage, out.image = imageChange(model, deployment, since)
	out.edits = inventory.RevisionDiff(model.Changes(deployment, since))
	return out, true
}

// revisionBorn is when a ReplicaSet became the newest revision: when it
// was created, or later when kwatch saw its revision number change while
// it watched (a rollback). Zero when its creation is not known.
func revisionBorn(rs inventory.Entity) time.Time {
	born := timestamp(rs, kube.AttrCreated)
	if born.IsZero() {
		return born
	}
	if at := valueSince(rs, kube.AttrRevision); at.After(rs.FirstSeen) {
		born = latest(born, at)
	}
	return born
}

func podContainers(
	model inventory.Reader, pod inventory.EntityID,
) []inventory.Entity {
	var out []inventory.Entity
	for _, id := range model.Related(pod, inventory.PartOf,
		inventory.Incoming) {
		if c, ok := model.Entity(id); ok {
			out = append(out, c)
		}
	}
	return out
}

// imageChange returns the image the Deployment's latest template change
// replaced and the image it set, or empty strings when the rollout did
// not change an image (or kwatch did not see it).
func imageChange(
	model inventory.Reader, deployment inventory.EntityID, since time.Time,
) (before, after string) {
	changes := model.Changes(deployment, since)
	for i := len(changes) - 1; i >= 0; i-- {
		for _, field := range changes[i].Fields {
			if strings.HasSuffix(field.Path, "].image") {
				return field.Before, field.After
			}
		}
	}
	return "", ""
}
