package format

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDuration(t *testing.T) {
	assert.Equal(t, "5s", Duration(5e9))
	assert.Equal(t, "2m30s", Duration(150e9))
	assert.Equal(t, "2h15m", Duration(8100e9))
	assert.Equal(t, "1m", Duration(60e9))
	assert.Equal(t, "1h", Duration(3600e9))
	assert.Equal(t, "2h", Duration(7200e9))
}

// A negative span, from a clock step, is never printed as one.
func TestDurationNeverNegative(t *testing.T) {
	if got := Duration(-81 * time.Minute); got != "0s" {
		t.Errorf("Duration(-81m) = %q, want 0s", got)
	}
}
