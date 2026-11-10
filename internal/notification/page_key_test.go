package notification

import (
	"encoding/json"
	"strings"
	"testing"
)

// The page flags travel with a message saved in the outbox, so a replay
// after a restart still goes to the page key; an unflagged message keeps
// its old JSON.
func TestPageFlagsSurviveTheOutbox(t *testing.T) {
	body, err := json.Marshal(Message{Key: "k", RaisesPage: true,
		PageKeyed: true})
	if err != nil {
		t.Fatal(err)
	}
	var back Message
	if err := json.Unmarshal(body, &back); err != nil {
		t.Fatal(err)
	}
	if !back.RaisesPage || !back.PageKeyed {
		t.Fatalf("flags lost: %s", body)
	}
	plain, _ := json.Marshal(Message{Key: "k"})
	if strings.Contains(string(plain), "Page") {
		t.Fatalf("unflagged message carries page flags: %s", plain)
	}
}

// The variant keeps its suffix under the cap and differs from the
// capped plain key.
func TestAlertKeyVariantKeepsSuffixUnderCap(t *testing.T) {
	m := Message{Key: strings.Repeat("k", 400)}
	plain := m.AlertKeyMax("dev", 255)
	got := m.AlertKeyVariant("dev", PageKeySuffix, 255)
	if len(got) > 255 || !strings.HasSuffix(got, PageKeySuffix) ||
		got == plain {
		t.Fatalf("variant %q (%d), plain %q", got, len(got), plain)
	}
	short := Message{Key: "p-42"}
	if v := short.AlertKeyVariant("dev", PageKeySuffix, 255); v !=
		short.AlertKey("dev")+PageKeySuffix {
		t.Fatalf("short variant = %q", v)
	}
}
