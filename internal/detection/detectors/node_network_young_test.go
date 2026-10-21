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

// routelessNode is a Ready node created joinedAgo before joinNow whose
// NetworkUnavailable has been True since it joined.
func routelessNode(joinedAgo time.Duration,
) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	id := newID(kube.KindNode, "", "n-new")
	created := joinNow.Add(-joinedAgo)
	attrs := conditionAttrs(map[string]inventory.Value{
		kube.AttrCreated: inventory.Time(created),
	}, "Ready", "True", "KubeletReady", created)
	conditionAttrs(attrs, "NetworkUnavailable", "True", "NoRouteCreated",
		created)
	observeEntity(m, id, created, attrs)
	return m, id
}

// The cloud controller creates a new node's route within minutes: the
// condition is expected while the node is young.
func TestNodeNetworkUnavailableWaitsWhileTheNodeIsYoung(t *testing.T) {
	m, id := routelessNode(2 * time.Minute)

	got := evaluate(NewNode(0), m, joinNow, id, nil)

	assert.Empty(t, got.Findings)
	assert.Equal(t, kube.BootWindow-2*time.Minute, got.RecheckAfter)
}

func TestNodeNetworkUnavailableIsReportedOnceTheNodeIsOld(t *testing.T) {
	m, id := routelessNode(kube.BootWindow + time.Minute)

	got := evaluate(NewNode(0), m, joinNow, id, nil).Findings

	require.Len(t, got, 1)
	assert.Equal(t, reasons.NetworkUnavailable, got[0].Reason)
	assert.Equal(t, detection.Critical, got[0].Severity)
}

func TestNodeNetworkUnavailableOfAnOldNodeIsReportedAtOnce(t *testing.T) {
	m := newTestModel()
	id := newID(kube.KindNode, "", "n-old")
	since := joinNow.Add(-time.Second)
	attrs := conditionAttrs(map[string]inventory.Value{
		kube.AttrCreated: inventory.Time(joinNow.Add(-48 * time.Hour)),
	}, "NetworkUnavailable", "True", "NoRouteCreated", since)
	observeEntity(m, id, since, attrs)

	got := evaluate(NewNode(0), m, joinNow, id, nil).Findings

	assert.Equal(t, []string{reasons.NetworkUnavailable},
		findingReasons(got))
}
