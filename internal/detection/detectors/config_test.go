package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestCertificateValid(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	expiry := now.Add(365 * 24 * time.Hour)

	secret := buildStorage(
		kube.KindSecret, "tls", "default", now,
		map[string]inventory.Value{
			kube.AttrCertExpiry: inventory.Time(expiry),
		},
	)

	detector := Certificate{}
	ctx := testDetectorContext(nil, now)

	findings := detector.Detect(ctx, secret)

	assert.Empty(t, findings)
}

func TestCertificateExpiring(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	expiry := now.Add(7 * 24 * time.Hour)

	secret := buildStorage(
		kube.KindSecret, "tls", "default", now,
		map[string]inventory.Value{
			kube.AttrCertExpiry: inventory.Time(expiry),
		},
	)

	detector := Certificate{}
	ctx := testDetectorContext(nil, now)

	findings := detector.Detect(ctx, secret)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.TLSCertExpiringSoon, findings[0].Reason)
	assert.Equal(t, detection.Warning, findings[0].Severity)
	assert.Contains(t, findings[0].Summary, "expires in")
}

func TestCertificateExpired(t *testing.T) {
	m := newTestModel()
	secret := newID(kube.KindSecret, "default", "tls")
	put(m, secret, t0, map[string]inventory.Value{
		kube.AttrCertExpiry: inventory.Time(t0.Add(-24 * time.Hour)),
	})
	ingress := newID(kube.KindIngress, "default", "web")
	put(m, ingress, t0, nil)
	link(m, ingress, inventory.References, secret)

	findings := Certificate{}.Detect(testDetectorContext(m, t0),
		entityOf(m, secret))

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.TLSCertExpired, findings[0].Reason)
	assert.Equal(t, detection.Critical, findings[0].Severity)
	assert.Contains(t, findings[0].Summary, "expired")
}

// Nothing references the Secret, so the expired certificate breaks
// nothing: it is reported, but not as an outage.
func TestCertificateExpiredButUnreferencedIsAWarning(t *testing.T) {
	m := newTestModel()
	secret := newID(kube.KindSecret, "default", "old-tls")
	put(m, secret, t0, map[string]inventory.Value{
		kube.AttrCertExpiry: inventory.Time(t0.Add(-24 * time.Hour)),
	})

	findings := Certificate{}.Detect(testDetectorContext(m, t0),
		entityOf(m, secret))

	require.Len(t, findings, 1)
	assert.Equal(t, detection.Warning, findings[0].Severity)
	assert.Contains(t, findings[0].Summary,
		"nothing in the cluster references it")
}

// A pod that mounts the Secret uses the certificate: the finding is
// critical and does not say nothing references it.
func TestCertificateExpiredMountedByPodIsCritical(t *testing.T) {
	m := newTestModel()
	secret := newID(kube.KindSecret, "default", "tls")
	put(m, secret, t0, map[string]inventory.Value{
		kube.AttrCertExpiry: inventory.Time(t0.Add(-24 * time.Hour)),
	})
	pod := newID(kube.KindPod, "default", "web-0")
	put(m, pod, t0, nil)
	link(m, pod, inventory.References, secret)

	findings := Certificate{}.Detect(testDetectorContext(m, t0),
		entityOf(m, secret))

	require.Len(t, findings, 1)
	assert.Equal(t, detection.Critical, findings[0].Severity)
	assert.NotContains(t, findings[0].Summary, "references it")
	assert.Equal(t, []detection.Evidence{{
		Label: detection.EvidenceUsedBy, Value: "pod/web-0"}},
		findings[0].Evidence)
}

