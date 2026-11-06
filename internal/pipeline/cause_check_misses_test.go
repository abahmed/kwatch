package pipeline

import (
	"encoding/json"
	"testing"

	"github.com/abahmed/kwatch/internal/incident"
)

// Recovery that followed a change to something other than the named
// cause keeps the incident as a likely miss, with what changed and the
// window to export it; a confirmed cause keeps none.
func TestCauseCheckKeepsALikelyMiss(t *testing.T) {
	c := newCheckerWithModel(t)
	changeOf(podEnt, 6, "spec.containers[app].image")
	c.observe([]incident.Decision{syntheticResolve("config-missing")})
	got := c.missesReport(checkAt(20))
	if len(got) != 1 {
		t.Fatalf("misses = %+v", got)
	}
	m := got[0]
	if m.ID != "i1" || m.Root != cfgMap.String() ||
		m.Other != podEnt.String() || m.Change != "image" {
		t.Errorf("miss = %+v", m)
	}
	if !m.ExportSince.Equal(checkAt(-10)) ||
		!m.ExportUntil.Equal(checkAt(25)) {
		t.Errorf("export window %s .. %s", m.ExportSince, m.ExportUntil)
	}

	confirmed := newCheckerWithModel(t)
	changeOf(cfgMap, 5, "data.KEY")
	confirmed.observe([]incident.Decision{syntheticResolve("config-missing")})
	if got := confirmed.missesReport(checkAt(20)); len(got) != 0 {
		t.Errorf("a confirmed cause kept misses %+v", got)
	}
}

// The saved value holds the counts and the misses; one saved by an
// older kwatch (the counts alone) still restores.
func TestCauseCheckSavedValueOfBothVersions(t *testing.T) {
	c := newCheckerWithModel(t)
	var saved json.RawMessage
	c.save = func(raw json.RawMessage) { saved = raw }
	changeOf(podEnt, 6, "spec.containers[app].image")
	c.observe([]incident.Decision{syntheticResolve("config-missing")})

	restored := newCheckerWithModel(t)
	restored.decode(saved)
	if _, other, _ := summaryOf(restored, "config-missing"); other != 1 {
		t.Errorf("restored other = %d, want 1", other)
	}
	if got := restored.missesReport(checkAt(20)); len(got) != 1 {
		t.Errorf("restored misses = %+v", got)
	}

	old, err := json.Marshal(c.counts)
	if err != nil {
		t.Fatal(err)
	}
	older := newCheckerWithModel(t)
	older.decode(old)
	if _, other, _ := summaryOf(older, "config-missing"); other != 1 {
		t.Errorf("older value: other = %d, want 1", other)
	}
	if got := older.misses.List(checkAt(20)); len(got) != 0 {
		t.Errorf("older value grew misses %+v", got)
	}
	var probe struct {
		Version int `json:"v"`
	}
	if json.Unmarshal(saved, &probe) != nil || probe.Version != 2 {
		t.Errorf("saved value %s has no version 2", saved)
	}
}
