package replay_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/replay"
)

func sampleLog() replay.Log {
	start := time.Date(2026, 9, 29, 14, 0, 0, 0, time.UTC)
	node := inventory.CoreID("node", "", "n1")
	return replay.Log{Start: start, Entries: []replay.Entry{
		{At: start, Observation: inventory.Observation{
			Kind: inventory.Observed, Source: "test", At: start,
			Entity:     node,
			Attributes: map[string]inventory.Value{"ready": inventory.Bool(true)},
		}},
		{At: start.Add(time.Second), Observation: inventory.Observation{
			Kind: inventory.Gone, Source: "test", At: start.Add(time.Second),
			Entity: node,
		}},
	}}
}

func TestLogWriteReadRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	in := sampleLog()
	if err := replay.Write(&buf, in); err != nil {
		t.Fatal(err)
	}
	first, _, _ := strings.Cut(buf.String(), "\n")
	want := `{"format":"kwatch-observations","version":1,` +
		`"start":"2026-09-29T14:00:00Z"}`
	if first != want {
		t.Fatalf("header %s, want %s", first, want)
	}
	out, err := replay.Read(strings.NewReader(buf.String() + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !out.Start.Equal(in.Start) || len(out.Entries) != 2 {
		t.Fatalf("read %+v", out)
	}
	for i := range in.Entries {
		got, want := out.Entries[i], in.Entries[i]
		if !got.At.Equal(want.At) || got.Observation.Kind !=
			want.Observation.Kind || got.Observation.Entity !=
			want.Observation.Entity {
			t.Fatalf("entry %d = %+v, want %+v", i, got, want)
		}
	}
}

func TestLogReadRejectsInvalidLogs(t *testing.T) {
	header := `{"format":"kwatch-observations","version":1,` +
		`"start":"2026-09-29T14:00:00Z"}`
	entry := func(at string) string {
		return `{"at":"` + at + `","obs":{"kind":"gone",` +
			`"entity":{"kind":"node","name":"n1"}}}`
	}
	for name, input := range map[string]string{
		"empty":    "",
		"not json": "hello",
		"wrong format": `{"format":"other","version":1,` +
			`"start":"2026-01-01T00:00:00Z"}`,
		"new version": `{"format":"kwatch-observations","version":2,` +
			`"start":"2026-01-01T00:00:00Z"}`,
		"no start":     `{"format":"kwatch-observations","version":1}`,
		"before start": header + "\n" + entry("2026-09-29T13:00:00Z"),
		"out of order": header + "\n" + entry("2026-09-29T14:00:02Z") +
			"\n" + entry("2026-09-29T14:00:01Z"),
		"bad entry": header + "\n" + `{"at":"2026-09-29T14:00:00Z"}`,
		"long line": header + "\n" + strings.Repeat("x", 2<<20),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := replay.Read(strings.NewReader(input)); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

type failingWriter struct{ after int }

func (w *failingWriter) Write(p []byte) (int, error) {
	if w.after <= 0 {
		return 0, errWrite
	}
	w.after--
	return len(p), nil
}

func TestLogWriteReportsErrors(t *testing.T) {
	for _, after := range []int{0, 1} {
		if err := replay.Write(&failingWriter{after: after},
			sampleLog()); err == nil {
			t.Fatalf("write after %d lines: expected an error", after)
		}
	}
}
