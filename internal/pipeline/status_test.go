package pipeline

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestStatusReportsPDBsAndIgnoresAdvice(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC),
		ch: make(chan time.Time)}
	e := newTestEngine(t, clock, (&sinkLog{}).sink, nil)
	api := inventory.EntityID{Kind: kube.KindDeployment, Namespace: "shop",
		Name: "api"}
	e.published.set(api, []detection.Finding{{Entity: api,
		Reason: reasons.RiskNoReadinessProbe, Advisory: true}})
	pdb := inventory.EntityID{Kind: kube.KindPDB, Namespace: "shop",
		Name: "api-pdb"}
	_, err := e.deps.Model.Apply(observed(pdb, map[string]inventory.Value{
		kube.AttrDisruptionsAllowed: inventory.Number(0),
		kube.AttrExpectedPods:       inventory.Number(1)}))
	if err != nil {
		t.Fatal(err)
	}

	report := e.Status()

	if !strings.Contains(report.Upgrade.Line(),
		"PDB shop/api-pdb currently allows 0 disruptions") {
		t.Fatalf("upgrade = %q", report.Upgrade.Line())
	}
}

func TestStatusIsSafeWhileTheEngineRuns(t *testing.T) {
	clock := &lockedClock{now: time.Date(2026, 10, 7, 10, 0, 0, 0,
		time.UTC), ch: make(chan time.Time)}
	progress := make(chan struct{}, 256)
	e := newTestEngine(t, clock, (&sinkLog{}).sink, func(d *Dependencies) {
		d.Progress = func() { progress <- struct{}{} }
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := startRun(ctx, e)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 200 {
			_ = e.Status().Text()
		}
	}()
	for i := range 20 {
		id := inventory.CoreID(kube.KindNode, "", string(rune('a'+i)))
		e.Submit(ctx, observed(id, map[string]inventory.Value{
			kube.AttrReady: inventory.Bool(false)}))
	}
	wg.Wait()
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
