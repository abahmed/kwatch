package app

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/kubeclient"
)

func TestSourceHealthDegradesResourceWhoseHandlerPanics(t *testing.T) {
	sink := &fakeStatusSink{}
	counts := []kubeclient.HandlerPanicCount{
		{Component: "inventory", Resource: "pods", Count: 3},
		{Component: "config", Resource: "kwatchconfigs", Count: 9},
	}
	health := newSourceHealth(sink, &fakeAvailability{}).
		withHandlerPanics(func() []kubeclient.HandlerPanicCount {
			return counts
		})

	health.report()
	require.Equal(t, recordedStatus{"degraded", "handler_panics", false},
		sink.get("source-pods"))
	require.Equal(t, recordedStatus{}, sink.get("source-kwatchconfigs"))

	// No new panic since the last report: the resource recovers.
	health.report()
	require.Equal(t, recordedStatus{"running", "", true},
		sink.get("source-pods"))

	counts[0].Count = 4
	health.report()
	require.Equal(t, "handler_panics", sink.get("source-pods").reason)
}

func TestSourceHealthKeepsMoreBasicReasonOverPanics(t *testing.T) {
	sink := &fakeStatusSink{}
	source := &fakeAvailability{}
	source.set(kube.SourceStatus{
		Resource: "pods", Required: true, Reason: kube.ReasonAPIUnavailable,
	})
	health := newSourceHealth(sink, source).
		withHandlerPanics(func() []kubeclient.HandlerPanicCount {
			return []kubeclient.HandlerPanicCount{
				{Component: "inventory", Resource: "pods", Count: 1},
			}
		})

	health.report()

	require.Equal(t, "api_unavailable", sink.get("source-pods").reason)
}
