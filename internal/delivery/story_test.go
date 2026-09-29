package delivery

import (
	"context"
	"testing"
	"text/template"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/notice"
)

type storyRecorder struct {
	fakeProvider
	stories []notice.Message
}

func (p *storyRecorder) SendStory(_ context.Context, m notice.Message) error {
	p.stories = append(p.stories, m)
	return nil
}

type eventRecorder struct {
	fakeProvider
	events []*event.Event
}

func (p *eventRecorder) SendEvent(_ context.Context, ev *event.Event) error {
	p.events = append(p.events, ev)
	return nil
}

func (p *eventRecorder) UsesEventDelivery() {}

func deliverStory(t *testing.T, provider Provider, job deliverJob) {
	t.Helper()
	am := managerWithEntries([]providerEntry{{
		provider: provider,
		retry:    retryConfig{maxAttempts: 1, delay: time.Millisecond},
	}})
	am.deliverOne(context.Background(), &managerEntries(am)[0], job)
}

func TestDispatchStoryUsesNativeRendering(t *testing.T) {
	provider := &storyRecorder{}

	deliverStory(t, provider, storyJob("api", "default"))

	require.Len(t, provider.stories, 1)
	assert.Equal(t, "api", provider.stories[0].Key)
}

func TestDispatchStoryMapsToPagingEvent(t *testing.T) {
	provider := &eventRecorder{}
	job := storyJob("api", "default")
	job.story.Status = notice.StatusResolved

	deliverStory(t, provider, job)

	require.Len(t, provider.events, 1)
	ev := provider.events[0]
	assert.Equal(t, "api", ev.DedupKey)
	assert.Equal(t, "resolved", ev.Action)
	assert.Contains(t, ev.Narrative, "api failed")
}

func TestStoryTextAppliesReasonTemplate(t *testing.T) {
	m := notice.Message{
		Title: "api failed",
		Route: notice.Route{Reasons: []string{"OOMKilled"}},
	}
	tmpl := template.Must(template.New("t").Parse("custom: {{.Message.Title}}"))

	assert.Equal(t, "custom: api failed", storyText(m,
		map[string]*template.Template{"oomkilled": tmpl}))
	assert.Equal(t, notice.Text(m), storyText(m, nil))
}

func TestStoryTextFallsBackWhenTemplateFails(t *testing.T) {
	m := notice.Message{
		Title: "api failed",
		Route: notice.Route{Reasons: []string{"Error"}},
	}
	broken := template.Must(template.New("t").Parse("{{.Missing.Field}}"))

	assert.Equal(t, notice.Text(m), storyText(m,
		map[string]*template.Template{"error": broken}))
}

func TestRoutedToMatchesStoryRoute(t *testing.T) {
	routes := []config.AlertRoute{{Namespaces: []string{"ops"}}}

	assert.True(t, routedTo(routes, storyJob("a", "ops")))
	assert.False(t, routedTo(routes, storyJob("a", "default")))
	assert.True(t, routedTo(nil, storyJob("a", "default")))
	assert.True(t, routedTo(routes, deliverJob{kind: jobMessage}))
}

func TestOfferQueuedJobSupersedesSameConversation(t *testing.T) {
	queue := make(chan deliverJob, 2)
	require.True(t, offerQueuedJob(queue, storyJob("a", "ns")))
	require.True(t, offerQueuedJob(queue, storyJob("b", "ns")))
	newer := storyJob("a", "ns")
	newer.story.Revision = 2

	require.True(t, offerQueuedJob(queue, newer))

	first, second := <-queue, <-queue
	assert.Equal(t, "a", first.story.Key)
	assert.Equal(t, 2, first.story.Revision, "newer revision must win")
	assert.Equal(t, "b", second.story.Key)
}

func TestOfferQueuedJobRejectsUnrelatedOverflow(t *testing.T) {
	queue := make(chan deliverJob, 1)
	require.True(t, offerQueuedJob(queue, storyJob("a", "ns")))

	assert.False(t, offerQueuedJob(queue, storyJob("b", "ns")))
	assert.False(t, offerQueuedJob(queue, deliverJob{kind: jobMessage}))
	assert.Equal(t, "a", (<-queue).story.Key)
}
