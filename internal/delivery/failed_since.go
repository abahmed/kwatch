package delivery

import (
	"sync"
	"time"
)

// failWindow is how far back FailedSince can count. failBuckets holds
// one bucket per minute of it, plus the current minute, so a window that
// starts part way through a minute is still covered.
const (
	failWindow  = time.Hour
	failBuckets = int(failWindow/time.Minute) + 1
)

// FailedSince counts the provider sends that failed at or after t: those
// given up or handed to the fallback and those to be retried. Every kind
// of send counts (incident messages, summaries, roll-ups, digests and
// plain messages), however many one incident had. It counts by the
// minute and looks back at most an hour: a failure in the minute that
// holds t counts, one older than an hour does not.
func (m *Manager) FailedSince(t time.Time) int {
	// A manager built without a clock (a zero Manager) has never sent:
	// the hour after t covers all it could hold.
	now := t.Add(failWindow)
	if m.now != nil {
		now = m.now()
	}
	return m.failures.since(t, now)
}

// failCounter counts failed provider sends per minute for the last
// hour. Its memory is fixed however many sends fail. The zero value is
// ready to use.
type failCounter struct {
	mu      sync.Mutex
	minute  [failBuckets]int64
	buckets [failBuckets]int
}

// add counts one failed send at the given time.
func (c *failCounter) add(at time.Time) {
	minute := unixMinute(at)
	i := bucketIndex(minute)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.minute[i] != minute {
		c.minute[i], c.buckets[i] = minute, 0
	}
	c.buckets[i]++
}

// since counts the failures from the minute that holds t to now, no
// further back than failWindow.
func (c *failCounter) since(t, now time.Time) int {
	from := max(unixMinute(t), unixMinute(now)-int64(failBuckets)+1)
	to := unixMinute(now)
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for i := range c.buckets {
		if c.minute[i] >= from && c.minute[i] <= to {
			n += c.buckets[i]
		}
	}
	return n
}

func unixMinute(t time.Time) int64 {
	return t.Unix() / 60
}

func bucketIndex(minute int64) int {
	i := minute % int64(failBuckets)
	if i < 0 {
		i += int64(failBuckets)
	}
	return int(i)
}
