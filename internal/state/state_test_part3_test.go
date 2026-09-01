package state

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

func TestEngineBackedBaselineRoundTrip(t *testing.T) {
	assert := assert.New(t)
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	sm := NewStateManager(client, "kwatch")

	// Save a realistic baseline (same format as controller.buildSeenSet
	// produces)
	baseline := map[string]map[string]int64{
		"default:dep-1:CrashLoopBackOff:": {"pod-1": time.Now().Unix()},
	}
	err := sm.SaveBaseline(ctx, baseline)
	assert.Nil(err)

	// Load baseline — simulates startup reload from ConfigMap
	loaded := sm.GetBaseline(ctx)
	assert.NotNil(loaded)
	assert.Equal(baseline, loaded)

	// Feed loaded baseline into correlation engine (as main.go does)
	e := correlation.NewEngine(correlation.Config{
		Window:   10 * time.Minute,
		Baseline: loaded,
	})

	// Previously seen pod+container should be suppressed
	ev1 := event.Event{
		PodName: "pod-1", Namespace: "default",
		Reason: "CrashLoopBackOff", ContainerName: "app",
	}
	_, action := e.Process(ev1, "dep-1", &model.ContainerState{RestartCount: 1})
	assert.Equal(model.ActionSkip, action, "baselined pod must be suppressed")

	// A new pod for the same owner+reason should create an incident
	ev2 := event.Event{
		PodName: "pod-2", Namespace: "default",
		Reason: "CrashLoopBackOff", ContainerName: "app",
	}
	_, action = e.Process(ev2, "dep-1", &model.ContainerState{RestartCount: 1})
	assert.Equal(model.ActionCreate, action, "unseen pod must create incident")
}

func TestSaveAndGetIncidents(t *testing.T) {
	assert := assert.New(t)
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	sm := NewStateManager(client, "kwatch")

	incidents := []map[string]interface{}{
		{
			"key":       "default:dep-1:OOMKilled:",
			"reason":    "OOMKilled",
			"namespace": "default",
			"name":      "dep-1",
			"resource":  "pod",
			"count":     3,
			"severity":  "high",
		},
	}
	err := sm.SaveIncidents(ctx, incidents)
	assert.Nil(err)

	var loaded []map[string]interface{}
	err = sm.GetIncidents(ctx, &loaded)
	assert.Nil(err)
	assert.Equal(1, len(loaded))
	assert.Equal("default:dep-1:OOMKilled:", loaded[0]["key"])
	assert.Equal(float64(3), loaded[0]["count"])
	assert.Equal("high", loaded[0]["severity"])

	cm, err := client.CoreV1().ConfigMaps(
		"kwatch",
	).Get(
		ctx,
		incidentsConfigMapName,
		metav1.GetOptions{},
	)
	assert.Nil(err)
	assert.NotNil(cm.BinaryData[incidentsKey])
}

func TestGetIncidentsNoConfigMap(t *testing.T) {
	assert := assert.New(t)
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	sm := NewStateManager(client, "kwatch")

	var loaded []map[string]interface{}
	err := sm.GetIncidents(ctx, &loaded)
	assert.Nil(err)
	assert.Nil(loaded)
}

func TestSaveIncidentsOverwrites(t *testing.T) {
	assert := assert.New(t)
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	sm := NewStateManager(client, "kwatch")

	first := []map[string]interface{}{
		{"key": "ns:dep-1:Error:", "count": 1},
	}
	err := sm.SaveIncidents(ctx, first)
	assert.Nil(err)

	var loaded []map[string]interface{}
	err = sm.GetIncidents(ctx, &loaded)
	assert.Nil(err)
	assert.Equal(1, len(loaded))
	assert.Equal("ns:dep-1:Error:", loaded[0]["key"])

	second := []map[string]interface{}{
		{"key": "ns:dep-2:OOMKilled:", "count": 5},
	}
	err = sm.SaveIncidents(ctx, second)
	assert.Nil(err)

	loaded = nil
	err = sm.GetIncidents(ctx, &loaded)
	assert.Nil(err)
	assert.Equal(1, len(loaded))
	assert.Equal("ns:dep-2:OOMKilled:", loaded[0]["key"])
}

func TestSaveAndGetPersistedIncidentsRoundTrip(t *testing.T) {
	assert := assert.New(t)
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	sm := NewStateManager(client, "kwatch")

	now := time.Now().Truncate(time.Second)
	incidents := []model.PersistedIncident{
		{
			Key:            "pod/ns/dep-1",
			Reason:         "OOMKilled",
			Namespace:      "ns",
			Name:           "dep-1",
			Resource:       "pod",
			Count:          3,
			FirstSeen:      now.Add(-time.Hour),
			LastSeen:       now,
			OwnerKind:      "Deployment",
			Severity:       model.SeverityHigh,
			State:          model.StatePendingResolve,
			ResolveAt:      now.Add(2 * time.Minute),
			NotifiedSig:    "firing|high",
			LastNotifiedAt: now,
			RenotifyCount:  1,
		},
	}
	err := sm.SaveIncidents(ctx, incidents)
	assert.Nil(err)

	var loaded []model.PersistedIncident
	err = sm.GetIncidents(ctx, &loaded)
	assert.Nil(err)
	assert.Equal(1, len(loaded))
	assert.Equal("pod/ns/dep-1", string(loaded[0].Key))
	assert.Equal("OOMKilled", loaded[0].Reason)
	assert.Equal(float64(3), float64(loaded[0].Count))
	assert.Equal(model.SeverityHigh, loaded[0].Severity)
	assert.Equal(model.StatePendingResolve, loaded[0].State)
	assert.True(
		loaded[0].ResolveAt.Equal(now.Add(2*time.Minute)),
		"ResolveAt must survive ConfigMap round trip",
	)
}

