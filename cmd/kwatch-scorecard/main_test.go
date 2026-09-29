package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunExitCodeZeroSuccess(t *testing.T) {
	file := tempFileWithContent(t,
		`{"ts":"2026-01-01T00:00:00Z","problem":"k","action":"create"}`)
	defer os.Remove(file)
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	code := run([]string{"-input", file}, out, errOut)
	assert.Equal(t, 0, code)
}

func TestRunExitCodeOneThresholdViolated(t *testing.T) {
	file := tempFileWithContent(t,
		`{"ts":"2026-01-01T00:00:00Z","problem":"k","action":"create"}
{"ts":"2026-01-01T00:00:01Z","problem":"k","action":"resolved"}
{"ts":"2026-01-01T00:00:02Z","problem":"k","action":"create"}`)
	defer os.Remove(file)
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	code := run(
		[]string{"-input", file, "-max-recreated", "0"},
		out, errOut,
	)
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut.String(), "threshold exceeded")
}

func TestRunExitCodeTwoBadInput(t *testing.T) {
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	code := run([]string{"-input", "/nonexistent/file"}, out, errOut)
	assert.Equal(t, 2, code)
	assert.Contains(t, errOut.String(), "read audit log")
}

func TestRunExitCodeTwoInvalidFlags(t *testing.T) {
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	code := run([]string{"-invalid-flag"}, out, errOut)
	assert.Equal(t, 2, code)
}

func TestRunJSONOutput(t *testing.T) {
	file := tempFileWithContent(t,
		`{"ts":"2026-01-01T00:00:00Z","problem":"k","action":"create"}`)
	defer os.Remove(file)
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	code := run([]string{"-input", file, "-json"}, out, errOut)
	assert.Equal(t, 0, code)
	assert.Contains(t, out.String(), "notifications")
	assert.Contains(t, out.String(), "{")
}

func TestRunTextOutput(t *testing.T) {
	file := tempFileWithContent(t,
		`{"ts":"2026-01-01T00:00:00Z","problem":"k","action":"create"}`)
	defer os.Remove(file)
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	code := run([]string{"-input", file}, out, errOut)
	assert.Equal(t, 0, code)
	assert.NotContains(t, out.String(), "{")
}

func TestRunMultipleThresholds(t *testing.T) {
	file := tempFileWithContent(t,
		`{"ts":"2026-01-01T00:00:00Z","problem":"k","action":"create"}
{"ts":"2026-01-01T00:00:01Z","problem":"k","action":"resolved"}
{"ts":"2026-01-01T00:00:02Z","problem":"k","action":"create"}
{"ts":"2026-01-01T00:00:03Z","problem":"k","action":"update"}
{"ts":"2026-01-01T00:00:04Z","problem":"k","action":"update"}
{"ts":"2026-01-01T00:00:05Z","problem":"k","action":"update"}`)
	defer os.Remove(file)
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	code := run(
		[]string{
			"-input", file,
			"-max-per-problem", "4",
			"-max-recreated", "0",
		},
		out, errOut,
	)
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut.String(), "threshold exceeded")
}

func TestRunRecreatedExitCodeOne(t *testing.T) {
	file := tempFileWithContent(t,
		`{"ts":"2026-01-01T00:00:00Z","problem":"k","action":"create"}
{"ts":"2026-01-01T00:00:01Z","problem":"k","action":"resolved"}
{"ts":"2026-01-01T00:00:02Z","problem":"k","action":"create"}`)
	defer os.Remove(file)
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	code := run(
		[]string{"-input", file, "-max-recreated", "0"},
		out, errOut,
	)
	assert.Equal(t, 1, code)
}

func TestRunEmptyInput(t *testing.T) {
	file := tempFileWithContent(t, "")
	defer os.Remove(file)
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	code := run([]string{"-input", file}, out, errOut)
	assert.Equal(t, 0, code)
}

func tempFileWithContent(t *testing.T, content string) string {
	f, err := os.CreateTemp("", "scorecard-test-*.jsonl")
	require.NoError(t, err)
	defer f.Close()
	_, err = f.WriteString(content)
	require.NoError(t, err)
	return f.Name()
}
