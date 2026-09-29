package detect

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

func TestCertificateValid(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	expiry := now.Add(365 * 24 * time.Hour)

	secret := buildStorage(
		kube.KindSecret, "tls", "default", now,
		map[string]knowledge.Value{
			kube.AttrCertExpiry: knowledge.Time(expiry),
		},
	)

	detector := Certificate{}
	ctx := testDetectorContext(nil, now)

	signals := detector.Detect(ctx, secret)

	assert.Empty(t, signals)
}

func TestCertificateExpiring(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	expiry := now.Add(7 * 24 * time.Hour)

	secret := buildStorage(
		kube.KindSecret, "tls", "default", now,
		map[string]knowledge.Value{
			kube.AttrCertExpiry: knowledge.Time(expiry),
		},
	)

	detector := Certificate{}
	ctx := testDetectorContext(nil, now)

	signals := detector.Detect(ctx, secret)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonTLSCertExpiringSoon, signals[0].Reason)
	assert.Equal(t, signal.Warning, signals[0].Severity)
	assert.Contains(t, signals[0].Summary, "expires in")
}

func TestCertificateExpired(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	expiry := now.Add(-24 * time.Hour)

	secret := buildStorage(
		kube.KindSecret, "tls", "default", now,
		map[string]knowledge.Value{
			kube.AttrCertExpiry: knowledge.Time(expiry),
		},
	)

	detector := Certificate{}
	ctx := testDetectorContext(nil, now)

	signals := detector.Detect(ctx, secret)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonTLSCertExpired, signals[0].Reason)
	assert.Equal(t, signal.Critical, signals[0].Severity)
	assert.Contains(t, signals[0].Summary, "expired")
}

func TestMissingSecret(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	model := newTestModel()
	podID := knowledge.EntityID{Kind: kube.KindPod, Namespace: "default",
		Name: "pod"}
	secretID := knowledge.EntityID{Kind: kube.KindSecret,
		Namespace: "default", Name: "creds"}

	// Create pod but not the secret
	model.Apply(knowledge.Fact{
		Kind:       knowledge.Observed,
		Source:     "test",
		At:         now,
		Entity:     podID,
		Attributes: map[string]knowledge.Value{},
	})

	// Create reference from pod to secret
	model.Apply(knowledge.Fact{
		Kind:     knowledge.Related,
		Source:   "test",
		At:       now,
		Entity:   podID,
		Relation: knowledge.References,
		Targets:  []knowledge.EntityID{secretID},
	})

	entity, _ := model.Entity(podID)
	detector := Missing{}
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonProjectedSecretMissing, signals[0].Reason)
	assert.Equal(t, signal.Critical, signals[0].Severity)
	assert.Contains(t, signals[0].Summary, "creds")
}

func TestMissingConfigMap(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	model := newTestModel()
	podID := knowledge.EntityID{Kind: kube.KindPod, Namespace: "default",
		Name: "pod"}
	configID := knowledge.EntityID{Kind: kube.KindConfigMap,
		Namespace: "default", Name: "app-config"}

	model.Apply(knowledge.Fact{
		Kind:       knowledge.Observed,
		Source:     "test",
		At:         now,
		Entity:     podID,
		Attributes: map[string]knowledge.Value{},
	})

	model.Apply(knowledge.Fact{
		Kind:     knowledge.Related,
		Source:   "test",
		At:       now,
		Entity:   podID,
		Relation: knowledge.References,
		Targets:  []knowledge.EntityID{configID},
	})

	entity, _ := model.Entity(podID)
	detector := Missing{}
	ctx := testDetectorContext(model, now)

	signals := detector.Detect(ctx, entity)

	require.Len(t, signals, 1)
	assert.Equal(t, constant.ReasonProjectedConfigMapMissing, signals[0].Reason)
}
