package enrichment

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/config"
)

func TestEventMessageEnricherMatchesConfiguredSubstring(t *testing.T) {
	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{
			Silences: []config.SilenceRule{{
				EventMessages: []string{"sync configmap cache"},
			}},
		}),
		Events: &[]corev1.Event{{
			Message: "failed to sync configmap cache: timed out",
		}},
	}

	if !(EventMessageEnricher{}).Enrich(ctx) {
		t.Fatal("matching event message was not suppressed")
	}
}

func TestEventMessageEnricherIgnoresNonMatchingMessages(t *testing.T) {
	ctx := &Context{
		Runtime: config.RuntimeConfigFor(&config.Config{
			Silences: []config.SilenceRule{{
				EventMessages: []string{"sync configmap cache"},
			}},
		}),
		Events: &[]corev1.Event{{
			Message: "Successfully pulled image",
		}},
	}

	if (EventMessageEnricher{}).Enrich(ctx) {
		t.Fatal("non-matching event message was suppressed")
	}
}

func TestEventMessageEnricherHandlesMissingEvents(t *testing.T) {
	ctx := &Context{Runtime: config.RuntimeConfigFor(&config.Config{})}

	if (EventMessageEnricher{}).Enrich(ctx) {
		t.Fatal("missing events should not suppress an incident")
	}
}
