package compose

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func windowFacts(healthy, failed time.Time) caseFacts {
	id := kube.EntityFor("apps/v1", "Deployment", "shop", "api")
	return caseFacts{window: &incident.HealthWindow{
		Healthy: healthy, Failed: failed,
		Changes: []inventory.Change{{
			Entity: id, At: healthy.Add(10 * time.Second), Actor: "bob",
			Fields: []inventory.FieldChange{{
				Path:   "spec.template.spec.containers[app].image",
				Before: "app:1", After: "app:2"}},
		}}}}
}

func TestWindowSentenceNamesBothTimesAndTheChange(t *testing.T) {
	healthy := time.Date(2026, 10, 13, 10, 1, 40, 0, time.UTC)
	got := windowSentences(windowFacts(healthy, healthy.Add(30*time.Second)))

	assert.Len(t, got, 1)
	assert.Contains(t, got[0].text,
		"Between 10:01:40 (last healthy) and 10:02:10 (first failure): ")
	assert.Contains(t, got[0].text, "bob")
}

func TestWindowSentenceNamesTheDayAcrossMidnight(t *testing.T) {
	healthy := time.Date(2026, 10, 12, 23, 50, 0, 0, time.UTC)
	got := windowSentences(windowFacts(healthy, healthy.Add(20*time.Minute)))

	assert.Contains(t, got[0].text, "Oct 12 23:50:00 (last healthy)")
	assert.Contains(t, got[0].text, "Oct 13 00:10:00 (first failure)")
}

func TestWindowSentenceIsAbsentWithoutBothTimes(t *testing.T) {
	at := time.Date(2026, 10, 13, 10, 0, 0, 0, time.UTC)
	assert.Empty(t, windowSentences(caseFacts{}))
	assert.Empty(t, windowSentences(windowFacts(time.Time{}, at)))
	assert.Empty(t, windowSentences(windowFacts(at, time.Time{})))
	assert.Empty(t, windowSentences(windowFacts(at, at.Add(-time.Minute))))
}
