package storage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var logStart = time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)

func minute(n int) time.Time {
	return logStart.Add(time.Duration(n) * time.Minute)
}

// collect returns every entry of entity in [since, until).
func collect(
	t *testing.T, l Log[string], entity string, since, until time.Time,
) []string {
	t.Helper()
	var out []string
	require.NoError(t, l.Range(entity, since, until,
		func(_ time.Time, v string) error {
			out = append(out, v)
			return nil
		}))
	return out
}

func TestLogBucketsRoundTrip(t *testing.T) {
	s := openClaimed(t, newFakeClock())
	logs := map[Bucket]Log[string]{
		Changes:  ChangeLog[string](s),
		Evidence: EvidenceLog[string](s),
		Timeline: TimelineLog[string](s),
		Audit:    AuditLog[string](s),
	}
	for bucket, l := range logs {
		require.NoError(t, l.Append("pod/a", minute(1), "second"))
		require.NoError(t, l.Append("pod/a", minute(0), "first"))
		require.NoError(t, l.Append("pod/ab", minute(0), "other"))

		assert.Equal(t, []string{"first", "second"},
			collect(t, l, "pod/a", time.Time{}, time.Time{}), bucket)
	}
}

func TestLogRangeReturnsEntryTimes(t *testing.T) {
	l := ChangeLog[string](openClaimed(t, newFakeClock()))
	require.NoError(t, l.Append("pod", minute(3), "x"))
	var times []time.Time

	require.NoError(t, l.Range("pod", time.Time{}, time.Time{},
		func(at time.Time, _ string) error {
			times = append(times, at)
			return nil
		}))

	require.Len(t, times, 1)
	assert.True(t, times[0].Equal(minute(3)))
}

func TestLogRangeBounds(t *testing.T) {
	l := TimelineLog[string](openClaimed(t, newFakeClock()))
	for i, v := range []string{"e0", "e1", "e2", "e3"} {
		require.NoError(t, l.Append("inc", minute(i), v))
	}
	cases := map[string]struct {
		since, until time.Time
		want         []string
	}{
		"closed":      {minute(1), minute(3), []string{"e1", "e2"}},
		"open since":  {time.Time{}, minute(2), []string{"e0", "e1"}},
		"open until":  {minute(2), time.Time{}, []string{"e2", "e3"}},
		"both open":   {time.Time{}, time.Time{}, []string{"e0", "e1", "e2", "e3"}},
		"empty range": {minute(2), minute(2), nil},
	}
	for name, tc := range cases {
		assert.Equal(t, tc.want,
			collect(t, l, "inc", tc.since, tc.until), name)
	}
}

func TestLogSameTimeKeepsAppendOrder(t *testing.T) {
	l := AuditLog[string](openClaimed(t, newFakeClock()))
	for _, v := range []string{"a", "b", "c"} {
		require.NoError(t, l.Append("decisions", minute(0), v))
	}

	assert.Equal(t, []string{"a", "b", "c"},
		collect(t, l, "decisions", time.Time{}, time.Time{}))
}

func TestLogRejectsBadEntity(t *testing.T) {
	l := ChangeLog[string](openClaimed(t, newFakeClock()))

	assert.ErrorIs(t, l.Append("", minute(0), "v"), ErrEmptyKey)
	assert.ErrorIs(t, l.Append("a\x00b", minute(0), "v"), ErrBadEntity)
}
