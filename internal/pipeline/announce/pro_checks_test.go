package announce

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/notification/compose"
)

var proChecksAt = time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)

type proSent struct {
	d incident.Decision
	m notification.Message
}

// proChecksCollector builds a Collector whose Sink records what it gets.
func proChecksCollector(
	proNote bool, withheld func(time.Time) feature.WithheldCounts,
) (*Collector, *[]proSent) {
	var sent []proSent
	c := New(Env{
		Messages: compose.Writer{ProNote: proNote},
		Withheld: withheld,
		Sink: func(_ context.Context, d incident.Decision,
			m notification.Message) {
			sent = append(sent, proSent{d, m})
		},
	})
	return c, &sent
}

func withheldOf(
	counts feature.WithheldCounts,
) func(time.Time) feature.WithheldCounts {
	return func(time.Time) feature.WithheldCounts { return counts }
}

func TestSendProChecksNoticeContent(t *testing.T) {
	c, sent := proChecksCollector(true,
		withheldOf(feature.WithheldCounts{2, 1, 0, 0}))
	c.sendProChecks(context.Background(), proChecksAt)

	require.Len(t, *sent, 1)
	got := (*sent)[0]
	assert.EqualValues(t, "pro checks", got.d.Reason)
	assert.True(t, got.m.IsNotice())
	assert.Equal(t, notification.MarkerLow, got.m.Marker)
	assert.Contains(t, got.m.Note, "Pro checks found 3 issues since "+
		"kwatch started: 2 readiness probes, 1 shared cause.")
	assert.Contains(t, got.m.Note, "kwatch.dev/pro")
}

func TestSendProChecksSingularIssue(t *testing.T) {
	c, sent := proChecksCollector(true,
		withheldOf(feature.WithheldCounts{0, 0, 0, 1}))
	c.sendProChecks(context.Background(), proChecksAt)

	require.Len(t, *sent, 1)
	assert.Contains(t, (*sent)[0].m.Note, "found 1 issue since")
}

func TestSendProChecksAsksForCountsAtNow(t *testing.T) {
	var asked time.Time
	c, _ := proChecksCollector(true,
		func(now time.Time) feature.WithheldCounts {
			asked = now
			return feature.WithheldCounts{}
		})
	c.sendProChecks(context.Background(), proChecksAt)
	assert.Equal(t, proChecksAt, asked)
}

func TestSendProChecksSilentWithoutFindings(t *testing.T) {
	c, sent := proChecksCollector(true,
		withheldOf(feature.WithheldCounts{}))
	c.sendProChecks(context.Background(), proChecksAt)
	assert.Empty(t, *sent)
}

func TestSendProChecksSilentWithoutProHints(t *testing.T) {
	asked := false
	c, sent := proChecksCollector(false,
		func(time.Time) feature.WithheldCounts {
			asked = true
			return feature.WithheldCounts{3, 0, 0, 0}
		})
	c.sendProChecks(context.Background(), proChecksAt)
	assert.Empty(t, *sent)
	assert.False(t, asked, "counts are not read without hints")
}

func TestSendProChecksSilentWithoutCounter(t *testing.T) {
	c, sent := proChecksCollector(true, nil)
	c.sendProChecks(context.Background(), proChecksAt)
	assert.Empty(t, *sent)
}

func TestSendProChecksSendsOnEveryCall(t *testing.T) {
	// The caller (finishStartup) runs once per start; the function
	// itself keeps no memory of what it sent.
	c, sent := proChecksCollector(true,
		withheldOf(feature.WithheldCounts{1, 0, 0, 0}))
	c.sendProChecks(context.Background(), proChecksAt)
	c.sendProChecks(context.Background(), proChecksAt)
	assert.Len(t, *sent, 2)
}
