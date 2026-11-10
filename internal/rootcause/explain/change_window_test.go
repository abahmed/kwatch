package explain

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// editedThenCrashing edits the Secret of api one minute after t0 and
// crash-loops both pods from minute four on. It returns the Secret and
// one failing container.
func editedThenCrashing(f *fixture) (secret, effect inventory.EntityID) {
	pods := f.workload("shop", "api", 2)
	secret = usesSecret(f, "db-creds", pods...)
	for _, pod := range pods {
		f.fail(containerOf(pod), "CrashLoop", failingH, 4,
			"parsing config: invalid syntax")
	}
	f.change(secret, 1, "data.password")
	return secret, containerOf(pods[0])
}

// The causal window is measured back from when the failure began. A
// crash loop solved again hours later still has the edit made before
// it started as its cause.
func TestChangeStaysTheCauseHoweverLongTheFailureLasts(t *testing.T) {
	for _, after := range []time.Duration{10 * time.Minute,
		31 * time.Minute, 2 * time.Hour, 20 * time.Hour,
		25 * time.Hour, 72 * time.Hour} {
		t.Run(after.String(), func(t *testing.T) {
			f := newFixture(t)
			secret, effect := editedThenCrashing(f)
			f.now = t0.Add(after)
			c := requireCause(t, f.explain(), effect, secret.String())
			if len(c.Changes) != 1 {
				t.Fatalf("the edit is not attached: %+v", c.Changes)
			}
		})
	}
}

// The window still has a start: an edit made longer before the failure
// began than the window is not what broke it.
func TestChangeLongBeforeTheFailureIsNotACause(t *testing.T) {
	f := newFixture(t)
	secret, _ := editedThenCrashing(f)
	f.changes[secret][0].At = t0.Add(-2 * time.Hour)
	f.now = t0.Add(3 * time.Hour)
	if e := f.explain(); blames(e, secret) {
		t.Fatalf("an edit two hours before the failure blamed: %v",
			causes(e))
	}
}

// An old failure beside a new one does not lend it its window: the
// edit that explains the old failure is too far from the new one.
func TestOldFailureDoesNotWidenTheWindowOfANewOne(t *testing.T) {
	f := newFixture(t)
	secret, _ := editedThenCrashing(f)
	late := f.workload("shop", "late", 1)
	f.relate(late[0], inventory.References, secret)
	f.fail(containerOf(late[0]), "CrashLoop", failingH, 180,
		"panic: index out of range")
	f.now = t0.Add(190 * time.Minute)
	e := f.explain()
	requireCause(t, e, containerOf(
		inventory.CoreID(kube.KindPod, "shop", "api-1-0")),
		secret.String())
	if c, ok := e.CauseOf(containerOf(late[0])); ok && c.Root == secret {
		t.Fatalf("an edit three hours before the new failure blamed")
	}
}

// A replica replaced during a long outage fails from its creation on;
// it belongs to the same failing workload, whose onset keeps the edit
// in view.
func TestReplacedReplicaKeepsTheWorkloadsWindow(t *testing.T) {
	f := newFixture(t)
	secret, _ := editedThenCrashing(f)
	rs := inventory.CoreID(kube.KindReplicaSet, "shop", "api-1")
	fresh := inventory.CoreID(kube.KindPod, "shop", "api-1-9")
	f.add(fresh, containerOf(fresh))
	f.relate(fresh, inventory.OwnedBy, rs)
	f.relate(containerOf(fresh), inventory.PartOf, fresh)
	f.relate(fresh, inventory.References, secret)
	f.fail(containerOf(fresh), "CrashLoop", failingH, 90,
		"parsing config: invalid syntax")
	f.now = t0.Add(95 * time.Minute)
	requireCause(t, f.explain(), containerOf(fresh), secret.String())
}

// A replica that has failed another way for a day (Pending) does not
// lend its onset to a crash loop that began minutes ago: an edit made
// eighteen hours ago is not the cause of the crash.
func TestOtherFailureOfTheWorkloadDoesNotWidenTheWindow(t *testing.T) {
	f := newFixture(t)
	pods := f.workload("shop", "api", 3)
	secret := usesSecret(f, "db-creds", pods...)
	f.fail(pods[2], "Unschedulable", failingH, -20*60,
		"0/3 nodes are available: insufficient cpu")
	for _, pod := range pods[:2] {
		f.fail(containerOf(pod), "CrashLoop", failingH, 4,
			"parsing config: invalid syntax")
	}
	f.change(secret, -18*60, "data.password")
	f.now = t0.Add(10 * time.Minute)
	if e := f.explain(); blames(e, secret) {
		t.Fatalf("an edit 18h before a fresh crash loop blamed: %v",
			causes(e))
	}
}

// The history has a deliberate end: a change made more than
// MaxChangeAge ago is not blamed, however long the failure it began
// has lasted.
func TestChangeOlderThanMaxChangeAgeIsNotACause(t *testing.T) {
	f := newFixture(t)
	secret, _ := editedThenCrashing(f)
	f.now = t0.Add(MaxChangeAge + time.Hour)
	if e := f.explain(); blames(e, secret) {
		t.Fatalf("an edit over a week old blamed: %v", causes(e))
	}
}
