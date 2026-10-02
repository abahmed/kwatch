package delivery

import (
	"context"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/notification"
)

type incidentRecorder struct {
	fakeProvider
	incidents []notification.Message
	messages  []string
}

func (p *incidentRecorder) SendIncident(
	_ context.Context, m notification.Message,
) error {
	p.incidents = append(p.incidents, m)
	return nil
}

func (p *incidentRecorder) SendMessage(_ context.Context, text string) error {
	p.messages = append(p.messages, text)
	return nil
}

func deliverIncident(t *testing.T, entry providerEntry, job deliverJob) {
	t.Helper()
	entry.retry = retryConfig{maxAttempts: 1, delay: time.Millisecond}
	am := managerWithEntries([]providerEntry{entry})
	am.deliverOne(context.Background(), &managerEntries(am)[0], job)
}

func TestDispatchIncidentCallsSendIncident(t *testing.T) {
	provider := &incidentRecorder{}

	deliverIncident(t, providerEntry{provider: provider},
		incidentJob("api", "default"))

	require.Len(t, provider.incidents, 1)
	assert.Empty(t, provider.messages)
	got := provider.incidents[0]
	assert.Equal(t, "api", got.Key)
	assert.Equal(t, "🔴 api failed", got.Note, "Note falls back to Text")
}

func TestDispatchIncidentTruncatesNoteAtPayloadLimit(t *testing.T) {
	provider := &incidentRecorder{}
	job := incidentJob("api", "default")
	job.incident.Note = "🔴 " + strings.Repeat("x", 100)
	job.incident.Short = "🔴 " + strings.Repeat("y", 100)

	deliverIncident(t, providerEntry{provider: provider, maxBytes: 40}, job)

	require.Len(t, provider.incidents, 1)
	got := provider.incidents[0]
	assert.LessOrEqual(t, len(got.Note), 40)
	assert.LessOrEqual(t, len(got.Short), 40)
	assert.True(t, strings.HasPrefix(got.Note, "🔴 xxx"))
	assert.True(t, strings.HasSuffix(got.Note, "…"))
	again := prepareIncident(*job.incident, nil, "", 40)
	assert.Equal(t, got.Note, again.Note, "truncation is deterministic")
}

func TestPrepareIncidentKeepsMarkerFirstOnFallback(t *testing.T) {
	m := notification.Message{Note: "🔴 api is down."}

	got := prepareIncident(m, nil, "Slack", 0)

	assert.True(t, strings.HasPrefix(got.Note, "🔴 api is down."))
	assert.Contains(t, got.Note, "fallback because Slack failed")
}

func TestIncidentNoteAppliesReasonTemplate(t *testing.T) {
	m := notification.Message{
		Title: "api failed", Note: "🔴 api failed.",
		Route: notification.Route{Reasons: []string{"OOMKilled"}},
	}
	tmpl := template.Must(template.New("t").Parse(
		"custom: {{.Message.Title}} / {{.Text}}"))

	assert.Equal(t, "custom: api failed / 🔴 api failed.", incidentNote(m,
		map[string]*template.Template{"oomkilled": tmpl}))
	assert.Equal(t, m.Note, incidentNote(m, nil))
}

func TestIncidentNoteFallsBackWhenTemplateFails(t *testing.T) {
	m := notification.Message{
		Title: "api failed",
		Route: notification.Route{Reasons: []string{"Error"}},
	}
	broken := template.Must(template.New("t").Parse("{{.Missing.Field}}"))

	assert.Equal(t, notification.Text(m), incidentNote(m,
		map[string]*template.Template{"error": broken}))
}

func TestRoutedToMatchesIncidentRoute(t *testing.T) {
	routes := []config.AlertRoute{{Namespaces: []string{"ops"}}}

	assert.True(t, routedTo(routes, incidentJob("a", "ops")))
	assert.False(t, routedTo(routes, incidentJob("a", "default")))
	assert.True(t, routedTo(nil, incidentJob("a", "default")))
	assert.True(t, routedTo(routes, deliverJob{kind: jobMessage}))
}

func TestOfferQueuedJobSupersedesSameConversation(t *testing.T) {
	queue := make(chan deliverJob, 2)
	require.True(t, offerQueuedJob(queue, incidentJob("a", "ns")).accepted)
	require.True(t, offerQueuedJob(queue, incidentJob("b", "ns")).accepted)
	newer := incidentJob("a", "ns")
	newer.incident.Revision = 2

	require.True(t, offerQueuedJob(queue, newer).accepted)

	first, second := <-queue, <-queue
	assert.Equal(t, "a", first.incident.Key)
	assert.Equal(t, 2, first.incident.Revision, "newer revision must win")
	assert.Equal(t, "b", second.incident.Key)
}

func TestOfferQueuedJobRejectsUnrelatedOverflow(t *testing.T) {
	queue := make(chan deliverJob, 1)
	require.True(t, offerQueuedJob(queue, incidentJob("a", "ns")).accepted)

	assert.False(t, offerQueuedJob(queue, incidentJob("b", "ns")).accepted)
	message := deliverJob{kind: jobMessage}
	assert.False(t, offerQueuedJob(queue, message).accepted)
	assert.Equal(t, "a", (<-queue).incident.Key)
}
