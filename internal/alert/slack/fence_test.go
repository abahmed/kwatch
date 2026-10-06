package slack

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCodeSectionOutputCannotCloseTheBlock(t *testing.T) {
	section := codeSection("before ``` <!channel> after")
	text := section.Text.Text
	assert.True(t, strings.HasPrefix(text, "```"))
	assert.True(t, strings.HasSuffix(text, "```"))
	inner := strings.TrimSuffix(strings.TrimPrefix(text, "```"), "```")
	assert.NotContains(t, inner, "```")
	assert.NotContains(t, inner, "<!channel>")
}
