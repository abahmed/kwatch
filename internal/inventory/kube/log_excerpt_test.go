package kube

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLogExcerptPrefersErrorLinesAndBoundsCount(t *testing.T) {
	var b strings.Builder
	b.WriteString("starting\n\n")
	for i := 0; i < 8; i++ {
		b.WriteString("connection refused\n")
		b.WriteString("all fine\n")
	}
	got := ErrorExcerpt(logLines([]byte(b.String())))
	assert.Len(t, got, maxLogExcerpt)
	for _, line := range got {
		assert.Equal(t, "connection refused", line)
	}
}

func TestLogExcerptFallsBackToTailWithoutErrors(t *testing.T) {
	got := ErrorExcerpt(logLines([]byte("one\ntwo\nthree\n")))
	assert.Equal(t, []string{"one", "two", "three"}, got)
}

func TestLogLinesRedactsAndDropsBlankLines(t *testing.T) {
	got := logLines([]byte("  \nfirst\n\n password=hunter2 \n"))
	if len(got) != 2 || got[0] != "first" {
		t.Fatalf("lines = %q, want two non-blank lines", got)
	}
	if strings.Contains(got[1], "hunter2") {
		t.Fatalf("line %q must be redacted", got[1])
	}
}

func TestLastBytesCutsAtLineOrRuneBoundary(t *testing.T) {
	cases := []struct {
		name, body, want string
		n                int
	}{
		{"short body is whole", "a\nb\n", "a\nb\n", 10},
		{"cut drops the partial line", "aaaa\nbb\ncc\n", "cc\n", 5},
		{"cut on a line start keeps it", "aaa\nbb\n", "bb\n", 3},
		{"one long line starts on a rune", "ééééé", "éé", 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(lastBytes([]byte(tc.body), tc.n))
			assert.Equal(t, tc.want, got)
		})
	}
}
