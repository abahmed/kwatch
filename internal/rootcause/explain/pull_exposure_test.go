package explain

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// rotatedSecretCase is a new pod of ledger whose pull is refused with a
// 401 while the pull Secret it names was edited hours before. It returns
// the failing container and the Secret.
func rotatedSecretCase(
	f *fixture, hoursBefore int, answer string,
) (effect, secret inventory.EntityID) {
	pod := f.workload("payments", "ledger", 1)[0]
	secret = inventory.CoreID(kube.KindSecret, "payments", "pull-creds")
	f.add(secret)
	f.relate(pod, inventory.References, secret)
	f.observe(pod, map[string]inventory.Value{
		kube.AttrPullSecrets: inventory.Text("pull-creds"),
		kube.AttrCreated:     inventory.Time(t0.Add(time.Minute))})
	f.noteRefused(pod, "Failed to pull image: "+answer, "")
	f.fail(containerOf(pod), "ImagePull", failingH, 3,
		"Back-off pulling image registry.example.com/payments/ledger:3")
	f.change(secret, -hoursBefore*60, "data..dockerconfigjson")
	return containerOf(pod), secret
}

const unauthorized = "failed to authorize: failed to fetch anonymous " +
	"token: unexpected status: 401 Unauthorized"

// A pull Secret edited hours before is named when a new pod's pull is
// refused with an authentication error: running pods never pull again.
func TestPullSecretChangedHoursBeforeIsTheCause(t *testing.T) {
	f := newFixture(t)
	effect, secret := rotatedSecretCase(f, 3, unauthorized)

	requireCause(t, f.explain(), effect, secret.String())
}

// Only an authentication answer reaches back: another failed pull does
// not blame an old edit.
func TestPullSecretChangeOutsideWindowNeedsAnAuthRefusal(t *testing.T) {
	f := newFixture(t)
	effect, secret := rotatedSecretCase(f, 3,
		"dial tcp 10.0.0.1:443: i/o timeout")

	if c, ok := f.explain().CauseOf(effect); ok && c.Root == secret {
		t.Fatalf("an old edit blamed for a timeout: %+v", c)
	}
}

// The window is bounded: an edit older than the exposure window is not
// blamed.
func TestPullSecretChangeBeyondExposureIsNotBlamed(t *testing.T) {
	f := newFixture(t)
	effect, secret := rotatedSecretCase(f, 30, unauthorized)

	if c, ok := f.explain().CauseOf(effect); ok && c.Root == secret {
		t.Fatalf("an edit 30 hours old blamed: %+v", c)
	}
}