func TestMissingSecret(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	model := newTestModel()
	podID := inventory.EntityID{Kind: kube.KindPod, Namespace: "default",
		Name: "pod"}
	secretID := inventory.EntityID{Kind: kube.KindSecret,
		Namespace: "default", Name: "creds"}

	// Create pod but not the secret
	model.Apply(inventory.Observation{
		Kind:       inventory.Observed,
		Source:     "test",
		At:         now,
		Entity:     podID,
		Attributes: map[string]inventory.Value{},
	})

	// Create reference from pod to secret
	model.Apply(inventory.Observation{
		Kind:     inventory.Related,
		Source:   "test",
		At:       now,
		Entity:   podID,
		Relation: inventory.References,
		Targets:  []inventory.EntityID{secretID},
	})

	entity, _ := model.Entity(podID)
	detector := Missing{}
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.ProjectedSecretMissing, findings[0].Reason)
	assert.Equal(t, detection.Critical, findings[0].Severity)
	assert.Contains(t, findings[0].Summary, "creds")
}

func TestMissingConfigMap(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	model := newTestModel()
	podID := inventory.EntityID{Kind: kube.KindPod, Namespace: "default",
		Name: "pod"}
	configID := inventory.EntityID{Kind: kube.KindConfigMap,
		Namespace: "default", Name: "app-config"}

	model.Apply(inventory.Observation{
		Kind:       inventory.Observed,
		Source:     "test",
		At:         now,
		Entity:     podID,
		Attributes: map[string]inventory.Value{},
	})

	model.Apply(inventory.Observation{
		Kind:     inventory.Related,
		Source:   "test",
		At:       now,
		Entity:   podID,
		Relation: inventory.References,
		Targets:  []inventory.EntityID{configID},
	})

	entity, _ := model.Entity(podID)
	detector := Missing{}
	ctx := testDetectorContext(model, now)

	findings := detector.Detect(ctx, entity)

	require.Len(t, findings, 1)
	assert.Equal(t, reasons.ProjectedConfigMapMissing, findings[0].Reason)
}

func TestMissingSkipsOptionalReference(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	model := newTestModel()
	podID := inventory.EntityID{Kind: kube.KindPod, Namespace: "default",
		Name: "pod"}
	secretID := inventory.EntityID{Kind: kube.KindSecret,
		Namespace: "default", Name: "extra"}
	model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: "test", At: now, Entity: podID,
		Attributes: map[string]inventory.Value{
			kube.AttrOptionalRefs: inventory.Text("secret/extra"),
		},
	})
	model.Apply(inventory.Observation{
		Kind: inventory.Related, Source: "test", At: now, Entity: podID,
		Relation: inventory.References,
		Targets:  []inventory.EntityID{secretID},
	})

	entity, _ := model.Entity(podID)
	findings := Missing{}.Detect(testDetectorContext(model, now), entity)

	assert.Empty(t, findings, "an optional Secret does not break the pod")
}

// podMissingSecret builds a pod with the given attributes that
// references a Secret nobody created.
func podMissingSecret(
	attrs map[string]inventory.Value,
) (*inventory.Model, inventory.Entity) {
	m := newTestModel()
	pod := newID(kube.KindPod, "default", "pod")
	put(m, pod, t0, attrs)
	link(m, pod, inventory.References,
		newID(kube.KindSecret, "default", "creds"))
	return m, entityOf(m, pod)
}

func TestMissingSkipsFinishedAndDeletingPods(t *testing.T) {
	for name, attrs := range map[string]map[string]inventory.Value{
		"succeeded": {kube.AttrPhase: inventory.Text("Succeeded")},
		"failed":    {kube.AttrPhase: inventory.Text("Failed")},
		"deleting":  {kube.AttrDeleting: inventory.Bool(true)},
	} {
		m, pod := podMissingSecret(attrs)

		assert.Empty(t, Missing{}.Detect(testDetectorContext(m, t0), pod),
			name)
	}
}

// A pod that already runs read its Secret at start. The deletion only
// matters at the next restart, so it is a heads-up, not an outage.
func TestMissingDowngradesARunningReadyPod(t *testing.T) {
	m, pod := podMissingSecret(map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Running"),
		kube.AttrReady: inventory.Bool(true),
	})

	got := Missing{}.Detect(testDetectorContext(m, t0), pod)

	require.Len(t, got, 1)
	assert.Equal(t, detection.Warning, got[0].Severity)
	assert.Contains(t, got[0].Summary, "next restart")
}
