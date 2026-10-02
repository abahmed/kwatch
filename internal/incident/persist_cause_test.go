package incident

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// legacyRecordJSON is an incident record from before causes carried
// their mode and proof codes: both live only in explain's words.
const legacyRecordJSON = `{
	"ID": "inc-1-7f3a", "Root": {"Kind": "secret", "Namespace": "shop",
		"Name": "db"},
	"Cause": {"Rule": "config-missing-or-changed",
		"Root": {"Kind": "secret", "Namespace": "shop", "Name": "db"},
		"Summary": "secret db (Missing) explains 2 failures",
		"Points": [
			{"Text": "2 of 3 dependents fail", "Weight": 0.1,
				"Supports": true},
			{"Text": "the errors of 2 of 2 failures name it",
				"Weight": 0.15, "Supports": true},
			{"Text": "it changed shortly before: data.a, data.b",
				"Weight": 0.2, "Supports": true},
			{"Text": "it began before the failures", "Weight": 0.05,
				"Supports": true},
			{"Text": "only one workload fails behind it", "Weight": 0.1,
				"Supports": false}
		]},
	"State": 2
}`

func TestRestoreDerivesLegacyCauseFields(t *testing.T) {
	var record Record
	if err := json.Unmarshal([]byte(legacyRecordJSON), &record); err != nil {
		t.Fatal(err)
	}
	m := newRig(t, Config{}).m
	m.Restore([]Record{record}, at(0))
	cause := m.Export()[0].Cause
	if cause.Mode != "Missing" {
		t.Errorf("mode = %q, want Missing", cause.Mode)
	}
	want := []rootcause.Proof{
		{Code: rootcause.ProofDependentsFail, Count: 2, Total: 3},
		{Code: rootcause.ProofErrorsName, Count: 2, Total: 2},
		{Code: rootcause.ProofChanged, Fields: []string{"data.a", "data.b"}},
		{Code: rootcause.ProofBeganBefore},
		// Words no earlier rewrite knew stay text.
		{},
	}
	if len(cause.Proof) != len(want) {
		t.Fatalf("proof = %+v", cause.Proof)
	}
	for i, p := range cause.Proof {
		got := rootcause.Proof{Code: p.Code, Count: p.Count,
			Total: p.Total, Fields: p.Fields}
		if !reflect.DeepEqual(got, want[i]) {
			t.Errorf("proof %d = %+v, want %+v", i, got, want[i])
		}
		if p.Text == "" {
			t.Errorf("proof %d lost its text", i)
		}
	}
	if record.Cause.Proof[0].Code != "" {
		t.Error("restore must not change the record it was given")
	}
}

func TestRestoreKeepsStructuredCause(t *testing.T) {
	cause := &rootcause.CauseRecord{Rule: "webhook-rejects",
		Mode:    detection.Mode("Webhook.Timeout"),
		Summary: "validatingwebhookconfiguration p (Other) explains 1",
		Proof: []rootcause.Proof{{Code: rootcause.ProofNothingUpstream,
			Text: "1 of 2 dependents fail", Supports: true}}}
	got := restoredCause(cause)
	if !reflect.DeepEqual(got, cause) || got == cause {
		t.Fatalf("restoredCause = %+v, want a copy of %+v", got, cause)
	}
}

// TestCauseRecordKeepsPersistedNames pins the JSON names of the cause
// record: renaming a Go field must not rename what is on disk.
func TestCauseRecordKeepsPersistedNames(t *testing.T) {
	data, err := json.Marshal(rootcause.CauseRecord{Rule: "self",
		Proof: []rootcause.Proof{{Text: "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{`"Rule"`, `"Points"`, `"Summary"`} {
		if !strings.Contains(string(data), name) {
			t.Errorf("%s missing from %s", name, data)
		}
	}
}
