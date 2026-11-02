package dashboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// hugeIncident is an incident whose full detail is far over the
// response limit: a long timeline, many log lines, and evidence with
// many lines and facts.
func hugeIncident() incident.Incident {
	p := testIncident("big", "shop", 0)
	pad := strings.Repeat("x", 300)
	for i := range 90 {
		p.Timeline = append(p.Timeline, incident.Event{
			At:   testEpoch.Add(time.Duration(i) * time.Minute),
			Text: fmt.Sprintf("failing again: event %d %s", i, pad)})
	}
	for i := range 200 {
		p.Facts = append(p.Facts, fmt.Sprintf("log %d %s", i, pad))
	}
	for s := range 6 {
		sec := incident.Section{Check: fmt.Sprintf("check%d", s),
			Title: "Logs", Status: "ok", Matched: []int{0, 99}}
		for i := range 100 {
			sec.Lines = append(sec.Lines,
				fmt.Sprintf("sec %d line %d %s", s, i, pad))
			sec.Facts = append(sec.Facts, incident.SectionFact{
				Label: "Pod", Value: fmt.Sprintf("pod-%d", i)})
		}
		p.Evidence = append(p.Evidence, sec)
	}
	p.Correlation = &incident.Correlation{Confirmed: "it is the db"}
	return p
}

func TestIncidentDetailHandlerFitsLargeIncidentInLimit(t *testing.T) {
	p := hugeIncident()
	full, _ := json.Marshal(toDetailUnbounded(p))
	if len(full) <= maxResponseBytes {
		t.Fatalf("fixture is only %d bytes", len(full))
	}
	w := get(t, newTestServer(p), "/api/incidents/big")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	if w.Body.Len() > maxResponseBytes {
		t.Fatalf("body is %d bytes", w.Body.Len())
	}
	var d IncidentDetail
	if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if d.Truncated == nil || d.Truncated.Timeline == 0 ||
		d.Truncated.LogLines == 0 {
		t.Fatalf("truncated = %+v", d.Truncated)
	}
	kept := len(d.Timeline) + d.Truncated.Timeline
	if kept != len(p.Timeline) {
		t.Errorf("timeline kept %d + dropped %d, want %d",
			len(d.Timeline), d.Truncated.Timeline, len(p.Timeline))
	}
	last := d.Timeline[len(d.Timeline)-1].Text
	if !strings.Contains(last, "event 89 ") {
		t.Errorf("newest event not kept: %.40q", last)
	}
	logs := d.CapturedLogLines
	if len(logs) == 0 ||
		!strings.HasPrefix(logs[len(logs)-1], "log 199 ") {
		t.Errorf("last log line not kept: %d lines", len(logs))
	}
	ev := d.Evidence[0]
	if len(ev.Matched) == 0 || !strings.HasPrefix(
		ev.Lines[ev.Matched[0]], "sec 0 line 0 ") {
		t.Errorf("first matched line not kept: matched %v", ev.Matched)
	}
}

func TestIncidentDetailTruncationIsDeterministic(t *testing.T) {
	s := newTestServer(hugeIncident())
	a := get(t, s, "/api/incidents/big").Body.Bytes()
	b := get(t, s, "/api/incidents/big").Body.Bytes()
	if !bytes.Equal(a, b) {
		t.Error("two reads of the same incident differ")
	}
}

func TestIncidentDetailOmitsTruncatedWhenItFits(t *testing.T) {
	w := get(t, newTestServer(testIncident("a", "shop", 0)),
		"/api/incidents/a")
	if strings.Contains(w.Body.String(), `"truncated"`) {
		t.Errorf("small detail says truncated: %s", w.Body.String())
	}
}

func TestIncidentDetailCountsTimelineOverItsBound(t *testing.T) {
	p := testIncident("a", "shop", 0)
	for i := range maxTimeline + 7 {
		p.Timeline = append(p.Timeline, incident.Event{
			At:   testEpoch.Add(time.Duration(i) * time.Second),
			Text: "something new"})
	}
	d := toDetail(p)
	if d.Truncated == nil || d.Truncated.Timeline != 7 {
		t.Errorf("truncated = %+v, want timeline 7", d.Truncated)
	}
}

func TestTrimEvidenceLinesKeepsMatchedLinesFirst(t *testing.T) {
	d := IncidentDetail{Evidence: []EvidenceSection{{
		Lines:   []string{"a", "b", "c", "d", "e", "f"},
		Matched: []int{1, 4}}}}
	var tr DetailTruncation
	if !trimEvidenceLines(&d, &tr, 0) {
		t.Fatal("nothing trimmed")
	}
	s := d.Evidence[0]
	if got := strings.Join(s.Lines, ""); got != "bef" {
		t.Errorf("lines = %q, want %q", got, "bef")
	}
	if fmt.Sprint(s.Matched) != "[0 1]" || tr.EvidenceLines != 3 {
		t.Errorf("matched %v truncation %+v", s.Matched, tr)
	}
}

func TestTrimEvidenceLinesDropsMatchedOnlyWhenNoneElseLeft(
	t *testing.T,
) {
	d := IncidentDetail{Evidence: []EvidenceSection{{
		Lines:   []string{"a", "b", "c", "d"},
		Matched: []int{3, 0, 2}}}}
	var tr DetailTruncation
	if !trimEvidenceLines(&d, &tr, 0) {
		t.Fatal("nothing trimmed")
	}
	s := d.Evidence[0]
	if got := strings.Join(s.Lines, ""); got != "cd" {
		t.Errorf("lines = %q, want %q", got, "cd")
	}
	if fmt.Sprint(s.Matched) != "[0 1]" || tr.EvidenceLines != 2 {
		t.Errorf("matched %v truncation %+v", s.Matched, tr)
	}
}

func TestShortenTextsKeepsCommands(t *testing.T) {
	long := strings.Repeat("y", 50)
	d := IncidentDetail{
		FixSteps:         []FixStep{{Command: long}},
		CapturedLogLines: []string{long, "short"},
	}
	var tr DetailTruncation
	shortenTexts(&d, &tr, 10)
	if d.FixSteps[0].Command != long {
		t.Error("a command was shortened")
	}
	if len([]rune(d.CapturedLogLines[0])) != 10 || tr.Texts != 1 {
		t.Errorf("line %q texts %d", d.CapturedLogLines[0], tr.Texts)
	}
}

func TestWriteJSONRefusesOversizedBodyWithPath(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	w := httptest.NewRecorder()
	writeJSON(w, req, strings.Repeat("z", maxResponseBytes))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d", w.Code)
	}
	_, err := encodeBody(strings.Repeat("z", maxResponseBytes))
	if err == nil || !strings.Contains(err.Error(),
		fmt.Sprintf("%d byte limit", maxResponseBytes)) {
		t.Errorf("err = %v", err)
	}
}

// toDetailUnbounded is the detail before fitting, to prove a fixture
// is over the limit.
func toDetailUnbounded(p incident.Incident) IncidentDetail {
	d := toDetail(p)
	d.Timeline = toTimeline(p.Timeline)
	d.CapturedLogLines = redactAll(p.Facts)
	d.Evidence = toEvidence(p.Evidence)
	return d
}
