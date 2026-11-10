package telegram

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/notification"
)

// With no room left for the output, none of it is sent: a zero budget
// must not turn into "no limit".
func TestIncidentTextZeroBudgetDropsOutput(t *testing.T) {
	m := notification.Notice("note")
	m.Output = []string{strings.Repeat("o", 6000)}

	text := incidentText(m, 1)

	assert.Less(t, len(text), 200, text)
}
