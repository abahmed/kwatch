package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// deprecatedModel has an API server at minor 29 (0 for unknown) and one
// deprecated API first seen seenAgo before t0.
func deprecatedModel(
	minor float64, group, version, resource, removed string,
	seenAgo time.Duration,
) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	if minor > 0 {
		put(m, kube.APIServer, t0, map[string]inventory.Value{
			kube.AttrServerMinor: inventory.Number(minor),
		})
	}
	api := kube.DeprecatedAPI{Group: group, Version: version,
		Resource: resource, Removed: removed}.ID()
	put(m, api, t0.Add(-seenAgo), map[string]inventory.Value{
		kube.AttrDeprecatedGroup:    inventory.Text(group),
		kube.AttrDeprecatedVersion:  inventory.Text(version),
		kube.AttrDeprecatedResource: inventory.Text(resource),
		kube.AttrDeprecatedRemoved:  inventory.Text(removed),
	})
	return m, api
}

func detectDeprecated(
	m *inventory.Model, id inventory.EntityID,
) []detection.Finding {
	return DeprecatedAPI{}.Detect(testDetectorContext(m, t0),
		entityOf(m, id))
}

func TestDeprecatedAPIFarRemovalIsOneInfoFinding(t *testing.T) {
	m, id := deprecatedModel(20, "policy", "v1beta1",
		"poddisruptionbudgets", "1.25", 10*time.Minute)

	got := detectDeprecated(m, id)

	require.Len(t, got, 1)
	assert.Equal(t, "DeprecatedAPIInUse", got[0].Reason)
	assert.Equal(t, detection.Info, got[0].Severity)
	assert.Equal(t, "removed in 1.25", got[0].Summary)
	assert.Contains(t, got[0].Evidence[0].Value, "Something still calls "+
		"policy/v1beta1 poddisruptionbudgets, which is removed in "+
		"Kubernetes 1.25")
	assert.Equal(t, "policy/v1", got[0].Evidence[1].Value)
}

func TestDeprecatedAPINearRemovalSaysWhichUpgrade(t *testing.T) {
	m, id := deprecatedModel(24, "batch", "v1beta1", "cronjobs",
		"1.25", 10*time.Minute)
	got := detectDeprecated(m, id)
	require.Len(t, got, 1)
	assert.Contains(t, got[0].Summary, "removed in 1.25, the next "+
		"minor upgrade")

	m, id = deprecatedModel(23, "batch", "v1beta1", "cronjobs",
		"1.25", 10*time.Minute)
	got = detectDeprecated(m, id)
	require.Len(t, got, 1)
	assert.Contains(t, got[0].Summary, "two minor upgrades away")
}

func TestDeprecatedAPIAlreadyRemovedSaysCallsFail(t *testing.T) {
	m, id := deprecatedModel(29, "policy", "v1beta1",
		"podsecuritypolicies", "1.25", 10*time.Minute)

	got := detectDeprecated(m, id)

	require.Len(t, got, 1)
	assert.Equal(t, "already removed in 1.25; calls fail", got[0].Summary)
	assert.Contains(t, got[0].Evidence[0].Value, "was removed")
}

func TestDeprecatedAPIWithoutServerVersionOrReplacement(t *testing.T) {
	m, id := deprecatedModel(0, "extensions", "v1beta1", "ingresses",
		"1.22", 10*time.Minute)

	got := detectDeprecated(m, id)

	require.Len(t, got, 1)
	assert.Equal(t, "removed in 1.22", got[0].Summary)
	assert.Equal(t, "the newer version", got[0].Evidence[1].Value)
}

func TestDeprecatedAPIWaitsBeforeReporting(t *testing.T) {
	m, id := deprecatedModel(24, "policy", "v1beta1",
		"poddisruptionbudgets", "1.25", 10*time.Second)

	assert.Empty(t, detectDeprecated(m, id))
}

func TestDeprecatedAPIIgnoresAnUnparsableRelease(t *testing.T) {
	m, id := deprecatedModel(24, "policy", "v1beta1",
		"poddisruptionbudgets", "", 10*time.Minute)

	assert.Empty(t, detectDeprecated(m, id))
}

func TestDeprecatedAPICoreGroupHasNoPrefix(t *testing.T) {
	m, id := deprecatedModel(24, "", "v1", "componentstatuses", "1.30",
		10*time.Minute)

	got := detectDeprecated(m, id)

	require.Len(t, got, 1)
	assert.Contains(t, got[0].Evidence[0].Value, "calls v1 componentstatuses")
	assert.Equal(t, "the newer version", got[0].Evidence[1].Value)
}
