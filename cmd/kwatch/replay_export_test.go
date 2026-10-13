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

func TestReplayExportNeedsAnOutputFile(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runReplayExport([]string{"--since", "2h"}, &out, &errOut,
		exportNow)
	if code != 2 || !strings.Contains(errOut.String(), "usage") {
		t.Errorf("code %d, stderr %q", code, errOut.String())
	}
}

func TestReplayExportRejectsABadWindow(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runReplayExport([]string{"--since", "soon", "--out",
		filepath.Join(t.TempDir(), "o.jsonl")}, &out, &errOut, exportNow)
	if code != 2 {
		t.Errorf("code %d, stderr %q", code, errOut.String())
	}
}

func TestReplayExportReportsAMissingStateFile(t *testing.T) {
	dir := t.TempDir()
	var out, errOut bytes.Buffer
	code := runReplayExport([]string{"--since", "2h", "--state",
		filepath.Join(dir, "none.db"), "--out",
		filepath.Join(dir, "o.jsonl")}, &out, &errOut, exportNow)
	if code != 1 || !strings.Contains(errOut.String(), "state file") {
		t.Errorf("code %d, stderr %q", code, errOut.String())
	}
}

func TestUnknownCommandListsReplayExport(t *testing.T) {
	var out, errOut bytes.Buffer
	runCommand([]string{"nope"}, &out, &errOut, func() int { return 0 })
	if !strings.Contains(errOut.String(), "replay-export") {
		t.Errorf("stderr %q", errOut.String())
	}
}
