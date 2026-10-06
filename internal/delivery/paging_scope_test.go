package delivery

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// A resolve of an incident that never reached the paging providers goes
// to chat only; a paging-only message goes to the paging providers only;
// everything else goes to both.
func TestAcceptsPagingOnlyAndSkipPaging(t *testing.T) {
	pager := providerEntry{provider: &skippingProvider{
		errorRecorderProvider{name: "Pager"}}}
	chat := providerEntry{provider: &errorRecorderProvider{name: "Chat"}}

	plain := incidentJob("k", "default")
	assert.True(t, acceptsPagingOnly(pager, plain))
	assert.True(t, acceptsPagingOnly(chat, plain))

	paging := incidentJob("k", "default")
	paging.incident.PagingOnly = true
	assert.True(t, acceptsPagingOnly(pager, paging))
	assert.False(t, acceptsPagingOnly(chat, paging))

	unpaged := incidentJob("k", "default")
	unpaged.incident.SkipPaging = true
	assert.False(t, acceptsPagingOnly(pager, unpaged),
		"no alert was opened, so none is closed")
	assert.True(t, acceptsPagingOnly(chat, unpaged))

	both := incidentJob("k", "default")
	both.incident.PagingOnly, both.incident.SkipPaging = true, true
	assert.False(t, acceptsPagingOnly(pager, both))
	assert.False(t, acceptsPagingOnly(chat, both))
}

// A fallback follows the same paging rules as a primary: a chat provider
// never takes over a paging-only message, and a pager never takes over a
// resolve that skips paging.
func TestDeliverFallbackHonoursPagingScope(t *testing.T) {
	m := newTestManager()
	pager := &providerEntry{provider: &skippingProvider{
		errorRecorderProvider{name: "Pager"}}}
	chat := &providerEntry{provider: &errorRecorderProvider{name: "Chat"}}

	paging := incidentJob("k", "default")
	paging.incident.PagingOnly = true
	err := m.deliverFallback(context.Background(), chat, "Pager", paging)
	assert.ErrorIs(t, err, errFallbackNotRouted,
		"chat must not receive a paging-only message")

	unpaged := incidentJob("k", "default")
	unpaged.incident.SkipPaging = true
	err = m.deliverFallback(context.Background(), pager, "Chat", unpaged)
	assert.ErrorIs(t, err, errFallbackNotRouted,
		"a pager must not close an alert it never opened")
}

type structuredProvider struct{ errorRecorderProvider }

func (*structuredProvider) ReceivesPagingOnly() bool { return true }

// A structured receiver takes paging-only closes on every path, and
// still takes plain messages and summaries.
func TestStructuredReceiverAcceptsPagingOnly(t *testing.T) {
	hook := providerEntry{provider: &structuredProvider{
		errorRecorderProvider{name: "Hook"}}}
	job := incidentJob("k", "default")
	job.incident.PagingOnly = true
	assert.True(t, acceptsPagingOnly(hook, job))
	assert.False(t, skipsPlainMessages(hook.provider))

	m := newTestManager()
	err := m.deliverFallback(context.Background(), &hook, "Chat", job)
	assert.NotErrorIs(t, err, errFallbackNotRouted)
	summary := incidentJob("startup/1", "default")
	err = m.deliverFallback(context.Background(), &hook, "Chat", summary)
	assert.NotErrorIs(t, err, errFallbackNotRouted)
}
