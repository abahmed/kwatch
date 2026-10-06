package scenarios

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/pipeline/announce"
	"github.com/abahmed/kwatch/internal/replay"
)

// restartGap is how long kwatch is down between the two sessions, inside
// the accepted 1 to 2 minute restart gap.
const restartGap = 90 * time.Second

// TestWarmRestartMidIncidentAnnouncesOnce restarts kwatch while an
// incident is open. The first session announces it and persists its
// state; the second starts from that state, sees the cluster through a
// fresh initial list, as informers deliver it after a restart, and runs
// until the fix. The incident must not be announced again, and it must
// still resolve exactly once in the second session.
//
// A replay cannot stop an engine mid-log, so the restart is two replays:
// the second log starts at the restart, re-lists every live object of
// the first session's cluster with the initial-list flag and re-delivers
// the Warning events the API still holds.
func TestWarmRestartMidIncidentAnnouncesOnce(t *testing.T) {
	store := newPersistingStore()
	first, failing := restartFirstSession()
	firstRun := restartReplay(t, first, store, replay.Options{
		SyncAt: first.Start, Tail: 5 * time.Minute,
	})
	announced := announcedIDs(firstRun)
	if len(announced) == 0 {
		t.Fatal("the first session announced nothing")
	}
	if store.records() == 0 {
		t.Fatal("the first session persisted no incident")
	}

	restartAt := firstRun.End.Add(restartGap)
	second := restartSecondSession(failing, restartAt)
	secondRun := restartReplay(t, second, store, replay.Options{
		SyncAt: restartAt, Tail: 20 * time.Minute,
	})

	reannounced := 0
	resolved := 0
	for _, d := range secondRun.Decisions {
		switch {
		case d.Reason == "startup summary", d.Reason == "roll-up":
			reannounced++
		case d.Action == incident.Announce:
			reannounced++
		case d.Action == incident.Resolve && announced[d.Incident.ID]:
			resolved++
		}
	}
	if reannounced != 0 {
		t.Errorf("re-announcements after a warm restart = %d, want 0: %s",
			reannounced, describeDecisions(secondRun))
	}
	if resolved != 1 {
		t.Errorf("resolves of the restored incident = %d, want 1: %s",
			resolved, describeDecisions(secondRun))
	}
}

// restartFailing is the state the first session hands to the second: its
// cluster and the failing workload.
type restartFailing struct {
	cluster *cluster
	w       *workload
	secret  string
}

// restartFirstSession records a Deployment whose pods reference a Secret
// nobody created, as in the missing-secret scenario.
func restartFirstSession() (replay.Log, restartFailing) {
	c := newCluster(scenarioStart, "")
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	c.after(time.Minute)
	w := c.deployment("billing", "invoicer",
		"registry.example.com/invoicer:1.0", 2)
	name := c.n("stripe-api")
	configTemplate(w, func(spec *corev1.PodSpec) {
		spec.Containers[0].EnvFrom = []corev1.EnvFromSource{{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: name,
				},
			},
		}}
	})
	w.setReady(0)
	c.create(w.objects())
	for i := range 2 {
		c.create(w.pod(i, "n"+itoa(i+1), startedNow,
			waiting("CreateContainerConfigError", restartMessage(name))))
	}
	configWarn(c, w, 0, 2, "Failed", "Error: "+restartMessage(name))
	c.after(4 * time.Minute)
	configWarn(c, w, 0, 2, "Failed", "Error: "+restartMessage(name))
	return c.log(), restartFailing{cluster: c, w: w, secret: name}
}

func restartMessage(secret string) string {
	return "secret \"" + secret + "\" not found"
}

// restartSecondSession re-lists the first session's live objects at
// restartAt, keeps the pods failing for a few minutes, then creates the
// missing Secret and lets the pods start.
func restartSecondSession(f restartFailing, restartAt time.Time) replay.Log {
	c := newCluster(scenarioStart, "")
	c.now = restartAt
	c.list(remaining(f.cluster)...)
	w := f.w
	w.c = c
	message := "Error: " + restartMessage(f.secret)
	configWarn(c, w, 0, 2, "Failed", message)
	c.after(3 * time.Minute)
	configWarn(c, w, 0, 2, "Failed", message)
	c.after(time.Minute)
	c.create(configSecret(c, "billing", "stripe-api", "api-key"))
	c.after(20 * time.Second)
	for i := range 2 {
		c.update(w.pod(i, "n"+itoa(i+1), startedNow))
	}
	w.setReady(2)
	c.update(w.objects())
	log := c.log()
	log.Start = restartAt.UTC()
	return log
}

// restartReplay runs one session with the shared store.
func restartReplay(
	t *testing.T, log replay.Log, store *persistingStore,
	opts replay.Options,
) replay.Result {
	t.Helper()
	deps := newDependencies()
	deps.Store = store
	result, err := replay.Run(context.Background(), log, deps, opts)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func announcedIDs(result replay.Result) map[string]bool {
	out := map[string]bool{}
	for _, d := range result.Decisions {
		if d.Action == incident.Announce {
			out[d.Incident.ID] = true
		}
	}
	return out
}

func describeDecisions(result replay.Result) string {
	out := ""
	for i, d := range result.Decisions {
		out += fmt.Sprintf("\n  %s action=%d id=%s root=%s reason=%q",
			result.Times[i].Format(time.TimeOnly), d.Action, d.Incident.ID,
			d.Incident.Root, d.Reason)
	}
	return out
}

// persistingStore is an incident store that keeps what it is given, so
// a second engine restores what the first one saved, as the state file
// does across a restart.
type persistingStore struct {
	mu           sync.Mutex
	incidents    []incident.Record
	fingerprints map[string]string
	startup      announce.StartupState
	hasStartup   bool
}

func newPersistingStore() *persistingStore {
	return &persistingStore{fingerprints: map[string]string{}}
}

func (s *persistingStore) records() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.incidents)
}

func (s *persistingStore) LoadIncidents() ([]incident.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]incident.Record(nil), s.incidents...), nil
}

func (s *persistingStore) SaveIncidents(records []incident.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.incidents = append([]incident.Record(nil), records...)
	return nil
}

func (s *persistingStore) LoadFingerprints() (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]string, len(s.fingerprints))
	for key, value := range s.fingerprints {
		out[key] = value
	}
	return out, nil
}

// SaveFingerprints keeps the values as text, as the state file reads
// them back.
func (s *persistingStore) SaveFingerprints(values map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fingerprints = make(map[string]string, len(values))
	for key, value := range values {
		s.fingerprints[key] = fmt.Sprint(value)
	}
	return nil
}

func (s *persistingStore) LoadStartup() (announce.StartupState, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startup, s.hasStartup, nil
}

func (s *persistingStore) SaveStartup(state announce.StartupState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startup, s.hasStartup = state, true
	return nil
}
