package explain

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const missingKeyText = "couldn't find key db-password in Secret " +
	"shop/db-creds"

// keylessCase starts two new pods of api at minute 120 that the kubelet
// cannot configure, with text as its error. It returns the Secret they
// read and one failing container.
func keylessCase(f *fixture, text string) (secret, effect inventory.EntityID) {
	pods := f.workload("shop", "api", 2)
	secret = usesSecret(f, "db-creds", pods...)
	for _, pod := range pods {
		f.observe(pod, map[string]inventory.Value{
			kube.AttrCreated: inventory.Time(t0.Add(2 * time.Hour))})
		f.fail(containerOf(pod), detection.ModeCreateErrorConfig,
			failingH, 120, text)
	}
	f.now = t0.Add(125 * time.Minute)
	return secret, containerOf(pods[0])
}

// The kubelet naming a Secret as lacking a key is a cause by itself:
// no edit of the Secret is on record.
func TestSecretNamedWithAMissingKeyIsTheCauseWithoutAChange(t *testing.T) {
	f := newFixture(t)
	secret, effect := keylessCase(f, missingKeyText)
	c := requireCause(t, f.explain(), effect, secret.String())
	if c.Mode != ModeKeyMissing {
		t.Fatalf("mode = %q, want %q", c.Mode, ModeKeyMissing)
	}
}

// Another object's missing key, or a config error that names no object,
// does not blame this Secret.
func TestMissingKeyBlamesOnlyTheObjectNamed(t *testing.T) {
	for name, text := range map[string]string{
		"another secret": "couldn't find key db-password in Secret " +
			"shop/other-creds",
		"same name in another namespace": "couldn't find key " +
			"db-password in Secret billing/db-creds",
		"a config map of that name": "couldn't find key db-password " +
			"in ConfigMap shop/db-creds",
		"no object named": "container has runAsNonRoot and image " +
			"will run as root",
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			secret, _ := keylessCase(f, text)
			if e := f.explain(); blames(e, secret) {
				t.Fatalf("secret blamed: %v", causes(e))
			}
		})
	}
}

// An edit hours before the pods that first read it started is attached
// as the change: a key read into the environment is resolved when a
// container starts, so the pod's creation is when the edit first
// mattered.
func TestEditReadAtStartHoursLaterIsBlamed(t *testing.T) {
	f := newFixture(t)
	secret, effect := keylessCase(f, missingKeyText)
	f.change(secret, 2, "data.db-password")
	c := requireCause(t, f.explain(), effect, secret.String())
	if !c.Mode.Within(ModeChanged) || len(c.Changes) != 1 {
		t.Fatalf("mode %q with changes %+v, want the edit", c.Mode,
			c.Changes)
	}
}

// The exposure needs the kubelet to name the object: pods that crash on
// their own hours after an edit do not reach back to it.
func TestOldEditIsNotBlamedForACrashThatDoesNotNameIt(t *testing.T) {
	f := newFixture(t)
	pods := f.workload("shop", "api", 2)
	secret := usesSecret(f, "db-creds", pods...)
	for _, pod := range pods {
		f.observe(pod, map[string]inventory.Value{
			kube.AttrCreated: inventory.Time(t0.Add(2 * time.Hour))})
		f.fail(containerOf(pod), "CrashLoop", failingH, 120,
			"panic: index out of range")
	}
	f.change(secret, 2, "data.db-replica")
	f.now = t0.Add(125 * time.Minute)
	if e := f.explain(); blames(e, secret) {
		t.Fatalf("an edit two hours old blamed for a panic: %v",
			causes(e))
	}
}
