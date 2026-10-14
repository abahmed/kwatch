package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTagFromGoMod(t *testing.T) {
	p := filepath.Join(t.TempDir(), "go.mod")
	require.NoError(t, os.WriteFile(p,
		[]byte("require (\n\tk8s.io/api v0.37.1\n)\n"), 0o600))
	tag, err := tagFromGoMod(p)
	require.NoError(t, err)
	assert.Equal(t, "v1.37.1", tag)

	require.NoError(t, os.WriteFile(p, []byte("module x\n"), 0o600))
	_, err = tagFromGoMod(p)
	assert.Error(t, err)
}

func TestWriteCheckReportsDrift(t *testing.T) {
	dir := t.TempDir()
	files := map[string][]byte{"a_generated.go": []byte("package x\n")}
	assert.Error(t, write(dir, files, true), "missing file is drift")
	require.NoError(t, write(dir, files, false))
	assert.NoError(t, write(dir, files, true))
	files["a_generated.go"] = []byte("package y\n")
	assert.Error(t, write(dir, files, true))
}

func TestTagFromGoModRejectsPreRelease(t *testing.T) {
	p := filepath.Join(t.TempDir(), "go.mod")
	for _, v := range []string{"v0.36.0-rc.1", "v0.0.0-20240101000000-abc",
		"v0.36.0+incompatible"} {
		require.NoError(t, os.WriteFile(p,
			[]byte("require (\n\tk8s.io/api "+v+"\n)\n"), 0o600))
		_, err := tagFromGoMod(p)
		require.Error(t, err, v)
		assert.Contains(t, err.Error(), "release", v)
	}
}

func TestDriftIsSortedAndHasItsOwnExitCode(t *testing.T) {
	files := map[string][]byte{"b.go": {1}, "a.go": {1}, "c.go": {1}}
	err := write(t.TempDir(), files, true)

	var drift *driftError
	require.ErrorAs(t, err, &drift)
	assert.Contains(t, err.Error(), "a.go, b.go, c.go")
	assert.Equal(t, 1, exitCode(err))
	assert.Equal(t, 2, exitCode(errors.New("git: network down")))
	assert.Equal(t, 0, exitCode(nil))
}

func TestCloneStopsWhenItsContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.Error(t, clone(ctx, "v1.0.0", t.TempDir()))
}
