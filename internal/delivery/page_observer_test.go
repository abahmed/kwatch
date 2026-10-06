package delivery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

func pagerEntryFor(p Provider) []providerEntry {
	return []providerEntry{{provider: p,
		retry: retryConfig{maxAttempts: 1, delay: time.Millisecond}}}
}

// A message the pager rejects for good leaves no alert open there, so
// the pipeline is told to stop counting the incident as paged.
func TestPermanentPagerFailureTellsThePipeline(t *testing.T) {
	pager := &skippingProvider{errorRecorderProvider{name: "Pager",
		err: transport.Permanent(errors.New("rejected"))}}
	m := newTestManager()
	var lost []string
	m.AttachPageObserver(func(key string) { lost = append(lost, key) })
	setManagerEntries(m, pagerEntryFor(pager))
	entries := managerEntries(m)

	m.deliverOne(context.Background(), &entries[0], incidentJob("a", "ns"))

	assert.Equal(t, []string{"a"}, lost)
}

// Once a pager accepted a message of the incident, its alert is open
// and a later rejected update does not say otherwise.
func TestRejectedUpdateAfterAcceptedPageKeepsItOpen(t *testing.T) {
	pager := &skippingProvider{errorRecorderProvider{name: "Pager"}}
	m := newTestManager()
	var lost []string
	m.AttachPageObserver(func(key string) { lost = append(lost, key) })
	setManagerEntries(m, pagerEntryFor(pager))
	entries := managerEntries(m)

	m.deliverOne(context.Background(), &entries[0], incidentJob("a", "ns"))
	pager.err = transport.Permanent(errors.New("rejected"))
	m.deliverOne(context.Background(), &entries[0], incidentJob("a", "ns"))

	assert.Empty(t, lost)
}

// A chat provider failing for good says nothing about the pagers.
func TestChatFailureDoesNotTouchPagedState(t *testing.T) {
	chat := &errorRecorderProvider{name: "Chat",
		err: transport.Permanent(errors.New("rejected"))}
	m := newTestManager()
	m.AttachPageObserver(func(string) { t.Fatal("not a pager") })
	setManagerEntries(m, pagerEntryFor(chat))
	entries := managerEntries(m)

	m.deliverOne(context.Background(), &entries[0], incidentJob("a", "ns"))
}

// The accepted-page ledger lives in memory. After a restart a rejected
// update finds it empty, and says nothing about the alert that is still
// open; the resolve then still goes to the pager.
func TestRejectedUpdateAfterRestartKeepsPagedFlag(t *testing.T) {
	pager := &skippingProvider{errorRecorderProvider{name: "Pager",
		err: transport.Permanent(errors.New("rejected"))}}
	m := newTestManager()
	var lost []string
	m.AttachPageObserver(func(key string) { lost = append(lost, key) })
	setManagerEntries(m, pagerEntryFor(pager))
	entries := managerEntries(m)

	update := incidentJob("a", "ns")
	update.incident.Revision = 3
	m.deliverOne(context.Background(), &entries[0], update)
	assert.Empty(t, lost, "an update must not clear the paged flag")

	pager.err = nil
	resolve := incidentJob("a", "ns")
	resolve.incident.Status = notification.StatusResolved
	acceptsResolve := acceptsPagingOnly(entries[0], resolve)
	assert.True(t, acceptsResolve, "resolve still goes to the pager")
}

// A resolve closes the alert, so a recurrence under the same key starts
// fresh: if its opening is then lost for good, the pipeline is told.
func TestRecurrenceAfterResolveIsJudgedFresh(t *testing.T) {
	pager := &skippingProvider{errorRecorderProvider{name: "Pager"}}
	m := newTestManager()
	var lost []string
	m.AttachPageObserver(func(key string) { lost = append(lost, key) })
	setManagerEntries(m, pagerEntryFor(pager))
	entries := managerEntries(m)

	m.deliverOne(context.Background(), &entries[0], incidentJob("a", "ns"))
	resolve := incidentJob("a", "ns")
	resolve.incident.Status = notification.StatusResolved
	m.deliverOne(context.Background(), &entries[0], resolve)

	pager.err = transport.Permanent(errors.New("rejected"))
	m.deliverOne(context.Background(), &entries[0], incidentJob("a", "ns"))

	assert.Equal(t, []string{"a"}, lost)
}
