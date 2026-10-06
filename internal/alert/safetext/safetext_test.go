package safetext

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFenceIsLongerThanAnyBacktickRun(t *testing.T) {
	out := NoteWithOutput("note", []string{"a ``` b", "`````"}, "\n\n", 0)
	assert.Contains(t, out, "\n``````\na ``` b\n`````\n``````")
	assert.True(t, strings.HasSuffix(out, "``````"))
}

func TestPlainOutputUsesThreeBackticks(t *testing.T) {
	out := NoteWithOutput("note", []string{"hello"}, "\n", 0)
	assert.Equal(t, "note\n```\nhello\n```", out)
}

func TestLimitCutsOutputBeforeTheClosingFence(t *testing.T) {
	long := []string{strings.Repeat("x`y", 400)}
	out := NoteWithOutput("note", long, "\n\n", 300)
	assert.LessOrEqual(t, len(out), 300)
	assert.True(t, strings.HasSuffix(out, "\n```"), out)
	assert.Contains(t, out, "…")
}

func TestNoRoomForOutputKeepsTheNote(t *testing.T) {
	out := NoteWithOutput(strings.Repeat("n", 100), []string{"o"}, "\n", 110)
	assert.NotContains(t, out, "```")
	assert.LessOrEqual(t, len(out), 110)
}

func TestNoOutputIsJustTheNote(t *testing.T) {
	assert.Equal(t, "note", NoteWithOutput("note", nil, "\n", 0))
}

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

func TestLinesAppliesTheFunction(t *testing.T) {
	got := Lines([]string{"a", "b"}, strings.ToUpper)
	assert.Equal(t, []string{"A", "B"}, got)
}

func TestPlainWithOutputKeepsTheNoteFirstAndBoundsTheRest(t *testing.T) {
	out := PlainWithOutput("note", []string{strings.Repeat("o", 500)},
		"\n\n", 100)
	assert.True(t, strings.HasPrefix(out, "note\n\n"))
	assert.LessOrEqual(t, len(out), 100)
	assert.Equal(t, "note\n\nline",
		PlainWithOutput("note", []string{"line"}, "\n\n", 100))
}
