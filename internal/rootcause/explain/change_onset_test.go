package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// usesSecret adds a Secret of shop that every pod references.
func usesSecret(
	f *fixture, name string, pods ...inventory.EntityID,
) inventory.EntityID {
	secret := inventory.CoreID(kube.KindSecret, "shop", name)
	f.add(secret)
	for _, pod := range pods {
		f.relate(pod, inventory.References, secret)
	}
	return secret
}

// TestChangeAfterFailureBeganIsNotACause: an edit made after the pods
// began to fail is a fix attempt, not what broke them.
func TestChangeAfterFailureBeganIsNotACause(t *testing.T) {
	f := newFixture(t)
	pods := f.workload("shop", "api", 2)
	secret := usesSecret(f, "db-creds", pods...)
	for _, pod := range pods {
		f.fail(containerOf(pod), "CrashLoop", failingH, 1,
			"parsing config: invalid syntax")
	}
	f.change(secret, 8, "data.password")
	e := f.explain()
	for _, root := range causes(e) {
		if root == secret.String() {
			t.Fatalf("an edit after the failures was blamed: %v",
				causes(e))
		}
	}
	requireCause(t, e, containerOf(pods[0]), "deployment/shop/api")
}

// TestChangeBeforeFailureBeganStaysACause: the same edit made before
// the failures began is still the suspect.
func TestChangeBeforeFailureBeganStaysACause(t *testing.T) {
	f := newFixture(t)
	pods := f.workload("shop", "api", 2)
	secret := usesSecret(f, "db-creds", pods...)
	for _, pod := range pods {
		f.fail(containerOf(pod), "CrashLoop", failingH, 4,
			"parsing config: invalid syntax")
	}
	f.change(secret, 1, "data.password")
	requireCause(t, f.explain(), containerOf(pods[0]), secret.String())
}

// TestChangeCountsAgainstEachEffectsOwnOnset: one edit sits after the
// first pod began to fail and before the second did; it can only be
// the cause of the second.
func TestChangeCountsAgainstEachEffectsOwnOnset(t *testing.T) {
	f := newFixture(t)
	early := f.workload("shop", "early", 1)
	late := f.workload("shop", "late", 1)
	secret := usesSecret(f, "shared-creds", early[0], late[0])
	f.fail(containerOf(early[0]), "CrashLoop", failingH, 1,
		"panic: cannot decode credentials")
	f.fail(containerOf(late[0]), "CrashLoop", failingH, 6,
		"panic: cannot decode credentials")
	f.change(secret, 4, "data.password")
	e := f.explain()
	cause, ok := e.CauseOf(containerOf(late[0]))
	if !ok || cause.Root.String() != secret.String() {
		t.Fatalf("the later failure should be blamed on the edit: %v",
			causes(e))
	}
	for _, covered := range cause.Covers {
		if covered == containerOf(early[0]) {
			t.Fatalf("the edit explains the failure that began before it")
		}
	}
}

// TestNodeChangeAfterFailureStaysACause: a node's change comes with its
// failure, so it is not dated against the pods it hurts.
func TestNodeChangeAfterFailureStaysACause(t *testing.T) {
	f := newFixture(t)
	nodes := f.nodes("zone-a", "n1")
	pods := f.workload("shop", "api", 2, nodes[0])
	for _, pod := range pods {
		f.fail(containerOf(pod), "CrashLoop", failingH, 1, "")
	}
	f.fail(nodes[0], "NotReady", failingH, 5, "")
	f.change(nodes[0], 5, "status")
	if _, ok := f.explain().CauseOf(containerOf(pods[0])); !ok {
		t.Fatal("a node that failed after its pods lost its cause")
	}
}

// TestChangedLaterEntityIsNotReportedHealthy: a Secret edited after
// the failures is neither a cause nor "healthy and unchanged".
func TestChangedLaterEntityIsNotReportedHealthy(t *testing.T) {
	f := newFixture(t)
	pods := f.workload("shop", "api", 2)
	secret := usesSecret(f, "db-creds", pods...)
	for _, pod := range pods {
		f.fail(containerOf(pod), "CrashLoop", failingH, 1,
			"panic: runtime error: index out of range [3]")
	}
	f.change(secret, 8, "data.password")
	for _, area := range f.explain().Areas {
		for _, ids := range area.Trace.Checked {
			for _, id := range ids {
				if id == secret {
					t.Fatalf("an edited Secret was reported unchanged")
				}
			}
		}
	}
}
