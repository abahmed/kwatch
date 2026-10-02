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
