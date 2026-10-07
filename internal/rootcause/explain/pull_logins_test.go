package explain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

const loginRegistry = "registry.example.com"

// loginFixture is n workloads whose pods pull from loginRegistry with
// the named Secrets and are refused with a 401.
func loginFixture(f *fixture, n int, secrets string) inventory.EntityID {
	var first inventory.EntityID
	for i := 0; i < n; i++ {
		name := string(rune('a'+i)) + "-app"
		pod := f.workload("ci", name, 1)[0]
		image := loginRegistry + "/ci/" + name + ":1"
		f.relate(containerOf(pod), inventory.Pulls,
			inventory.CoreID(kube.KindImage, "", image))
		attrs := map[string]inventory.Value{}
		if secrets != "" {
			attrs[kube.AttrPullSecrets] = inventory.Text(secrets)
		}
		f.observe(pod, attrs)
		f.noteRefused(pod, "Failed to pull image: failed to authorize: "+
			"401 Unauthorized", "unauthorized: authentication required")
		f.fail(containerOf(pod), "ImagePull", failingH, 2,
			"Back-off pulling image "+image)
		if first.IsZero() {
			first = containerOf(pod)
		}
	}
	return first
}

func loginRecord(t *testing.T, f *fixture, effect inventory.EntityID,
) rootcause.CauseRecord {
	t.Helper()
	cause := requireCause(t, f.explain(), effect, "registry//"+loginRegistry)
	return f.snapshot().Record(cause)
}

func TestCauseRecordNamesTheLoginAndTheRefusal(t *testing.T) {
	f := newFixture(t)
	changed := t0.Add(-92 * 24 * time.Hour)
	f.observe(inventory.CoreID(kube.KindSecret, "ci", "regcred"),
		map[string]inventory.Value{
			kube.AttrSecretType: inventory.Text(
				"kubernetes.io/dockerconfigjson"),
			kube.AttrChanged: inventory.Time(changed),
		})
	record := loginRecord(t, f, loginFixture(f, 4, "regcred"))
	require.Len(t, record.Logins, 1)
	login := record.Logins[0]
	assert.Equal(t, rootcause.LoginFound, login.State)
	assert.Equal(t, "regcred", login.Secret)
	assert.Equal(t, "ci", login.Namespace)
	assert.Equal(t, "kubernetes.io/dockerconfigjson", login.Type)
	assert.True(t, changed.Equal(login.Changed))
	assert.Equal(t, "unauthorized: authentication required",
		record.Refused)
}

func TestCauseRecordTellsNoneAndUnwatchedApart(t *testing.T) {
	tests := []struct {
		name, secrets, state string
		unwatched            bool
	}{
		{"none named", "", rootcause.LoginNone, false},
		{"secrets not listed", "regcred", rootcause.LoginUnwatched, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			effect := loginFixture(f, 4, tt.secrets)
			snapshot := f.snapshot()
			if tt.unwatched {
				snapshot.Synced = func(k inventory.Kind) bool {
					return k != kube.KindSecret
				}
			}
			c := requireCause(t, Explain(snapshot), effect,
				"registry//"+loginRegistry)
			record := snapshot.Record(c)
			require.Len(t, record.Logins, 1)
			assert.Equal(t, tt.state, record.Logins[0].State)
		})
	}
}

// TestMissingPullSecretIsNotTheRegistrysFault: pods that name a pull
// Secret that does not exist pull anonymously; the registry is not
// blamed for refusing them.
func TestMissingPullSecretIsNotTheRegistrysFault(t *testing.T) {
	f := newFixture(t)
	effect := loginFixture(f, 4, "regcred")
	v := newView(f.snapshot())
	registry := inventory.CoreID(kube.KindRegistry, "", loginRegistry)
	assert.Empty(t, v.virtualModes(registry, effect, LinkPulls))
}

func TestOtherRegistryCausesCarryNoLogins(t *testing.T) {
	f := newFixture(t)
	first := registryPullFixture(f, 4, detailedPull)
	cause := requireCause(t, f.explain(), first,
		"registry//"+unreachableRegistry)
	record := f.snapshot().Record(cause)
	assert.Empty(t, record.Logins)
	assert.Empty(t, record.Refused)
}

// TestCauseRecordMarksAMissingSecretNextToAPresentOne: a pod that names
// two pull Secrets, one present and one absent, still pulls with the
// present one, so the registry is blamed and the absent one is said.
func TestCauseRecordMarksAMissingSecretNextToAPresentOne(t *testing.T) {
	f := newFixture(t)
	f.observe(inventory.CoreID(kube.KindSecret, "ci", "backup"),
		map[string]inventory.Value{
			kube.AttrSecretType: inventory.Text(
				"kubernetes.io/dockerconfigjson")})
	record := loginRecord(t, f, loginFixture(f, 4, "regcred,backup"))
	require.Len(t, record.Logins, 2)
	assert.Equal(t, "backup", record.Logins[0].Secret)
	assert.Equal(t, rootcause.LoginFound, record.Logins[0].State)
	assert.Equal(t, "regcred", record.Logins[1].Secret)
	assert.Equal(t, rootcause.LoginMissing, record.Logins[1].State)
}

// noteRefused records a failed-pull event whose registry answer is kept
// apart from the cut message.
func (f *fixture) noteRefused(id inventory.EntityID, message, refusal string) {
	f.apply(inventory.Observation{Kind: inventory.Noted, Entity: id,
		Note: inventory.Note{At: t0.Add(time.Minute), Source: "kubelet",
			Reason: "Failed", Message: message, Warning: true,
			Refusal: refusal}})
}

// TestMissingPullSecretIsTheCauseOfTheFailedPulls: pods that name a pull
// Secret that does not exist fail to pull; the Secret is blamed, not
// the registry that refuses their anonymous pulls.
func TestMissingPullSecretIsTheCauseOfTheFailedPulls(t *testing.T) {
	f := newFixture(t)
	effect := loginFixture(f, 3, "regcred")
	secret := inventory.CoreID(kube.KindSecret, "ci", "regcred")
	for _, pod := range f.model.Entities(kube.KindPod) {
		f.relate(pod, inventory.References, secret)
	}
	requireCause(t, f.explain(), effect, "secret/ci/regcred")
}
