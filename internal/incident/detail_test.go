package incident

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestStoreDetailKeepsEachFactOnce(t *testing.T) {
	r := newRig(t, Config{})
	announced(t, r, podSig("web"))
	id := r.only().ID

	r.m.StoreDetail(id, Detail{Headline: "web is crash looping",
		Facts: []string{"panic: boom", "exit 2"}})
	r.m.StoreDetail(id, Detail{Facts: []string{"panic: boom", "retry"}})

	got := r.m.Incident(id)
	want := []string{"panic: boom", "exit 2", "retry"}
	if strings.Join(got.Facts, "|") != strings.Join(want, "|") {
		t.Fatalf("facts = %q, want %q", got.Facts, want)
	}
	if got.Headline != "web is crash looping" {
		t.Fatalf("headline = %q, want the stored one", got.Headline)
	}
}

func TestStoreDetailCutsALongFactOnARuneBoundary(t *testing.T) {
	r := newRig(t, Config{})
	announced(t, r, podSig("web"))
	id := r.only().ID
	long := strings.Repeat("a", maxFactLength-1) + "é tail"

	r.m.StoreDetail(id, Detail{Facts: []string{long}})

	got := r.m.Incident(id).Facts[0]
	if len(got) > maxFactLength || !utf8.ValidString(got) {
		t.Fatalf("fact cut badly: %d bytes, valid=%v",
			len(got), utf8.ValidString(got))
	}
}

func TestStoreDetailKeepsTheMostRecentFacts(t *testing.T) {
	r := newRig(t, Config{})
	announced(t, r, podSig("web"))
	id := r.only().ID
	facts := make([]string, 0, maxFacts+5)
	for i := range maxFacts + 5 {
		facts = append(facts, strings.Repeat("x", i+1))
	}

	r.m.StoreDetail(id, Detail{Facts: facts})

	got := r.m.Incident(id).Facts
	if len(got) != maxFacts || got[len(got)-1] != facts[len(facts)-1] {
		t.Fatalf("kept %d facts ending %q", len(got), got[len(got)-1])
	}
}

func TestRestoreReadsARecordWrittenBeforeFacts(t *testing.T) {
	old := newRig(t, Config{})
	announced(t, old, podSig("web"))
	body, err := json.Marshal(old.m.Export())
	if err != nil {
		t.Fatal(err)
	}
	var raw []map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	for _, rec := range raw {
		delete(rec, "Facts")
		delete(rec, "Headline")
		delete(rec, "DocsLink")
	}
	v2, _ := json.Marshal(raw)
	var records []Record
	if err := json.Unmarshal(v2, &records); err != nil {
		t.Fatalf("v2 record does not decode: %v", err)
	}

	fresh := newRig(t, Config{})
	fresh.m.Restore(records, at(10*time.Minute))

	got := fresh.only()
	if got.State != Open || len(got.Facts) != 0 || got.Headline != "" {
		t.Fatalf("restored incident = %+v", got)
	}
}

func TestExportKeepsTheDetailAcrossARestart(t *testing.T) {
	old := newRig(t, Config{})
	announced(t, old, podSig("web"))
	id := old.only().ID
	old.m.StoreDetail(id, Detail{Headline: "h", DocsLink: "https://x",
		Facts: []string{"log line"},
		Steps: []FixStep{{Text: "Roll back:", Command: "kubectl rollout"}}})

	fresh := newRig(t, Config{})
	fresh.m.Restore(old.m.Export(), at(10*time.Minute))

	got := fresh.only()
	if got.Headline != "h" || got.DocsLink != "https://x" ||
		len(got.Facts) != 1 || len(got.Steps) != 1 ||
		got.Steps[0].Command != "kubectl rollout" {
		t.Fatalf("detail lost on restore: %+v", got)
	}
}

func TestStoreDetailReplacesStepsAndBoundsThem(t *testing.T) {
	r := newRig(t, Config{})
	announced(t, r, podSig("web"))
	id := r.only().ID
	many := make([]FixStep, 0, maxSteps+3)
	for i := range maxSteps + 3 {
		many = append(many, FixStep{Text: strings.Repeat("s", i+1),
			Command: strings.Repeat("c", maxFactLength+10)})
	}

	r.m.StoreDetail(id, Detail{Steps: many})
	r.m.StoreDetail(id, Detail{Headline: "no steps keeps the old ones"})

	got := r.m.Incident(id).Steps
	if len(got) != maxSteps || len(got[0].Command) > maxFactLength {
		t.Fatalf("steps = %d, first command %d bytes", len(got),
			len(got[0].Command))
	}
	r.m.StoreDetail(id, Detail{Steps: []FixStep{{Text: "new"}}})
	if got := r.m.Incident(id).Steps; len(got) != 1 || got[0].Text != "new" {
		t.Fatalf("newer steps did not replace the old: %+v", got)
	}
}
