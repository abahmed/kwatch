package k8s

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
