package kube

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// numbered returns n lines "<prefix> <i>".
func numbered(prefix string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%s %d", prefix, i)
	}
	return out
}

// crashOutput is startup noise, a panic with its trace, then a long
// run of retries that ends in the latest line.
func crashOutput() []string {
	lines := numbered("starting", 5)
	lines = append(lines,
		"panic: runtime error: invalid memory address",
		"goroutine 1 [running]:",
		"main.main()",
		"\t/src/main.go:12 +0x1d")
	return append(lines, numbered("retrying", 40)...)
}

func TestCapturedLinesKeepWhatFits(t *testing.T) {
	lines := numbered("ok", 5)

	assert.Equal(t, lines, CapturedLines(lines, 25))
}

func TestCapturedLinesKeepTheStartOfTheTroubleAndTheEnd(t *testing.T) {
	lines := crashOutput()

	got := CapturedLines(lines, 25)

	require.Len(t, got, 25)
	assert.Equal(t, "panic: runtime error: invalid memory address", got[0],
		"the first error leads; the startup noise is gone")
	assert.Equal(t, "\t/src/main.go:12 +0x1d", got[3])
	assert.Equal(t, "retrying 3", got[7])
	assert.Equal(t, "… 20 lines skipped …", got[8])
	assert.Equal(t, "retrying 24", got[9])
	assert.Equal(t, "retrying 39", got[24])
}

func TestCapturedLinesWithoutAnErrorKeepTheEnd(t *testing.T) {
	lines := numbered("ok", 40)

	got := CapturedLines(lines, 25)

	assert.Equal(t, lines[15:], got)
}

func TestCapturedLinesNeedNoMarkerWhenTheEndReachesTheError(t *testing.T) {
	lines := append(numbered("starting", 10), "error: bad config")
	lines = append(lines, numbered("retrying", 10)...)

	got := CapturedLines(lines, 15)

	assert.Equal(t, lines[6:], got)
	assert.Contains(t, got, "error: bad config")
}

func TestCapturedLinesKeepTheirStartAsMoreArrive(t *testing.T) {
	held := CapturedLines(crashOutput(), 25)

	held = CapturedLines(append(held, numbered("later", 5)...), 25)

	require.Len(t, held, 25)
	assert.Equal(t, "panic: runtime error: invalid memory address",
		held[0])
	assert.Equal(t, "… 25 lines skipped …", held[8],
		"the count grows by the lines that moved past")
	assert.Equal(t, "later 4", held[24])
}

func TestCapturedLinesStayBoundedForTinyLimits(t *testing.T) {
	for limit := 1; limit <= 4; limit++ {
		held := CapturedLines(crashOutput(), limit)
		held = CapturedLines(append(held, "more"), limit)

		assert.LessOrEqual(t, len(held), limit, "limit %d", limit)
	}
}

func TestCapturedLinesReadOnlyTheirOwnMarker(t *testing.T) {
	for _, line := range []string{
		"… many lines skipped …", "… -3 lines skipped …",
		"… 3 lines skipped", "3 lines skipped …",
	} {
		_, ok := parseSkipped(line)

		assert.False(t, ok, line)
	}
}
