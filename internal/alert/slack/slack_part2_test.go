package slack

import (
	"errors"
	"strings"
	"testing"
	"time"

	slackClient "github.com/slack-go/slack"
	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/message"
	"github.com/abahmed/kwatch/internal/model"
)

func TestSendIncidentTokenCreate(t *testing.T) {
	assert := assert.New(t)

	s := &Slack{
		channel: "#alerts",
		appCfg:  &config.App{ClusterName: "dev"},
	}

	var capturedBlocks *slackClient.Blocks
	var capturedThreadTS string
	s.postBlocksFn = func(
		blocks *slackClient.Blocks, threadTS string,
	) (string, error) {
		capturedBlocks = blocks
		capturedThreadTS = threadTS
		return "12345.67890", nil
	}

	err := s.SendIncident(testIncident(), model.ActionCreate)
	assert.Nil(err)
	assert.NotNil(capturedBlocks)
	assert.Empty(capturedThreadTS)

	// verify threadMap was populated
	s.mu.Lock()
	ts, ok := s.threadMap["default:deploy-1:CrashLoopBackOff"]
	s.mu.Unlock()
	assert.True(ok)
	assert.Equal("12345.67890", ts)
}

func TestSendIncidentTokenUpdate(t *testing.T) {
	assert := assert.New(t)

	s := &Slack{
		channel: "#alerts",
		appCfg:  &config.App{ClusterName: "dev"},
		threadMap: map[string]string{
			"default:deploy-1:CrashLoopBackOff": "12345.67890",
		},
	}

	var capturedBlocks *slackClient.Blocks
	var capturedThreadTS string
	s.postBlocksFn = func(
		blocks *slackClient.Blocks, threadTS string,
	) (string, error) {
		capturedBlocks = blocks
		capturedThreadTS = threadTS
		return "12345.67891", nil
	}

	err := s.SendIncident(testIncident(), model.ActionUpdate)
	assert.Nil(err)
	assert.NotNil(capturedBlocks)
	assert.Equal("12345.67890", capturedThreadTS)
}

func TestSendIncidentTokenUpdateNoThread(t *testing.T) {
	assert := assert.New(t)

	s := &Slack{
		channel: "#alerts",
		appCfg:  &config.App{ClusterName: "dev"},
		// no threadMap set — first update should still work (no thread)
	}

	var capturedThreadTS string
	s.postBlocksFn = func(
		_ *slackClient.Blocks, threadTS string,
	) (string, error) {
		capturedThreadTS = threadTS
		return "12345.67890", nil
	}

	err := s.SendIncident(testIncident(), model.ActionUpdate)
	assert.Nil(err)
	assert.Empty(capturedThreadTS)
}

func TestSendIncidentTokenSkip(t *testing.T) {
	assert := assert.New(t)

	s := &Slack{
		channel: "#alerts",
		appCfg:  &config.App{ClusterName: "dev"},
	}

	called := false
	s.postBlocksFn = func(_ *slackClient.Blocks, _ string) (string, error) {
		called = true
		return "", nil
	}

	err := s.SendIncident(testIncident(), model.ActionSkip)
	assert.Nil(err)
	assert.False(called)
}

// --- buildIncidentBlocks ---

func TestBuildIncidentBlocks(t *testing.T) {
	assert := assert.New(t)

	inc := testIncident()
	blocks := buildIncidentBlocks(inc, &config.App{ClusterName: "prod-cluster"})

	assert.NotNil(blocks)
	assert.Greater(len(blocks.BlockSet), 0)
}

func TestBuildIncidentUpdateBlocks(t *testing.T) {
	assert := assert.New(t)

	inc := testIncident()
	blocks := buildIncidentUpdateBlocks(inc)

	assert.NotNil(blocks)
	// header (pod has Resources)
	assert.Equal(1, len(blocks.BlockSet))
}

func TestFormatIncidentText(t *testing.T) {
	assert := assert.New(t)

	inc := testIncident()
	text := formatIncidentText(inc, model.ActionCreate)
	assert.Contains(text, "CrashLoopBackOff")
	assert.Contains(text, "deploy-1")

	textUpdate := formatIncidentText(inc, model.ActionUpdate)
	assert.Contains(textUpdate, "CrashLoopBackOff")
}

func TestBuildIncidentBlocksWithLogsEvents(t *testing.T) {
	assert := assert.New(t)

	inc := testIncident()
	inc.Events = "Warning Unhealthy pod-1 liveness probe failed"
	inc.Logs = "Error: connection refused"
	inc.IncludeEvents = true
	inc.IncludeLogs = true

	blocks := buildIncidentBlocks(inc, &config.App{ClusterName: "prod-cluster"})

	assert.NotNil(blocks)
	foundEvents := false
	foundLogs := false
	for _, b := range blocks.BlockSet {
		if s, ok := b.(slackClient.SectionBlock); ok && s.Text != nil {
			if s.Text.Text == ":mag: *Events*" {
				foundEvents = true
			}
			if s.Text.Text == ":memo: *Logs*" {
				foundLogs = true
			}
		}
	}
	assert.True(foundEvents, "Events block should be present")
	assert.True(foundLogs, "Logs block should be present")
}

func TestBuildIncidentUpdateBlocksWithLogsEvents(t *testing.T) {
	assert := assert.New(t)

	inc := testIncident()
	inc.Events = "Warning BackOff restarting container"
	inc.Logs = "Error: server closed connection"
	inc.IncludeEvents = true
	inc.IncludeLogs = true

	blocks := buildIncidentUpdateBlocks(inc)

	assert.NotNil(blocks)
	assert.Greater(
		len(blocks.BlockSet),
		1,
		"update blocks should include Logs/Events sections",
	)
}

