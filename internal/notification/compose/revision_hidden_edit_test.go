package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
)

// A revision whose only edit changed a hidden value still names that
// edit, without either value.
func TestRevisionDiffNamesAHiddenEnvEdit(t *testing.T) {
	p := badRollout()
	p.Cause = revisionCause(inventory.FieldChange{
		Path:   "containers[app].env.FOO",
		Before: inventory.HiddenValue, After: inventory.HiddenValue})

	text := notification.Text(Writer{}.Write(announce(p), at(6, 0)))

	assert.Contains(t, text, "Only change in revision 14: env FOO changed")
}
