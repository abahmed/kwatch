package alert

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
)

func TestFanOutSaturatedQueueRecordsDeadLetter(t *testing.T) {
	am := &AlertManager{}
	ch := make(chan deliverJob, channelCap)
	inc := &model.Incident{
		Subject: model.Subject{
			Key:    "arriving-job",
			Name:   "n1",
			Reason: "Error",
		},
	}
	for i := 0; i < channelCap; i++ {
		ch <- deliverJob{inc: &model.Incident{
			Subject: model.Subject{
				Key: model.IncidentKey(fmt.Sprintf("queued-%d", i)),
			},
		}}
	}
	am.entries = []providerEntry{{
		provider: &fakeProvider{},
		ch:       ch,
	}}

	am.mu.Lock()
	am.fanOut(deliverJob{inc: inc, action: model.ActionCreate})
	am.mu.Unlock()

	// Under saturation the already-queued notifications are kept — they are
	// the earlier, more diagnostic ones — and the arriving job is dead-
	// lettered instead. Evicting the oldest could discard an incident's
	// CREATE while keeping a later UPDATE for the same incident.
	assert.Len(t, ch, channelCap, "queue stays full; nothing was evicted")

	first := <-ch
	assert.Equal(t, model.IncidentKey("queued-0"), first.inc.Key,
		"the earliest queued notification must survive")

	dl := am.DeadLetters()
	dlList, ok := dl.([]DeadLetterEntry)
	require.True(t, ok)
	assert.Len(t, dlList, 1)
	assert.Equal(t, "arriving-job", dlList[0].Key,
		"the job that could not be queued is the one recorded")
	assert.Contains(t, dlList[0].Error, "queue saturated")
}

// Permanent failures are not retried and must not block later alerts.
func TestSendWithRetryStopsOnPermanentError(t *testing.T) {
	attempts := 0
	err := sendWithRetry(context.Background(), func() error {
		attempts++
		return event.Permanent(errors.New("invalid_blocks"))
	}, retryConfig{maxAttempts: 5, delay: time.Millisecond}, "test")
	require.Error(t, err)
	assert.Equal(t, 1, attempts, "a permanent error must not be retried")
	assert.True(
		t,
		event.IsPermanent(err),
		"the permanent marker must survive to the caller",
	)

	// A transient error still gets every attempt.
	attempts = 0
	_ = sendWithRetry(context.Background(), func() error {
		attempts++
		return errors.New("connection reset")
	}, retryConfig{maxAttempts: 3, delay: time.Millisecond}, "test")
	assert.Equal(t, 3, attempts, "a transient error is retried to maxAttempts")
}

type fakeInsightProvider struct {
	fakeProvider
	gotInsight   *insight.Insight
	plainCalled  bool
	insightCalls int
}

func (p *fakeInsightProvider) SendIncident(
	*model.Incident,
	model.IncidentAction,
) error {
	p.plainCalled = true
	return nil
}

func (p *fakeInsightProvider) SendIncidentWithInsight(
	_ *model.Incident,
	_ model.IncidentAction,
	ins *insight.Insight,
) error {
	p.insightCalls++
	p.gotInsight = ins
	return nil
}

// A provider that can render a diagnosis must be handed one. Falling back to
// the plain SendIncident would silently drop the cause, impact and changes.
func TestDeliverOnePrefersInsightCapableProvider(t *testing.T) {
	fp := &fakeInsightProvider{}
	am := &AlertManager{}
	entry := providerEntry{provider: fp, retry: retryConfig{maxAttempts: 1}}
	inc := &model.Incident{
		Subject: model.Subject{
			Key:    "ns:dep:Error:",
			Reason: "Error",
			Name:   "dep",
		},
	}
	ins := &insight.Insight{
		Cause:   "node worker-2 may be unhealthy",
		Pattern: "node_failure",
	}

	am.deliverOne(context.Background(), &entry, inc, model.ActionCreate, ins)

	assert.Equal(t, 1, fp.insightCalls)
	assert.False(
		t,
		fp.plainCalled,
		"the insight-aware path must be used, not the plain one",
	)
	require.NotNil(t, fp.gotInsight)
	assert.Equal(t, "node worker-2 may be unhealthy", fp.gotInsight.Cause)
}
