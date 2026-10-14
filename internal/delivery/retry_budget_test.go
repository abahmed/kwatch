package delivery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// A long configured delay must not park the worker: the job stops retrying
// inline once the next wait would pass the budget, and returns its error so
// the caller requeues it.
func TestSendWithRetryStopsWhenWaitsPassBudget(t *testing.T) {
	attempts := 0
	started := time.Now()

	err := sendWithRetry(context.Background(), func() error {
		attempts++
		return errors.New("provider down")
	}, retryConfig{
		maxAttempts: 20, delay: 2 * maxInlineRetryTime,
		maxBackoff: 10 * time.Minute,
	}, "test")

	assert.Error(t, err)
	assert.Equal(t, 1, attempts, "no inline wait may pass the budget")
	assert.Less(t, time.Since(started), time.Second)
}
