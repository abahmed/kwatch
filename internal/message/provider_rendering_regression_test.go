package message

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/model"
)

func TestProviderRenderersShareSafeHumanSemantics(t *testing.T) {
	report := &Report{
		Action: "create", Severity: "high", Reason: "CrashLoopBackOff",
		Namespace: "payments", Resource: "pod", Name: "checkout",
		Summary: SummarySection{Emoji: "🟠", Label: "Pod keeps restarting"},
		Diagnosis: &DiagnosisSection{
			Cause:   "a recent change may be related",
			Pattern: "recent_change", Confidence: 0.42,
			Hint: "the container has restarted repeatedly",
		},
	}
	renderers := map[string]Renderer{
		"plain":   NewPlainTextRenderer(),
		"slack":   NewSlackRenderer(),
		"discord": NewDiscordRenderer(),
	}
	for name, renderer := range renderers {
		t.Run(name, func(t *testing.T) {
			text := renderer.RenderCreate(report)
			require.Contains(t, text, "Pod keeps restarting")
			require.NotContains(t, strings.ToLower(text), "confidence")
			require.NotContains(t, text, "42%")
			require.NotContains(t, text, "a recent change may be related")
		})
	}
}

func TestProviderRenderersKeepVerifiedRecoveryEvidence(t *testing.T) {
	report := &Report{
		Action: "resolved", Reason: "CrashLoopBackOff", Resource: "pod",
		Namespace: "payments", Name: "checkout",
		Summary: SummarySection{Emoji: "✅", Label: "Pod recovered"},
		Resolution: &model.Resolution{
			Summary:  "the replacement Pod is ready",
			Evidence: "all containers have been ready for 2m",
		},
	}
	for _, renderer := range []Renderer{
		NewPlainTextRenderer(), NewSlackRenderer(), NewDiscordRenderer(),
	} {
		text := renderer.RenderResolved(report)
		require.Contains(t, text, "replacement Pod is ready")
		require.Contains(t, text, "all containers have been ready")
	}
}
