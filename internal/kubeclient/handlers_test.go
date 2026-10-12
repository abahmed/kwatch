package kubeclient

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/cache"
)

func TestSafeEventHandlerContainsPanicsAndContinues(t *testing.T) {
	called := 0
	handler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(interface{}) {
			called++
			if called == 1 {
				panic("test panic")
			}
		},
	}
	safe := SafeEventHandler("test", "pods", handler)

	safe.OnAdd(struct{}{}, false)
	safe.OnAdd(struct{}{}, false)

	require.Equal(t, 2, called)
}

func TestSafeEventHandlerUnwrapsDeleteTombstone(t *testing.T) {
	var got []interface{}
	safe := SafeEventHandler("test", "pods", cache.ResourceEventHandlerFuncs{
		DeleteFunc: func(obj interface{}) { got = append(got, obj) },
	})

	safe.OnDelete(cache.DeletedFinalStateUnknown{Key: "ns/a", Obj: "last"})
	safe.OnDelete("plain")

	require.Equal(t, []interface{}{"last", "plain"}, got)
}

func TestSafeEventHandlerRecordsPanicsPerResource(t *testing.T) {
	count := func(component, resource string) int64 {
		for _, p := range HandlerPanics() {
			if p.Component == component && p.Resource == resource {
				return p.Count
			}
		}
		return 0
	}
	before := count("ledger-test", "widgets")
	safe := SafeEventHandler("ledger-test", "widgets",
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(interface{}) { panic("boom") },
		})

	safe.OnAdd(struct{}{}, false)
	safe.OnAdd(struct{}{}, false)

	require.Equal(t, before+2, count("ledger-test", "widgets"))
}

func TestPanicLedgerBoundsDistinctResources(t *testing.T) {
	ledger := &panicLedger{counts: map[[2]string]int64{}}
	for i := 0; i < maxPanicResources+10; i++ {
		ledger.record("c", string(rune('a'+i%26))+string(rune('A'+i/26)))
	}

	snapshot := ledger.snapshot()

	require.LessOrEqual(t, len(snapshot), maxPanicResources+1)
	var other int64
	for _, p := range snapshot {
		if p.Resource == overflowResource {
			other = p.Count
		}
	}
	require.Equal(t, int64(10), other)
}
