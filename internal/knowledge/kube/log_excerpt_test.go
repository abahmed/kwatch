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
	got := excerpt([]byte(b.String()))
	assert.Len(t, got, maxLogExcerpt)
	for _, line := range got {
		assert.Equal(t, "connection refused", line)
	}
}

func TestLogExcerptFallsBackToTailWithoutErrors(t *testing.T) {
	got := excerpt([]byte("one\ntwo\nthree\n"))
	assert.Equal(t, []string{"one", "two", "three"}, got)
}
