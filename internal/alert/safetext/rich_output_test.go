package safetext

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/notification"
)

func TestShortOutputFitsUnderALimit(t *testing.T) {
	m := providertest.Announce()
	out := RichWithOutput(m, notification.MarkdownDialect(nil, "\n"), "\n", 2000)
	assert.Contains(t, out, "\n```\npanic: out of memory\n```")
}

// Output shorter than minOutputBudget is kept when the whole message
// fits, and dropped, leaving the note alone, when it does not.
func TestShortOutputKeptOnlyWhenItFits(t *testing.T) {
	m := providertest.Announce()
	d := notification.MarkdownDialect(nil, "\n")
	note := m.RenderWithin(d, 2000)
	full := RichWithOutput(m, d, "\n", 2000)
	assert.Less(t, len(m.Output[0]), minOutputBudget)
	assert.Contains(t, full, "panic: out of memory")

	tight := RichWithOutput(m, d, "\n", len(note)+5)
	assert.NotContains(t, tight, "panic: out of memory")
	assert.LessOrEqual(t, len(tight), len(note)+5)
}