func TestSaveBaselineTooLarge(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewSimpleClientset()
	sm := NewStateManager(client, "kwatch")
	// Build a baseline large enough that even gzipped it exceeds
	// baselineMaxBytes (~1,032,192). Use many entries with unique keys
	// to minimise gzip leverage.
	large := make(map[string]map[string]int64)
	for i := 0; i < 200000; i++ {
		key := fmt.Sprintf("k-%08d", i)
		large[key] = map[string]int64{
			fmt.Sprintf("p-%08d", i): int64(1718064000 + i),
		}
	}

	err := sm.SaveBaseline(context.Background(), large)
	assert.NotNil(err, "oversized baseline should be rejected")
	assert.Contains(err.Error(), "exceeds budget")
}

// A snapshot too large for a ConfigMap used to save nothing at all, so a big
// cluster silently lost every incident on each restart. Keeping the freshest
// incidents that fit is strictly better than keeping none.
func TestIncidentStateTrimsToBudgetInsteadOfDroppingAll(t *testing.T) {
	now := time.Now().UTC()
	// Deliberately incompressible payloads so gzip cannot rescue the size.
	// A short repeating pattern would simply be compressed away.
	seed := uint64(0x9E3779B97F4A7C15)
	blob := func(n int) string {
		b := make([]byte, n)
		for i := range b {
			seed ^= seed << 13
			seed ^= seed >> 7
			seed ^= seed << 17
			b[i] = byte('a' + seed%26)
		}
		return string(b)
	}
	var big []model.PersistedIncident
	for i := 0; i < 4000; i++ {
		big = append(big, model.PersistedIncident{
			Key:      model.IncidentKey(fmt.Sprintf("ns:dep-%04d:Error:", i)),
			Reason:   "Error",
			Name:     blob(600),
			LastSeen: now.Add(-time.Duration(i) * time.Minute),
		})
	}

	raw, err := gzJSON(big)
	require.NoError(t, err)
	require.Greater(
		t,
		len(raw),
		baselineMaxBytes,
		"fixture must exceed the budget",
	)

	kept := trimIncidentsToBudget(big)
	require.NotEmpty(t, kept, "must keep something rather than nothing")
	require.Less(t, len(kept), len(big))

	fitted, err := gzJSON(kept)
	require.NoError(t, err)
	assert.LessOrEqual(
		t,
		len(fitted),
		baselineMaxBytes,
		"trimmed payload must fit",
	)

	// The freshest incident survives; the stalest does not.
	assert.Equal(t, model.IncidentKey("ns:dep-0000:Error:"), kept[0].Key)
	for _, inc := range kept {
		assert.NotEqual(t, model.IncidentKey("ns:dep-3999:Error:"), inc.Key,
			"the stalest incident should be shed first")
	}
	t.Logf("%d incidents (%d gz-bytes) -> kept %d (%d gz-bytes, budget %d)",
		len(big), len(raw), len(kept), len(fitted), baselineMaxBytes)
}

// A snapshot that already fits must pass through untouched.
func TestIncidentStateUnderBudgetIsUnchanged(t *testing.T) {
	in := []model.PersistedIncident{
		{Key: "ns:a:Error:", LastSeen: time.Now()},
		{Key: "ns:b:Error:", LastSeen: time.Now().Add(-time.Hour)},
	}
	assert.Equal(t, in, trimIncidentsToBudget(in))
}

// v0.10.x wrote incident state as a JSON object keyed by id whose values were
// full model.Incident values; the reader always expected a []PersistedIncident.
// The mismatch meant no restore ever succeeded, and the first upgrade produced
// exactly this error in production. This reproduces it and proves recovery.
func TestLegacyIncidentStateIsMigratedOnLoad(t *testing.T) {
	ctx := context.Background()
	sm := NewStateManager(fake.NewSimpleClientset(), "kwatch")
	now := time.Now().UTC().Truncate(time.Second)

	legacy := map[string]any{
		"b-second": map[string]any{
			"Key":       "ns:dep-b:OOMKilled:",
			"Reason":    "OOMKilled",
			"Namespace": "ns",
			"Name":      "dep-b",
			"Count":     7,
			"FirstSeen": now,
		},
		"a-first": map[string]any{
			"Key":       "ns:dep-a:CrashLoopBackOff:",
			"Reason":    "CrashLoopBackOff",
			"Namespace": "ns",
			"Name":      "dep-a",
			"Count":     2,
			"FirstSeen": now,
		},
	}
	require.NoError(t, sm.SaveIncidents(ctx, legacy))

	var direct []model.PersistedIncident
	err := sm.GetIncidents(ctx, &direct)
	require.Error(
		t,
		err,
		"the untyped reader must still fail on the legacy shape — that is the "+
			"bug",
	)
	assert.Contains(
		t,
		err.Error(),
		"cannot unmarshal object into Go value of type "+
			"[]model.PersistedIncident",
	)

	got, err := sm.LoadPersistedIncidents(ctx)
	require.NoError(t, err, "the typed loader must recover the legacy shape")
	require.Len(t, got, 2)
	assert.Equal(
		t,
		"dep-a",
		got[0].Name,
		"restores are sorted by key, not map order",
	)
	assert.Equal(t, "dep-b", got[1].Name)
	assert.Equal(t, 7, got[1].Count)

	// Once re-saved in the current shape, no migration is needed.
	require.NoError(t, sm.SavePersistedIncidents(ctx, got))
	again, err := sm.LoadPersistedIncidents(ctx)
	require.NoError(t, err)
	assert.Equal(t, got, again)
}
