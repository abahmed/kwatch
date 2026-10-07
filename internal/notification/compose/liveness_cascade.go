package compose

import (
	"sort"
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
)

// weightLivenessCascade ranks the restarts with the other facts that
// link a dependent's failure to its cause.
const weightLivenessCascade = 0.66

// maxCascadeNames bounds the dependents named in the sentence.
const maxCascadeNames = 3

// livenessCascadeSentences say that pods of other workloads were
// restarted by a liveness probe that runs their readiness check, after
// the root failed: "db went down at 10:02; 12 pods of api and worker
// were then restarted by their liveness probe (same check as
// readiness), so the restarts are a symptom." They say nothing for
// kills of the root's own pods or of a probe that checks something else.
func livenessCascadeSentences(f caseFacts) []sentence {
	if f.p.Cause == nil {
		return nil
	}
	pods, owners := cascadeKills(f)
	if len(pods) == 0 || len(owners) == 0 {
		return nil
	}
	root := f.p.Root
	text := shortName(root) + " went down"
	if at := cascadeRootSince(f); !at.IsZero() {
		text += " at " + clock(at)
	}
	text += "; " + countPods(len(pods)) + " of " +
		joinAnd(owners, maxCascadeNames) + " " + wasWere(len(pods)) +
		" then restarted by their liveness probe (same check as " +
		"readiness), so the restarts are a symptom."
	return []sentence{{part: partProof, weight: weightLivenessCascade,
		text: text}}
}

// cascadeKills are the pods of other workloads that liveness killed
// with the same check as readiness, and those workloads' names, sorted.
func cascadeKills(f caseFacts) (map[string]bool, []string) {
	pods := map[string]bool{}
	names := map[string]bool{}
	for _, m := range f.members {
		if m.Reason != reasons.LivenessKilled ||
			evidence(m, detection.EvidenceLivenessSameCheck) == "" {
			continue
		}
		owner, ok := ownerIn(f.p.Impact, m.Entity)
		if !ok || owner == f.p.Root {
			continue
		}
		pod, _ := splitContainer(m.Entity.Name)
		pods[m.Entity.Namespace+"/"+pod] = true
		names[shortName(owner)] = true
	}
	var owners []string
	for name := range names {
		owners = append(owners, name)
	}
	sort.Strings(owners)
	return pods, owners
}

// cascadeRootSince is when the root began failing, from its own
// findings or its pods', the earliest of them, else the cause's start.
func cascadeRootSince(f caseFacts) time.Time {
	var since time.Time
	for _, m := range f.members {
		owner, owned := ownerIn([]inventory.EntityID{f.p.Root}, m.Entity)
		if m.Entity != f.p.Root && !(owned && owner == f.p.Root) {
			continue
		}
		if m.Advisory || m.Since.IsZero() {
			continue
		}
		if since.IsZero() || m.Since.Before(since) {
			since = m.Since
		}
	}
	if since.IsZero() {
		since = f.p.Cause.Began
	}
	return since
}

func countPods(n int) string {
	if n == 1 {
		return "1 pod"
	}
	return strconv.Itoa(n) + " pods"
}

func wasWere(n int) string {
	if n == 1 {
		return "was"
	}
	return "were"
}
