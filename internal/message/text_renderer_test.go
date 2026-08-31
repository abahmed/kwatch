package message

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	kwcontext "github.com/abahmed/kwatch/internal/context"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
)

func sampleReport(action model.IncidentAction) *Report {
	now := time.Now()
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
		Cause:   "node ip-10-0-81-7 may be unhealthy",
		Pattern: "node_failure",
		Impact:  "affects service api",
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
	return NewReportBuilder("dev").Build(inc, action, ins)
}

func TestTextRendererReadsTopDown(t *testing.T) {
	out := NewPlainTextRenderer().RenderCreate(sampleReport(model.ActionCreate))
	lines := strings.Split(out, "\n")

	// Headline: human label first, then what it happened to, then the raw code.
	assert.Equal(
		t,
		"🟠 Pod not ready — dev/api · Deployment · "+
			"ContainersNotReady · high",
		lines[0],
	)
	// The reason appears exactly once, not four times.
	assert.Equal(t, 1, strings.Count(out, "ContainersNotReady"))
	// The state message leads the natural-language explanation and is not
	// repeated in the hint.
	assert.True(t, strings.HasPrefix(lines[1], "pod stopped being ready 2m ago"))
	assert.Equal(t, 1, strings.Count(out, "pod stopped being ready"))
	// Diagnosis comes before the hint and the details without exposing a form.
	cause, hint, meta := strings.Index(
		out,
		"The strongest signal points to",
	), strings.Index(
		out,
		"Hint:",
	), strings.Index(
		out,
		"Container:",
	)
	assert.True(
		t,
		cause < hint && hint < meta,
		"order must be cause → hint → details",
	)
	// A change carries its age.
	assert.Regexp(
		t,
		regexp.MustCompile(
			`A recent change may be related: deployment dev/api updated \d+[smh] ago`,
		),
		out,
	)
	// Short names, no registry, no domain.
	assert.Contains(t, out, "Image: api:1.2.0")
	assert.NotContains(t, out, "amazonaws")
	assert.Contains(t, out, "Node: ip-10-0-81-7")
	assert.NotContains(t, out, "compute.internal")
	// No blank lines from empty sections.
	assert.NotContains(t, out, "\n\n")
}

func TestTextRendererGroupSubjectIsUsedVerbatim(t *testing.T) {
	r := NewReportBuilder("").Build(&model.Incident{
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
	assert.Contains(t, out, "Seen: ×6")
	assert.Contains(t, out, "Peak: 6 pods")
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
		"✅ Resolved — Pod not ready — dev/api · "+
			"ContainersNotReady\nlasted 2m · 3 occurrences · node ip-10-0-81-7",
		out,
	)
}
