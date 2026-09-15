package enrichment

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/config"
)

func TestEventMessageEnricherSuppressesConfiguredMessage(t *testing.T) {
	ctx := &Context{
		Sources: Sources{Runtime: config.RuntimeConfigFor(&config.Config{
			Silences: []config.SilenceRule{{
				EventMessages: []string{"evicted"},
			}},
		})},
		Events: &[]corev1.Event{{Message: "evicted by the operator"}},
	}

	if !(EventMessageEnricher{}).Enrich(ctx) {
		t.Fatal("expected configured event message to suppress the finding")
	}
}

func TestPodEventsEnricherSuppressesDeletingPod(t *testing.T) {
	ctx := &Context{
		PodHasIssues: true,
		Events: &[]corev1.Event{{
			Type:    corev1.EventTypeWarning,
			Message: "deleting pod during rollout",
		}},
	}

	if !(PodEventsEnricher{}).Enrich(ctx) {
		t.Fatal("expected deleting-pod event to suppress the finding")
	}
	if ctx.PodHasIssues || ctx.ContainersHasIssues {
		t.Fatal("expected Pod findings to be cleared")
	}
}

func TestLogsUnavailableRecognizesKubeletResponse(t *testing.T) {
	if !LogsUnavailable("unable to retrieve container logs for pod") {
		t.Fatal("expected kubelet unavailable response to be recognized")
	}
	if LogsUnavailable("panic: runtime error") {
		t.Fatal("did not expect application output to be classified as unavailable")
	}
}
