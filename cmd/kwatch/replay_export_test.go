package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var exportNow = func() time.Time {
	return time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
}

func allowAll(string) bool { return true }

func TestReplayExportNeedsAnOutputFile(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runReplayExport([]string{"--since", "2h"}, &out, &errOut,
		exportNow, allowAll)
	if code != 2 || !strings.Contains(errOut.String(), "usage") {
		t.Errorf("code %d, stderr %q", code, errOut.String())
	}
}

func TestReplayExportRejectsABadWindow(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runReplayExport([]string{"--since", "soon", "--out",
		filepath.Join(t.TempDir(), "o.jsonl")}, &out, &errOut, exportNow, allowAll)
	if code != 2 {
		t.Errorf("code %d, stderr %q", code, errOut.String())
	}
}

func TestReplayExportReportsAMissingStateFile(t *testing.T) {
	dir := t.TempDir()
	var out, errOut bytes.Buffer
	code := runReplayExport([]string{"--since", "2h", "--state",
		filepath.Join(dir, "none.db"), "--out",
		filepath.Join(dir, "o.jsonl")}, &out, &errOut, exportNow, allowAll)
	if code != 1 || !strings.Contains(errOut.String(), "state file") {
		t.Errorf("code %d, stderr %q", code, errOut.String())
	}
}

func TestUnknownCommandListsReplayExport(t *testing.T) {
	var out, errOut bytes.Buffer
	runCommand([]string{"nope"}, &out, &errOut, testNow,
		func() int { return 0 })
	if !strings.Contains(errOut.String(), "replay-export") {
		t.Errorf("stderr %q", errOut.String())
	}
}

// The edition comes from the state file the command was pointed at, so a
// Pro installation's replay-export works in the process the operator runs.
func TestReplayExportAsksForTheEditionOfTheStateFileItReads(t *testing.T) {
	var asked []string
	var out, errOut bytes.Buffer
	code := runReplayExport([]string{"--since", "2h", "--state",
		"/data/state.db", "--out", t.TempDir() + "/o.jsonl"}, &out,
		&errOut, exportNow, func(path string) bool {
			asked = append(asked, path)
			return false
		})
	if code != 1 || len(asked) != 1 || asked[0] != "/data/state.db" ||
		!strings.Contains(errOut.String(), "kwatch Pro feature") {
		t.Fatalf("code %d, asked %v, stderr %q", code, asked,
			errOut.String())
	}
}
