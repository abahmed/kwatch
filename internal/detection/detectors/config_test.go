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
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	expiry := now.Add(-24 * time.Hour)

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
	assert.Equal(t, reasons.TLSCertExpired, findings[0].Reason)
	assert.Equal(t, detection.Critical, findings[0].Severity)
	assert.Contains(t, findings[0].Summary, "expired")
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
