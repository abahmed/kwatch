package replay_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/replay"
)

// An Encoder writes the same bytes as Write, one entry at a time, so a
// big export never holds the whole log.
func TestEncoderMatchesWrite(t *testing.T) {
	log := sampleLog()
	var want bytes.Buffer
	if err := replay.Write(&want, log); err != nil {
		t.Fatal(err)
	}
	var got bytes.Buffer
	enc, err := replay.NewEncoder(&got, log.Start)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range log.Entries {
		if err := enc.Entry(entry); err != nil {
			t.Fatal(err)
		}
	}
	if got.String() != want.String() {
		t.Errorf("encoder wrote\n%s\nwant\n%s", got.String(), want.String())
	}
}

func TestEncoderRefusesEntriesOutOfOrder(t *testing.T) {
	log := sampleLog()
	var buf bytes.Buffer
	enc, err := replay.NewEncoder(&buf, log.Start)
	if err != nil {
		t.Fatal(err)
	}
	if err := enc.Entry(log.Entries[1]); err != nil {
		t.Fatal(err)
	}
	if err := enc.Entry(log.Entries[0]); err == nil {
		t.Error("an entry before the previous one was accepted")
	}
	early := log.Entries[0]
	early.At = log.Start.Add(-time.Second)
	fresh, _ := replay.NewEncoder(&buf, log.Start)
	if err := fresh.Entry(early); err == nil {
		t.Error("an entry before the start was accepted")
	}
}
