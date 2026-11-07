package safetext

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWeComUserMentionIsBroken(t *testing.T) {
	got := WeCom("hi <@zhangsan> and @all")
	assert.NotContains(t, got, "<@")
	assert.NotContains(t, got, "@all")
}

func TestZulipGroupMentionsAreBroken(t *testing.T) {
	got := Zulip("@*admins* @_*admins* @**all** @_**bob** @everyone")
	for _, live := range []string{"@*", "@_*", "@everyone"} {
		assert.NotContains(t, got, live)
	}
}

func TestPlainWithOutputKeepsTheNoteFirstAndBoundsTheRest(t *testing.T) {
	out := PlainWithOutput("note", []string{strings.Repeat("o", 500)},
		"\n\n", 100)
	assert.True(t, strings.HasPrefix(out, "note\n\n"))
	assert.LessOrEqual(t, len(out), 100)
	assert.Equal(t, "note\n\nline",
		PlainWithOutput("note", []string{"line"}, "\n\n", 100))
}
