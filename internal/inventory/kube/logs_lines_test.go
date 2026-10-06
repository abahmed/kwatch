package kube

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogLinesKeepsLinesAfterAVeryLongOne(t *testing.T) {
	long := strings.Repeat("x y ", 50<<10)
	body := []byte("first line\n" + long + "\nfatal: after the long line\n")
	lines := logLines(body)
	require.NotEmpty(t, lines)
	assert.Equal(t, "fatal: after the long line", lines[len(lines)-1])
}
