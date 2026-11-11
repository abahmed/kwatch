package kube

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// A failed discovery pass keeps what earlier passes left out: a kind
// over the budget stays noted, so it is not trusted as watched.
func TestDynamicSourceFailedDiscoveryKeepsSkippedKinds(t *testing.T) {
	disco := newLockedDiscovery(append(anchorLists(), widgetList())...)
	r := runDynamicSource(t, DynamicConfig{
		Client: dynamicClient(widgetCRD()), Discovery: disco, Budget: 2,
	})
	assert.Equal(t, 1, r.pass(t).Skipped)

	disco.set()
	disco.fail(errors.New("discovery unavailable"))
	r.src.requestDiscovery()
	assert.Equal(t, 1, r.pass(t).Skipped,
		"a failed pass forgot the kind it never watched")
}
