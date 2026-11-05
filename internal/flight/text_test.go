package flight

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/replay"
)

// textRecorder records with KeepText on the state file at path.
func textRecorder(
	t *testing.T, clock *clockAt, path string, keep bool,
) *Recorder {
	t.Helper()
	store := openStore(t, path, clock.Now)
	salt, err := LoadSalt(store)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := NewRecorderWith(NewRing(store), salt, clock.Now,
		Options{KeepText: keep})
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func crashedPod() inventory.Observation {
	return inventory.Observation{
		Kind: inventory.Observed, Source: "kubernetes",
		Entity: inventory.CoreID("pod", "shop", "api-1"),
		Attributes: map[string]inventory.Value{
			"last.error.line": inventory.Text("Unhandled exception. " +
				"app.ConfigError: setting X must be specified " +
				"(password=hunter2hunter2)"),
		},
		Note: inventory.Note{Reason: "BackOff",
			Message: "Back-off restarting failed container in api-1"},
	}
}

// exported records a crashed pod, exports the window and reads it back.
func exported(t *testing.T, keep bool) replay.Log {
	t.Helper()
	clock := newClock()
	dir := t.TempDir()
	path := filepath.Join(dir, "state.db")
	rec := textRecorder(t, clock, path, keep)
	submitAll(rec, crashedPod())
	rec.Flush()
	out := filepath.Join(dir, "incident.jsonl")
	if _, err := Export(ExportOptions{StatePath: path, Out: out}); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	log, err := replay.Read(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(log.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(log.Entries))
	}
	return log
}

// The evidence line reaches a replay of the export, scrubbed: what failed
// stays, the credential and the names go, and a name inside the text gets
// the pod's own pseudonym.
func TestExportKeepsScrubbedEvidenceText(t *testing.T) {
	o := exported(t, true).Entries[0].Observation
	line := o.Attributes["last.error.line"].AsText()
	if !strings.Contains(line, "Unhandled exception. app.ConfigError: "+
		"setting X must be specified") {
		t.Errorf("evidence line lost: %q", line)
	}
	if strings.Contains(line, "hunter2hunter2") {
		t.Errorf("credential kept: %q", line)
	}
	if !strings.HasSuffix(o.Note.Message, "container in "+o.Entity.Name) ||
		strings.Contains(o.Note.Message, "api-1") {
		t.Errorf("event message %q, pod %q", o.Note.Message, o.Entity.Name)
	}
}

// With KeepText off, free text is dropped as before.
func TestExportDropsTextWhenKeepTextIsOff(t *testing.T) {
	o := exported(t, false).Entries[0].Observation
	if _, ok := o.Attributes["last.error.line"]; ok || o.Note.Message != "" {
		t.Errorf("text kept: %+v %q", o.Attributes, o.Note.Message)
	}
}
