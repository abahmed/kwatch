package announce

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/notification"
)

// dailyCollector is a Collector whose today's counts are *today, past
// the startup note.
func dailyCollector(
	proNote bool, today *feature.WithheldCounts,
) (*Collector, *[]proSent) {
	c, sent := proChecksCollector(proNote,
		withheldOf(feature.WithheldCounts{}))
	c.env.WithheldToday = func(time.Time) feature.WithheldCounts {
		return *today
	}
	c.sendProChecks(context.Background(), proChecksAt)
	return c, sent
}

func TestDailyProChecksSendsOnceADay(t *testing.T) {
	today := feature.WithheldCounts{}
	c, sent := dailyCollector(true, &today)
	ctx := context.Background()
	today = feature.WithheldCounts{2, 0, 0, 0}
	c.SendDailyProChecks(ctx, proChecksAt.Add(time.Hour))
	require.Len(t, *sent, 1)
	got := (*sent)[0]
	assert.EqualValues(t, "pro checks", got.d.Reason)
	assert.True(t, got.m.IsNotice())
	assert.Equal(t, notification.MarkerLow, got.m.Marker)
	assert.Contains(t, got.m.Note,
		"Pro checks found 2 issues today: 2 readiness probes.")

	today = feature.WithheldCounts{3, 0, 0, 0}
	c.SendDailyProChecks(ctx, proChecksAt.Add(2*time.Hour))
	assert.Len(t, *sent, 1, "one note per day, even with news")

	next := proChecksAt.Add(24 * time.Hour)
	today = feature.WithheldCounts{0, 1, 0, 0}
	c.SendDailyProChecks(ctx, next)
	require.Len(t, *sent, 2)
	assert.Contains(t, (*sent)[1].m.Note, "found 1 issue today")
}

func TestDailyProChecksSilentWhenNothingIsNew(t *testing.T) {
	today := feature.WithheldCounts{1, 0, 0, 0}
	c, sent := dailyCollector(true, &today)
	*sent = nil
	c.SendDailyProChecks(context.Background(),
		proChecksAt.Add(time.Hour))
	assert.Empty(t, *sent, "the startup note already said these")
}

func TestDailyProChecksSilentWithoutFindings(t *testing.T) {
	today := feature.WithheldCounts{}
	c, sent := dailyCollector(true, &today)
	c.SendDailyProChecks(context.Background(), proChecksAt.Add(time.Hour))
	c.SendDailyProChecks(context.Background(),
		proChecksAt.Add(30*time.Hour))
	assert.Empty(t, *sent)
}

func TestDailyProChecksSilentWhenHintsAreHidden(t *testing.T) {
	today := feature.WithheldCounts{}
	c, sent := dailyCollector(false, &today)
	today = feature.WithheldCounts{4, 0, 0, 0}
	c.SendDailyProChecks(context.Background(),
		proChecksAt.Add(30*time.Hour))
	assert.Empty(t, *sent)
}

func TestDailyProChecksWaitsForTheStartupNote(t *testing.T) {
	c, sent := proChecksCollector(true,
		withheldOf(feature.WithheldCounts{}))
	c.env.WithheldToday = withheldOf(feature.WithheldCounts{1, 0, 0, 0})
	c.SendDailyProChecks(context.Background(), proChecksAt)
	assert.Empty(t, *sent)
}
