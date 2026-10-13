package scenarios

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/pipeline"
	"github.com/abahmed/kwatch/internal/replay"
)

// TestFastCrashLoopNamesCrashLoopBackOff replays a pod that was already
// crash-looping when kwatch started. The container runs a second, exits
// 42 and backs off for longer each time. The first message is decided
// 75 seconds after the incident opened, and the instant that lands on
// may be the second the container is terminated or the minutes it waits
// in back-off. Both are the same crash loop, so the audit must name
// CrashLoopBackOff either way: routing and severity are keyed by reason.
func TestFastCrashLoopNamesCrashLoopBackOff(t *testing.T) {
	for _, third := range []int{60, 74, 100} {
		t.Run(itoa(third), func(t *testing.T) {
			lines := fastCrashLoopAudit(t, time.Duration(third))
			for _, line := range lines {
				if strings.Contains(line, "CrashLoopBackOff") {
					return
				}
			}
			t.Errorf("no audit entry names CrashLoopBackOff:\n%s",
				strings.Join(lines, "\n"))
		})
	}
}

// fastCrashLoopAudit returns one audit line per decision.
func fastCrashLoopAudit(t *testing.T, third time.Duration) []string {
	t.Helper()
	c := newCluster(scenarioStart, "")
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"))
	w := c.deployment("shop", "persistent",
		"registry.example.com/crash:1", 1)
	c.list(w.objects())
	c.list(w.pod(0, "n1", startedNow, crashLoop(42, "Error", "", 2)))
	var at time.Duration
	for i, start := range []time.Duration{3, 33, third} {
		c.after(start*time.Second - at)
		at = start*time.Second + time.Second
		r := int32(i + 3)
		c.update(w.pod(0, "n1", quickRun(r)))
		c.after(time.Second)
		c.update(w.pod(0, "n1", crashLoop(42, "Error", "", r)))
	}
	c.after(240 * time.Second)
	result, err := replay.Run(context.Background(), c.log(),
		newDependencies(), replay.Options{SyncAt: scenarioStart})
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for i, d := range result.Decisions {
		e := pipeline.AuditEntry(d, result.Messages[i], result.Times[i])
		lines = append(lines, string(e.Action)+" "+e.Reason+" ["+
			e.DecisionReason+"] at "+
			result.Times[i].Sub(scenarioStart).String())
	}
	return lines
}

// quickRun is the container that ran a second and exited 42.
func quickRun(restarts int32) podState {
	return func(c *cluster, pod *corev1.Pod) {
		notReady(c, pod)
		status := &pod.Status.ContainerStatuses[0]
		status.RestartCount = restarts
		status.Started = boolPtr(false)
		exit := &corev1.ContainerStateTerminated{
			ExitCode: 42, Reason: "Error",
			StartedAt:  metav1.NewTime(c.now.Add(-time.Second)),
			FinishedAt: metav1.NewTime(c.now),
		}
		status.State = corev1.ContainerState{Terminated: exit}
		status.LastTerminationState = corev1.ContainerState{
			Terminated: exit}
	}
}
