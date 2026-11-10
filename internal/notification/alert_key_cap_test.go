package notification

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// A key within the limit is what every provider got before.
func TestAlertKeyMaxLeavesAShortKeyAlone(t *testing.T) {
	m := Message{Key: "inc-1"}

	assert.Equal(t, m.AlertKey("prod"), m.AlertKeyMax("prod", 100))
}

// A key over the limit is cut to it, stable between calls, and two keys
// that share a long prefix stay different.
func TestAlertKeyMaxCapsALongKeyStably(t *testing.T) {
	long := strings.Repeat("a", 150)
	a, b := Message{Key: long + "1"}, Message{Key: long + "2"}

	got := a.AlertKeyMax("prod", 100)

	assert.Len(t, got, 100)
	assert.True(t, strings.HasPrefix(got, "kwatch-prod-aaaa"), got)
	assert.Equal(t, got, a.AlertKeyMax("prod", 100), "stable")
	assert.NotEqual(t, got, b.AlertKeyMax("prod", 100), "unique")
}
