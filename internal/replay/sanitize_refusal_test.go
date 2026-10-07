package replay_test

import (
	"encoding/json"
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/replay"
)

// A registry's refusal can name a user or repository, so a shared log
// drops it with the message it was cut from, unless messages are kept.
func TestSanitizerDropsTheRegistryRefusalWithTheMessage(t *testing.T) {
	obs := inventory.Observation{Kind: inventory.Noted,
		Entity: inventory.EntityID{Kind: "pod", Namespace: "shop",
			Name: "api"},
		Note: inventory.Note{Source: "kubelet", Reason: "Failed",
			Message: "pull failed", Refusal: "denied: acme cannot pull"}}

	hidden := mustSanitizer(t, replay.SanitizeOptions{Salt: "s"})
	if got := hidden.Observation(obs).Note.Refusal; got != "" {
		t.Fatalf("refusal kept: %q", got)
	}
	kept := mustSanitizer(t, replay.SanitizeOptions{Salt: "s",
		KeepMessages: true})
	if got := kept.Observation(obs).Note.Refusal; got == "" {
		t.Fatal("refusal dropped though messages are kept")
	}
}

func TestRegistryRefusalSurvivesTheRecordedLogShape(t *testing.T) {
	in := inventory.Observation{Kind: inventory.Noted,
		Note: inventory.Note{Reason: "Failed",
			Refusal: "unauthorized: authentication required"}}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out inventory.Observation
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out.Note.Refusal != in.Note.Refusal {
		t.Fatalf("refusal = %q", out.Note.Refusal)
	}
}
