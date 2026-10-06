package delivery

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
		map[string]string{"known": "ISSUE-1"},
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
		map[string]bool{"untracked": true},
	}
	entry := &providerEntry{provider: tracker}

	assert.False(t, resolveWithoutMapping(entry, resolveJob("untracked")))
	assert.True(t, resolveWithoutMapping(entry, resolveJob("unknown")))
}
