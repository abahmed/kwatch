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
		`{"ts":"2026-01-01T00:00:00Z","incident":"k","action":"create"}`)
	defer os.Remove(file)
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	code := run([]string{"-input", file}, out, errOut)
	assert.Equal(t, 0, code)
}

func TestRunExitCodeOneThresholdViolated(t *testing.T) {
	file := tempFileWithContent(t,
		`{"ts":"2026-01-01T00:00:00Z","incident":"k","action":"create"}
{"ts":"2026-01-01T00:00:01Z","incident":"k","action":"resolved"}
{"ts":"2026-01-01T00:00:02Z","incident":"k","action":"create"}`)
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
		`{"ts":"2026-01-01T00:00:00Z","incident":"k","action":"create"}`)
	defer os.Remove(file)
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	code := run([]string{"-input", file, "-json"}, out, errOut)
	assert.Equal(t, 0, code)
	assert.Contains(t, out.String(), "notifications")
	assert.Contains(t, out.String(), "{")
}

func TestRunTextOutput(t *testing.T) {
	file := tempFileWithContent(t,
		`{"ts":"2026-01-01T00:00:00Z","incident":"k","action":"create"}`)
	defer os.Remove(file)
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	code := run([]string{"-input", file}, out, errOut)
	assert.Equal(t, 0, code)
	assert.NotContains(t, out.String(), "{")
}

func TestRunMultipleThresholds(t *testing.T) {
	file := tempFileWithContent(t,
		`{"ts":"2026-01-01T00:00:00Z","incident":"k","action":"create"}
{"ts":"2026-01-01T00:00:01Z","incident":"k","action":"resolved"}
{"ts":"2026-01-01T00:00:02Z","incident":"k","action":"create"}
{"ts":"2026-01-01T00:00:03Z","incident":"k","action":"update"}
{"ts":"2026-01-01T00:00:04Z","incident":"k","action":"update"}
{"ts":"2026-01-01T00:00:05Z","incident":"k","action":"update"}`)
	defer os.Remove(file)
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	code := run(
		[]string{
			"-input", file,
			"-max-per-incident", "4",
			"-max-recreated", "0",
		},
		out, errOut,
	)
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut.String(), "threshold exceeded")
}

func TestRunRecreatedExitCodeOne(t *testing.T) {
	file := tempFileWithContent(t,
		`{"ts":"2026-01-01T00:00:00Z","incident":"k","action":"create"}
{"ts":"2026-01-01T00:00:01Z","incident":"k","action":"resolved"}
{"ts":"2026-01-01T00:00:02Z","incident":"k","action":"create"}`)
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

// Limits with a production goal apply without flags: an incident that
// resolves and comes back is a re-created incident.
func TestRunAppliesProductionGoalsByDefault(t *testing.T) {
	file := tempFileWithContent(t,
		`{"ts":"2026-01-01T00:00:00Z","incident":"k","action":"create"}
{"ts":"2026-01-01T00:00:01Z","incident":"k","action":"resolved"}
{"ts":"2026-01-01T00:00:02Z","incident":"k","action":"create"}`)
	defer os.Remove(file)
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	code := run([]string{"-input", file}, out, errOut)
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut.String(), "re-created incidents %")
	code = run([]string{"-input", file, "-max-recreated-pct", "-1"},
		&bytes.Buffer{}, &bytes.Buffer{})
	assert.Equal(t, 0, code)
}

func TestRunLimitsMessagesPerIncident(t *testing.T) {
	file := tempFileWithContent(t,
		`{"ts":"2026-01-01T00:00:00Z","incident":"k","action":"create"}
{"ts":"2026-01-01T00:00:01Z","incident":"k","action":"update"}
{"ts":"2026-01-01T00:00:02Z","incident":"k","action":"update"}
{"ts":"2026-01-01T00:00:03Z","incident":"k","action":"update"}
{"ts":"2026-01-01T00:00:04Z","incident":"k","action":"update"}
{"ts":"2026-01-01T00:00:05Z","incident":"k","action":"update"}`)
	defer os.Remove(file)
	errOut := &bytes.Buffer{}
	code := run([]string{"-input", file}, &bytes.Buffer{}, errOut)
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut.String(), "most messages for one incident")
}

func TestRunLimitsMessagesPerIncidentP95(t *testing.T) {
	file := tempFileWithContent(t,
		`{"ts":"2026-01-01T00:00:00Z","incident":"k","action":"create"}
{"ts":"2026-01-01T00:00:01Z","incident":"k","action":"update"}
{"ts":"2026-01-01T00:00:02Z","incident":"k","action":"update"}
{"ts":"2026-01-01T00:00:03Z","incident":"k","action":"update"}`)
	defer os.Remove(file)
	errOut := &bytes.Buffer{}
	code := run([]string{"-input", file}, &bytes.Buffer{}, errOut)
	assert.Equal(t, 1, code)
	assert.Contains(t, errOut.String(), "p95 messages per incident")
	assert.NotContains(t, errOut.String(),
		"most messages for one incident", "4 is within the most of 5")
	code = run([]string{"-input", file,
		"-max-messages-per-incident-p95", "4"},
		&bytes.Buffer{}, &bytes.Buffer{})
	assert.Equal(t, 0, code)
}
