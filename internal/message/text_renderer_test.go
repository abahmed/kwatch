package message

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/clock"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
)

func sampleReport(action model.IncidentAction) *Report {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	inc := &model.Incident{
		Subject: model.Subject{
			Reason:        "ContainersNotReady",
			Namespace:     "dev",
			OwnerKind:     "Deployment",
			Resource:      "pod",
			Name:          "api",
			ContainerName: "api",
			Image: "registry.example.com/team/" +
				"api:1.2.0",
			NodeName: "ip-10-0-81-7.us-east-1.compute.internal",
		},
		Status: model.Status{
			Count:        3,
			RestartCount: 2,
			LastContainerState: &model.ContainerState{
				Msg:      "pod stopped being ready 2m ago",
				ExitCode: 137,
			},
			FirstSeen: now.Add(-2 * time.Minute),
			LastSeen:  now,
			Severity:  model.SeverityHigh,
		},
		Evidence: model.Evidence{
			Hint: "pod stopped being ready 2m ago; check readiness " +
				"probe and recent logs",
			Events:        "Aug 25 23:54:07  Unhealthy  Liveness probe failed",
			IncludeEvents: true,
		},
	}

	ins := &insight.Insight{
		Cause:      "node ip-10-0-81-7 may be unhealthy",
		Pattern:    "node_failure",
		Impact:     "affects service api",
		CauseState: insight.CauseConfirmed,
		Confidence: 0.9,
		Evidence:   []string{"node evidence"},
		RecentChanges: []kwcontext.Change{
			{
				Resource:  "deployment",
				Namespace: "dev",
				Name:      "api",
				Type:      kwcontext.ChangeUpdate,
				Timestamp: now.Add(-3 * time.Minute),
			},
		},
	}
	return NewReportBuilderWithClock(
		"dev", clock.Func(func() time.Time { return now }),
	).Build(inc, action, ins)
}

func TestTextRendererReadsTopDown(t *testing.T) {
	out := NewPlainTextRenderer().RenderCreate(sampleReport(model.ActionCreate))
	lines := strings.Split(out, "\n")

	// Headline: human label first, then what it happened to, then the raw code.
	assert.Equal(
		t,
		"🟠 Pod not ready — dev/api · Deployment",
		lines[0],
	)
	assert.Contains(t, out, "Node ip-10-0-81-7 may be unhealthy.")
	assert.NotContains(t, out, "ContainersNotReady")
	assert.NotContains(t, out, "Timeline:")
	// A change carries its age.
	assert.Regexp(
		t,
		regexp.MustCompile(
			`A recent change may be related: deployment dev/api updated \d+[smh] ago`,
		),
		out,
	)
	// Short names, no registry, no domain.
	assert.Contains(t, out, "Container api")
	assert.NotContains(t, out, "api:1.2.0")
	assert.NotContains(t, out, "amazonaws")
	assert.Contains(t, out, "node ip-10-0-81-7")
	assert.NotContains(t, out, "compute.internal")
	// No blank lines from empty sections.
	assert.NotContains(t, out, "\n\n")
}

func TestTextRendererGroupSubjectIsUsedVerbatim(t *testing.T) {
	r := newTestReportBuilder("").Build(&model.Incident{
		Subject: model.Subject{
			Reason:    "ContainersNotReady",
			Namespace: "dev",
			Resource:  "pod",
			OwnerKind: "Deployment",
			Name:      "6 workloads in dev: accounts, api",
		},
		Status: model.Status{
			Count:         6,
			PeakResources: 6,
		},
	},

		model.ActionCreate, nil)
	out := NewPlainTextRenderer().RenderCreate(r)
	assert.Contains(
		t,
		out,
		"Pod not ready — 6 workloads in dev: accounts, api",
	)
	assert.NotContains(
		t,
		out,
		"dev/6 workloads",
		"a sentence is not qualified with a namespace",
	)
	assert.NotContains(
		t,
		out,
		"· Deployment",
		"the first member's kind does not label a group",
	)
	assert.Contains(t, out, "6 affected pods")
}

// The three text renderers must say the same thing; only the markup may
// differ. Stripping the markup from each must give identical text.
func TestTextRenderersAgreeModuloMarkup(t *testing.T) {
	fence := regexp.MustCompile("\\n?```\\n?")
	strip := func(s string) string {
		s = strings.ReplaceAll(s, "**", "")
		s = strings.ReplaceAll(s, "*", "")
		s = fence.ReplaceAllString(s, "\n")
		s = strings.ReplaceAll(s, "\n\n", "\n")
		s = strings.ReplaceAll(s, "💡", "Hint:")
		s = strings.ReplaceAll(s, "`", "")
		return strings.TrimSpace(s)
	}
	actions := []model.IncidentAction{
		model.ActionCreate, model.ActionUpdate, model.ActionResolved,
	}
	for _, action := range actions {
		r := sampleReport(action)
		plain := strip(NewPlainTextRenderer().RenderCreate(r))
		slack := strip(NewSlackRenderer().RenderCreate(r))
		discord := strip(NewDiscordRenderer().RenderCreate(r))
		require.Equal(
			t,
			plain,
			slack,
			"slack text diverged from plain for %v",
			action,
		)
		require.Equal(
			t,
			plain,
			discord,
			"discord diverged from plain for %v",
			action,
		)
	}
}

func TestTextRendererResolvedIsOneBreath(t *testing.T) {
	r := sampleReport(model.ActionResolved)
	out := NewPlainTextRenderer().RenderResolved(r)
	assert.Equal(
		t,
		"✅ Resolved — Pod not ready — dev/api\n"+
			"lasted 2m · node ip-10-0-81-7",
		out,
	)
}
