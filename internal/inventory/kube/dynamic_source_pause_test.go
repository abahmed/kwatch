package kube

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Under API pressure the safety timer's pass waits; a CRD change still
// starts one, and the first tick after the pressure runs it.
func TestDynamicSafetyPassWaitsUnderAPIPressure(t *testing.T) {
	high := true
	d := NewDynamicSource(DynamicConfig{Paused: func() bool { return high }})
	tick := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan bool, 1)
	go func() { done <- d.nextPass(ctx, tick) }()

	tick <- time.Time{}
	tick <- time.Time{} // taken only once the first tick was skipped
	d.mu.Lock()
	assert.False(t, d.retryDue, "the paused ticks run no pass")
	d.mu.Unlock()
	d.requestDiscovery()
	assert.True(t, <-done, "a CRD change still starts a pass")

	high = false
	go func() { done <- d.nextPass(ctx, tick) }()
	tick <- time.Time{}
	assert.True(t, <-done, "the pass runs once the pressure is over")

	go func() { done <- d.nextPass(ctx, tick) }()
	cancel()
	assert.False(t, <-done, "it stops with its context")
}
