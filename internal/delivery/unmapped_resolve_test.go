package delivery

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/alert/issues"
	"github.com/abahmed/kwatch/internal/clock"
)

// issueProvider is a tracker backed by the real issue map, which keys
// issues by the incident's thread key, not its bare key.
type issueProvider struct {
	skippingProvider
	issues *issues.Map
}

func (p *issueProvider) HasThread(key string) bool {
	return p.issues.HasThread(key)
}

func (p *issueProvider) SnapshotThreads() map[string]string {
	return p.issues.SnapshotThreads()
}

func (p *issueProvider) RestoreThreads(saved map[string]string) {
	p.issues.RestoreThreads(saved)
}

type issueAPI struct{}

func (issueAPI) Name() string { return "Jira" }

func (issueAPI) Create(context.Context, string, string) (string, error) {
	return "ISSUE-1", nil
}

func (issueAPI) Comment(context.Context, string, string) error { return nil }

func (issueAPI) Close(context.Context, string, string) error { return nil }

// With the real issue map, a resolve of an incident that has an issue
// is not reported as unmapped, and one without an issue is.
func TestResolveOfAMappedIssueIsNotFlaggedWithTheRealMap(t *testing.T) {
	tracker := &issueProvider{
		skippingProvider{errorRecorderProvider{name: "Jira"}},
		issues.NewMap(clock.RealClock{}.Now),
	}
	entry := &providerEntry{provider: tracker}
	opened := incidentJob("inc-1", "shop")
	require.NoError(t, tracker.issues.Deliver(context.Background(),
		issueAPI{}, *opened.incident, "title", "body"))

	assert.False(t, resolveWithoutMapping(entry, resolveJob("inc-1")),
		"the incident has an issue: its resolve closes it")
	assert.True(t, resolveWithoutMapping(entry, resolveJob("inc-2")))
}

// trackerProvider is an issue tracker that keeps threads by key.
type trackerProvider struct {
	skippingProvider
	threads map[string]string
}

func (p *trackerProvider) SnapshotThreads() map[string]string {
	return p.threads
}

func (p *trackerProvider) RestoreThreads(map[string]string) {}

func TestResolveWithoutMappingIsFlagged(t *testing.T) {
	tracker := &trackerProvider{
		skippingProvider{errorRecorderProvider{name: "Jira"}},
		map[string]string{"kwatch-known": "ISSUE-1"},
	}
	entry := &providerEntry{provider: tracker}

	assert.True(t, resolveWithoutMapping(entry, resolveJob("unknown")))
	assert.False(t, resolveWithoutMapping(entry, resolveJob("known")))
	assert.False(t, resolveWithoutMapping(entry, incidentJob("unknown", "ns")),
		"only a resolve has an issue to close")

	chat := &providerEntry{provider: &errorRecorderProvider{name: "Chat"}}
	assert.False(t, resolveWithoutMapping(chat, resolveJob("unknown")))
}

// lookupProvider answers per key and fails if the whole map is copied.
type lookupProvider struct {
	skippingProvider
	tracked map[string]bool
}

func (p *lookupProvider) HasThread(key string) bool { return p.tracked[key] }

func (p *lookupProvider) SnapshotThreads() map[string]string {
	panic("a resolve must not snapshot the whole thread map")
}

func (p *lookupProvider) RestoreThreads(map[string]string) {}

// An incident the tracker holds only as an untracked marker is known to
// it, so its resolve is not reported, and no map is copied to find out.
func TestResolveUsesThePerKeyLookup(t *testing.T) {
	tracker := &lookupProvider{
		skippingProvider{errorRecorderProvider{name: "Jira"}},
		map[string]bool{"kwatch-untracked": true},
	}
	entry := &providerEntry{provider: tracker}

	assert.False(t, resolveWithoutMapping(entry, resolveJob("untracked")))
	assert.True(t, resolveWithoutMapping(entry, resolveJob("unknown")))
}
