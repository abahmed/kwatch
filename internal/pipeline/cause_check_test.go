package pipeline

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

var (
	checkStart = time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	cfgMap     = inventory.EntityID{Kind: kube.KindConfigMap,
		Namespace: "shop", Name: "settings"}
	podEnt = inventory.EntityID{Kind: kube.KindPod,
		Namespace: "shop", Name: "api-1"}
	otherCfg = inventory.EntityID{Kind: kube.KindConfigMap,
		Namespace: "shop", Name: "unrelated"}
)

func checkAt(min int) time.Time {
	return checkStart.Add(time.Duration(min) * time.Minute)
}

func changeOf(id inventory.EntityID, min int, path string) {
	checkModel.change(id, checkAt(min), path)
}

// checkModelT wraps a model for synthetic incidents.
type checkModelT struct{ m *inventory.Model }

var checkModel checkModelT

func (c checkModelT) change(id inventory.EntityID, at time.Time, path string) {
	_, _ = c.m.Apply(inventory.Observation{
		Kind: inventory.Changed, Source: "test", At: at, Entity: id,
		Change: inventory.Change{Entity: id, At: at,
			Fields: []inventory.FieldChange{{Path: path, After: "x"}}},
	})
}

func syntheticResolve(rule string) incident.Decision {
	finding := detection.Finding{Entity: podEnt}
	return incident.Decision{Action: incident.Resolve,
		Incident: incident.Incident{
			ID: "i1", Root: cfgMap, Opened: checkAt(0),
			RecoveringSince: checkAt(10), Resolved: checkAt(15),
			Cause: &rootcause.CauseRecord{Rule: rule, Root: cfgMap},
			Members: map[detection.Key]detection.Finding{
				{Entity: podEnt, Reason: "CrashLoopBackOff"}: finding},
		}}
}

func newCheckerWithModel(t *testing.T) *causeChecker {
	t.Helper()
	model := inventory.NewModel(inventory.Options{})
	checkModel = checkModelT{model}
	return newCauseChecker(model)
}

func summaryOf(c *causeChecker, rule string) (int, int, int) {
	for _, r := range c.counts.Summary(checkAt(20)) {
		if r.Rule == rule {
			return r.Confirmed, r.Other, r.None
		}
	}
	return 0, 0, 0
}

func TestCauseCheckConfirmedByAChangeToTheCause(t *testing.T) {
	c := newCheckerWithModel(t)
	changeOf(cfgMap, 5, "data.KEY")
	c.observe([]incident.Decision{syntheticResolve("config-missing")})
	if a, b, n := summaryOf(c, "config-missing"); a != 1 || b != 0 || n != 0 {
		t.Errorf("confirmed/other/none = %d/%d/%d, want 1/0/0", a, b, n)
	}
}

func TestCauseCheckSomethingElseChanged(t *testing.T) {
	c := newCheckerWithModel(t)
	changeOf(podEnt, 6, "spec.containers[app].image")
	c.observe([]incident.Decision{syntheticResolve("config-missing")})
	if a, b, n := summaryOf(c, "config-missing"); a != 0 || b != 1 || n != 0 {
		t.Errorf("confirmed/other/none = %d/%d/%d, want 0/1/0", a, b, n)
	}
}

func TestCauseCheckNothingChanged(t *testing.T) {
	c := newCheckerWithModel(t)
	changeOf(otherCfg, 5, "data.KEY") // unrelated: never counted
	c.observe([]incident.Decision{syntheticResolve("config-missing")})
	if a, b, n := summaryOf(c, "config-missing"); a != 0 || b != 0 || n != 1 {
		t.Errorf("confirmed/other/none = %d/%d/%d, want 0/0/1", a, b, n)
	}
}

func TestCauseCheckIgnoresAChangeAfterRecovery(t *testing.T) {
	c := newCheckerWithModel(t)
	changeOf(cfgMap, 12, "data.KEY")
	c.observe([]incident.Decision{syntheticResolve("config-missing")})
	if a, _, n := summaryOf(c, "config-missing"); a != 0 || n != 1 {
		t.Errorf("a change after recovery began confirmed the cause")
	}
}

func TestCauseCheckCountsTheCauseRecovering(t *testing.T) {
	c := newCheckerWithModel(t)
	checkModel.m.MarkHealth(cfgMap, true, "Missing", checkAt(1))
	checkModel.m.MarkHealth(cfgMap, false, "", checkAt(8))
	c.observe([]incident.Decision{syntheticResolve("config-missing")})
	if a, _, _ := summaryOf(c, "config-missing"); a != 1 {
		t.Errorf("cause recovery before the members was not confirmed")
	}
}

func TestCauseCheckSkipsWhatItCannotJudge(t *testing.T) {
	c := newCheckerWithModel(t)
	superseded := syntheticResolve("rollout")
	superseded.Reason = incident.ReasonSuperseded
	noCause := syntheticResolve("rollout")
	noCause.Incident.Cause = nil
	announce := syntheticResolve("rollout")
	announce.Action = incident.Announce
	c.observe([]incident.Decision{superseded, noCause, announce})
	if got := c.counts.Summary(checkAt(20)); len(got) != 0 {
		t.Errorf("counted %+v, want nothing", got)
	}
}

func TestCauseCheckNeverReachesTheDecision(t *testing.T) {
	c := newCheckerWithModel(t)
	d := syntheticResolve("config-missing")
	before := d.Incident.Cause.Score
	changeOf(cfgMap, 5, "data.KEY")
	c.observe([]incident.Decision{d})
	if d.Incident.Cause.Score != before {
		t.Error("the self-check changed a cause score")
	}
}

func TestCauseCheckSurvivesARestart(t *testing.T) {
	store := openTempStore(t)
	disk := NewIncidentStore(store)
	first := newCheckerWithModel(t)
	var saved json.RawMessage
	first.save = func(raw json.RawMessage) { saved = raw }
	changeOf(cfgMap, 5, "data.KEY")
	first.observe([]incident.Decision{syntheticResolve("config-missing")})
	if saved == nil {
		t.Fatal("nothing was handed to the writer")
	}
	if err := disk.(causeStore).SaveCauseChecks(saved); err != nil {
		t.Fatal(err)
	}
	second := newCheckerWithModel(t)
	second.restore(disk)
	if a, _, _ := summaryOf(second, "config-missing"); a != 1 {
		t.Errorf("restored confirmed = %d, want 1", a)
	}
}

func TestStatusReportsTheCauseSelfCheck(t *testing.T) {
	c := newCheckerWithModel(t)
	changeOf(cfgMap, 5, "data.KEY")
	c.observe([]incident.Decision{syntheticResolve("config-missing")})
	got := c.report(checkAt(20))
	if len(got) != 1 || got[0].Total != 1 || got[0].Text == "" {
		t.Errorf("report = %+v", got)
	}
}
