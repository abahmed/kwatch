package main

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "rewrite docs/kubernetes-coverage.md")

func TestCoverageDocIsUpToDate(t *testing.T) {
	root := filepath.Join("..", "..")
	want, err := render(root)
	require.NoError(t, err)
	path := filepath.Join(root, "docs", "kubernetes-coverage.md")
	if *update {
		require.NoError(t, os.WriteFile(path, want, 0o644))
	}
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got),
		"run: go run ./cmd/coveragedocs")
}

func TestRunCheckReportsStaleDocument(t *testing.T) {
	out := filepath.Join(t.TempDir(), "doc.md")
	root := filepath.Join("..", "..")
	require.NoError(t, os.WriteFile(out, []byte("old"), 0o644))
	assert.Error(t, run([]string{"-root", root, "-output", out, "-check"}))
	require.NoError(t, run([]string{"-root", root, "-output", out}))
	assert.NoError(t, run([]string{"-root", root, "-output", out, "-check"}))
}
