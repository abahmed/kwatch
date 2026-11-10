package scenarios

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/replay"
)

// startupClearingLog records kwatch starting while eight Deployments in
// four namespaces already fail to pull from one registry. The pulls
// recover 40 seconds after the start.
func startupClearingLog() replay.Log {
	return startupPullLog(40*time.Second, true)
}

// startupPullLog is that start; when recover is false the pulls keep
// failing for the whole run, and waited is how long the run lasts.
func startupPullLog(waited time.Duration, recover bool) replay.Log {
	c := newCluster(scenarioStart, "")
	since := c.now.Add(-20 * time.Minute)
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	var apps []*workload
	for _, ns := range []string{"alpha", "beta", "gamma", "delta"} {
		for _, name := range []string{"web", "jobs"} {
			w := c.deployment(ns, name,
				clusterRegistry+"/"+ns+"/"+name+":1.0", 1)
			w.setReady(0)
			c.list(w.objects())
			pod := w.pod(0, "n1", createdAt(since), waiting(
				"ImagePullBackOff", "Back-off pulling image \""+
					clusterRegistry+"/"+ns+"/"+name+":1.0\""),
				notReadySince(since))
			svc := clusterService(c, ns, name, 8080)
			svc.Spec.Type = corev1.ServiceTypeLoadBalancer
			c.list(pod, svc, trafficUnreadySlice(c, name, pod))
			apps = append(apps, w)
		}
	}
	c.after(waited)
	if !recover {
		return c.log()
	}
	for _, w := range apps {
		w.setReady(1)
		c.update(w.objects())
		pod := w.pod(0, "n1", createdAt(since))
		c.update(pod, trafficSlice(c, w.deployment.Name, pod))
	}
	return c.log()
}

// Conditions that already exist when kwatch starts and clear within
// the first minute must not page: the settle time is measured from the
// start, and a page-tier decision goes through the startup hold.
func TestStartupConditionsThatClearDoNotPage(t *testing.T) {
	log := startupClearingLog()
	opts := replay.Options{SyncAt: log.Start.Add(2 * time.Second),
		Tail: 10 * time.Minute}
	result, err := replay.Run(context.Background(), log,
		newDependencies(), opts)
	if err != nil {
		t.Fatal(err)
	}
	for i, d := range result.Decisions {
		if d.Incident.Tier == incident.Page {
			t.Errorf("page-tier decision at %s: %s",
				result.Times[i].Sub(log.Start), d.Reason)
		}
	}
	if n := pagesRaised(result); n != 0 {
		t.Errorf("pages raised = %d, want 0", n)
	}
}

// A failure that is still there after the settle is a real outage and
// pages, but not before kwatch has been up for the full settle.
func TestStartupOutagePagesAfterTheSettle(t *testing.T) {
	log := startupPullLog(5*time.Minute, false)
	opts := replay.Options{SyncAt: log.Start.Add(2 * time.Second),
		Tail: 5 * time.Minute}
	result, err := replay.Run(context.Background(), log,
		newDependencies(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if pagesRaised(result) != 1 {
		t.Fatalf("pages = %d, want 1:%s", pagesRaised(result),
			describeDecisions(result))
	}
	for i, d := range result.Decisions {
		if d.Action == incident.Announce &&
			result.Times[i].Sub(log.Start) < incident.DefaultSettle {
			t.Errorf("announced at %s, before the settle",
				result.Times[i].Sub(log.Start))
		}
	}
}