func TestFormatIncidentTextWithLogsEvents(t *testing.T) {
	assert := assert.New(t)

	inc := testIncident()
	inc.Events = "Warning Unhealthy"
	inc.Logs = "Error: timeout"
	inc.IncludeEvents = true
	inc.IncludeLogs = true

	text := formatIncidentText(inc, model.ActionCreate)
	assert.Contains(text, "Events:")
	assert.Contains(text, "Warning Unhealthy")
	assert.Contains(text, "Logs:")
	assert.Contains(text, "Error: timeout")
}

func TestFormatIncidentTextUpdateWithLogsEvents(t *testing.T) {
	assert := assert.New(t)

	inc := testIncident()
	inc.Events = "Warning BackOff"
	inc.Logs = "Error: crash"
	inc.IncludeEvents = true
	inc.IncludeLogs = true

	text := formatIncidentText(inc, model.ActionUpdate)
	assert.Contains(text, "Events:")
	assert.Contains(text, "Warning BackOff")
	assert.Contains(text, "Logs:")
	assert.Contains(text, "Error: crash")
}

// Incidents are keyed by owner, so one incident can name several replicas
// under Resources while the attached evidence came from exactly one of them.
// dev showed "api-...-gjwjp" under Resources and events describing
// "api-...-96p24"; the alert must say which pod it is showing.
func TestEvidenceIsAttributedToItsPod(t *testing.T) {
	base := func() *model.Incident {
		return &model.Incident{
			Subject: model.Subject{
				Name:      "api",
				Reason:    "ContainersNotReady",
				Namespace: "dev",
			},
			Evidence: model.Evidence{
				Events:        "[..] FailedScheduling no nodes available",
				IncludeEvents: true,
				EvidencePod:   "api-584ddc9849-96p24",
			},
		}

	}

	// Several replicas covered: the evidence pod must be named.
	multi := base()
	multi.Resources = map[string]bool{
		"api-584ddc9849-gjwjp": true,
		"api-584ddc9849-96p24": true,
	}
	title := evidenceTitle(":mag: *Events*", multi)
	if !strings.Contains(title, "api-584ddc9849-96p24") {
		t.Errorf(
			"multi-pod incident must attribute its evidence, got %q",
			title,
		)
	}

	// A single pod that is the incident itself needs no redundant label.
	single := base()
	single.Name = "api-584ddc9849-96p24"
	single.Resources = map[string]bool{"api-584ddc9849-96p24": true}
	if got := evidenceTitle(":mag: *Events*", single); got != ":mag: *Events*" {
		t.Errorf(
			"single-pod incident should not repeat the pod name, got %q",
			got,
		)
	}

	// No recorded source: unchanged.
	none := base()
	none.EvidencePod = ""
	if got := evidenceTitle(":mag: *Events*", none); got != ":mag: *Events*" {
		t.Errorf("unattributed evidence must render unchanged, got %q", got)
	}
}

// Slack reports request and credential problems as bare error codes. Those
// cannot succeed on retry and must be marked permanent; rate limits keep
// their Retry-After; anything else stays transient.
func TestSlackErrorClassification(t *testing.T) {
	codes := []string{
		"invalid_blocks", "channel_not_found", "invalid_auth", "not_in_channel",
	}
	for _, code := range codes {
		err := wrapSlackRateLimit(errors.New(code))
		assert.True(t, event.IsPermanent(err), "%s must be permanent", code)
	}
	transient := wrapSlackRateLimit(errors.New("connection reset by peer"))
	assert.False(
		t,
		event.IsPermanent(transient),
		"network errors stay retryable",
	)
	assert.Nil(t, wrapSlackRateLimit(nil))
}

// The rich (token) Slack path builds its blocks from the incident alone and
// used to drop the diagnosis entirely. It must now render cause, impact and
// recent changes, and stay unchanged when there is no diagnosis.
func TestIncidentBlocksRenderDiagnosis(t *testing.T) {
	app := &config.App{ClusterName: "dev"}
	inc := &model.Incident{
		Subject: model.Subject{
			Reason:    "ContainersNotReady",
			Name:      "api",
			Namespace: "dev",
		},
		Status: model.Status{
			Count: 1,
		},
	}

	flatten := func(b *slackClient.Blocks) string {
		var sb strings.Builder
		for _, blk := range b.BlockSet {
			if sec, ok := blk.(slackClient.SectionBlock); ok &&
				sec.Text != nil {
				sb.WriteString(sec.Text.Text)
				sb.WriteString("\n")
			}
		}
		return sb.String()
	}

	ins := &insight.Insight{
		Cause:   "node ip-10-0-81-7 may be unhealthy",
		Pattern: "node_failure",
		Impact:  "12 pods on this node, affecting 3 services",
		RecentChanges: []kwcontext.Change{
			{
				Resource:  "configmap",
				Namespace: "dev",
				Name:      "api-config",
				Type:      kwcontext.ChangeUpdate,
				Timestamp: time.Now().Add(-3 * time.Minute),
			},
		},
	}
	text := flatten(buildIncidentBlocksWithInsight(inc, app, ins))
	assert.NotContains(t, text, "Why:")
	assert.Contains(t, text, "node ip-10-0-81-7 may be unhealthy")
	assert.Contains(
		t,
		text,
		message.Narrative(reportFor(inc, model.ActionCreate, ins, app)),
	)
	assert.Contains(t, text, "12 pods on this node")
	assert.Contains(t, text, "configmap dev/api-config update")

	plain := flatten(buildIncidentBlocksWithInsight(inc, app, nil))
	assert.NotContains(t, plain, "Diagnosis", "no diagnosis, no section")
	assert.Equal(
		t,
		plain,
		flatten(buildIncidentBlocks(inc, app)),
		"the old entry point is unchanged",
	)
}
