package incident

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A randomized walk through the lifecycle: failures come and go, causes
// flip between a pod and its node, kwatch restarts, time jumps. Whatever
// the order, what people are told must stay a sane conversation:
//
//   - an incident is announced once;
//   - an update or a resolve only follows an announcement that is open;
//   - an incident does not resolve twice in a row;
//   - an ID that was given out is never given to another incident.
//
// The seeds are fixed, so a failure names the seed and replays exactly.

const (
	walkSeeds = 120
	walkSteps = 90
	// walkGrace is the restore grace of a restart in the walk.
	walkGrace = 2 * time.Minute
)

// thread is what one incident ID has told so far.
type thread struct {
	announced, open bool
}

// walk is one randomized run and what it has seen.
type walk struct {
	t       *testing.T
	r       *rig
	rng     *rand.Rand
	seed    int64
	now     time.Time
	threads map[string]*thread
	// seen are the IDs the manager ever held; gone, those it dropped.
	seen, gone map[string]bool
	trail      []string
	restarts   int
}

func (w *walk) failf(format string, args ...any) {
	w.t.Helper()
	tail := w.trail[max(0, len(w.trail)-12):]
	w.t.Fatalf("seed %d: %s\nlast steps:\n  %s", w.seed,
		fmt.Sprintf(format, args...), joinLines(tail))
}

func joinLines(lines []string) string {
	out := ""
	for i, line := range lines {
		if i > 0 {
			out += "\n  "
		}
		out += line
	}
	return out
}

func walkFindings() []detection.Finding {
	crash := podSig("web")
	crash.Mode = "CrashLoop"
	down := sig(entity(kube.KindPod, "api"), reasons.CrashLoopBackOff,
		detection.Critical)
	low := sig(entity(kube.KindPod, "batch"), "FailedMount",
		detection.Info)
	return []detection.Finding{crash, down, low, podSig("db"), nodeDown()}
}

// step makes one random move and returns its name.
func (w *walk) step() string {
	findings := walkFindings()
	f := findings[w.rng.Intn(len(findings))]
	switch w.rng.Intn(10) {
	case 0, 1, 2:
		w.r.raise(w.now, f)
		return "raise " + f.Entity.Name
	case 3, 4:
		w.r.clear(w.now, f)
		return "clear " + f.Entity.Name
	case 5:
		return w.flipCause(f)
	case 6:
		w.restart()
		return "restart"
	case 7:
		w.now = w.now.Add([]time.Duration{20 * time.Minute, 3 * time.Hour,
			26 * time.Hour}[w.rng.Intn(3)])
		return "jump"
	}
	w.now = w.now.Add(time.Duration(1+w.rng.Intn(9)) * 10 * time.Second)
	return "wait"
}

// flipCause blames the node for a pod's failure, or stops blaming it,
// and has the failure explained again.
func (w *walk) flipCause(f detection.Finding) string {
	if f.Entity.Kind != kube.KindPod {
		return "flip skipped"
	}
	name := "blame node for " + f.Entity.Name
	if _, blamed := w.r.rule.causes[f.Entity]; blamed {
		delete(w.r.rule.causes, f.Entity)
		name = "stop blaming node for " + f.Entity.Name
	} else {
		w.r.cause(f.Entity, nodeID(), "node n1 is down")
	}
	if active, ok := w.r.active[f.Key()]; ok {
		w.r.raise(w.now, active)
	}
	return name
}

// restart saves the incidents, starts a new manager from them and has
// the detectors raise what still fails, as a restart of kwatch does.
func (w *walk) restart() {
	records := w.r.m.Export()
	// An empty store draws a new nonce, as a real one does at random.
	w.restarts++
	nonce := fmt.Sprintf("%04x", w.restarts)
	w.r.m = NewManager(Config{IDNonce: nonce}, w.r.rule)
	w.r.m.Restore(records, w.now.Add(walkGrace))
	var active []detection.Finding
	for _, f := range w.r.active {
		active = append(active, f)
	}
	w.r.raise(w.now, active...)
}

// hear checks one decision against what its incident told before.
func (w *walk) hear(d Decision) {
	w.t.Helper()
	id := d.Incident.ID
	th := w.threads[id]
	if th == nil {
		th = &thread{}
		w.threads[id] = th
	}
	what := fmt.Sprintf("%v (%s) of %s", d.Action, d.Reason, id)
	w.trail = append(w.trail, "  -> "+what)
	switch {
	case d.Unannounced:
		// A pager-only close of an incident nobody was told about.
	case d.Action == Announce:
		if th.announced {
			w.failf("%s: announced twice", what)
		}
		th.announced, th.open = true, true
	case !th.announced:
		w.failf("%s: never announced", what)
	case d.Action == Resolve:
		if !th.open {
			w.failf("%s: resolved twice in a row", what)
		}
		th.open = false
	case !th.open && d.Reason != ReasonFailingAgain &&
		d.Reason != ReasonFlapping:
		w.failf("%s: update after its resolve", what)
	default:
		th.open = true
	}
}

// census checks that no ID the manager dropped is held again.
func (w *walk) census() {
	w.t.Helper()
	held := map[string]bool{}
	for _, record := range w.r.m.Export() {
		held[record.ID] = true
		if w.gone[record.ID] {
			w.failf("ID %s was given out again", record.ID)
		}
		w.seen[record.ID] = true
	}
	for id := range w.seen {
		if !held[id] {
			w.gone[id] = true
		}
	}
}

func TestLifecycleInvariantsHoldOnRandomWalks(t *testing.T) {
	for seed := int64(1); seed <= walkSeeds; seed++ {
		w := &walk{t: t, r: newRig(t, Config{}), seed: seed,
			rng: rand.New(rand.NewSource(seed)), now: at(0),
			threads: map[string]*thread{},
			seen:    map[string]bool{}, gone: map[string]bool{}}
		w.r.observe(nodeID())
		for i := 0; i < walkSteps; i++ {
			w.trail = append(w.trail, w.step())
			for _, d := range w.r.tick(w.now) {
				w.hear(d)
			}
			w.census()
		}
	}
}

// The walk is only worth its name if it gets everywhere.
func TestLifecycleWalkReachesEveryAction(t *testing.T) {
	seen := map[Action]bool{}
	restarts := 0
	for seed := int64(1); seed <= 20; seed++ {
		w := &walk{t: t, r: newRig(t, Config{}), seed: seed,
			rng: rand.New(rand.NewSource(seed)), now: at(0),
			threads: map[string]*thread{},
			seen:    map[string]bool{}, gone: map[string]bool{}}
		for i := 0; i < walkSteps; i++ {
			if w.step() == "restart" {
				restarts++
			}
			for _, d := range w.r.tick(w.now) {
				seen[d.Action] = true
			}
		}
	}
	if !seen[Announce] || !seen[Update] || !seen[Resolve] || restarts == 0 {
		t.Fatalf("walk too shallow: actions %v, restarts %d", seen, restarts)
	}
}
