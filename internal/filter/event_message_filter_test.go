package filter

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/config"
)

func TestEventMessageFilterMatchesConfiguredSubstring(t *testing.T) {
	ctx := &Context{
		Sources: Sources{Config: &config.Config{
			Suppression: config.SuppressionIndex{
				EventMessages: []string{"sync configmap cache"},
			},
		}},
		Events: &[]corev1.Event{{
			Message: "failed to sync configmap cache: timed out",
		}},
	}

	if !(EventMessageFilter{}).Enrich(ctx) {
		t.Fatal("matching event message was not suppressed")
	}
}

func TestEventMessageFilterIgnoresNonMatchingMessages(t *testing.T) {
	ctx := &Context{
		Sources: Sources{Config: &config.Config{
			Suppression: config.SuppressionIndex{
				EventMessages: []string{"sync configmap cache"},
			},
		}},
		Events: &[]corev1.Event{{
			Message: "Successfully pulled image",
		}},
	}

	if (EventMessageFilter{}).Enrich(ctx) {
		t.Fatal("non-matching event message was suppressed")
	}
}

func TestEventMessageFilterHandlesMissingEvents(t *testing.T) {
	ctx := &Context{Sources: Sources{Config: &config.Config{}}}

	if (EventMessageFilter{}).Enrich(ctx) {
		t.Fatal("missing events should not suppress an incident")
	}
}
