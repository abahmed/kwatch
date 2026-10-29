package structured

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestIncidentCarriesEvidenceOnlyWhenGathered(t *testing.T) {
	plain, _ := json.Marshal(NewIncident("dev", providertest.Announce()))
	if strings.Contains(string(plain), `"evidence"`) {
		t.Fatalf("a message without evidence grew the field: %s", plain)
	}
	body, _ := json.Marshal(NewIncident("dev", providertest.ProEvidence()))
	var got struct {
		Evidence struct {
			Confirmed bool `json:"confirmed"`
			Checks    int  `json:"checks"`
			Quote     struct {
				Container string   `json:"container"`
				Run       string   `json:"run"`
				Lines     []string `json:"lines"`
			} `json:"quote"`
			Fix []struct {
				Command  string `json:"command"`
				Mutating bool   `json:"mutating"`
			} `json:"fix"`
		} `json:"evidence"`
		Steps []any `json:"steps"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	ev := got.Evidence
	if !ev.Confirmed || ev.Checks != 5 || ev.Quote.Run != "previous" ||
		len(ev.Fix) != 1 || !ev.Fix[0].Mutating || len(got.Steps) == 0 {
		t.Fatalf("payload = %s", body)
	}
}
