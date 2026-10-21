package scenarios

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/abahmed/kwatch/internal/replay"
)

// The staging cluster scales down every night and wakes every morning;
// kwatch restarts with it and restores its state. A Deployment that has
// been unavailable for weeks gets a new pod each morning that never
// becomes ready. Its findings come back only after the restore grace (the
// new pod is created minutes after kwatch starts and is held while the
// cluster wakes), so the restart listing used to miss it: after the first
// day nobody was told it was still down.
func TestStagingWakeCycleNamesTheUnavailableDeploymentEveryMorning(
	t *testing.T,
) {
	store := newPersistingStore()
	s := &wakeCycle{}
	for day := range 4 {
		log := s.day(day)
		run := restartReplay(t, log, store, replay.Options{
			SyncAt: log.Start, Tail: 30 * time.Minute})
		named := false
		for _, m := range run.Messages {
			named = named ||
				strings.Contains(m.Title+m.Note, "cert-controller")
		}
		assert.True(t, named, "day %d: nobody was told the Deployment "+
			"is still down: %s", day, describeDecisions(run))
	}
}

// wakeCycle is the staging cluster over several days.
type wakeCycle struct {
	prev *cluster
	w    *workload
}

func (s *wakeCycle) day(day int) replay.Log {
	start := time.Date(2026, 10, 1+day, 5, 35, 0, 0, time.UTC)
	c := newCluster(start, "")
	if s.prev != nil {
		c.list(remaining(s.prev)...)
		s.w.c = c
	}
	var nodes []*corev1.Node
	for i := range 2 {
		n := c.node(fmt.Sprintf("d%dn%d", day, i+1), "zone-a")
		n.CreationTimestamp = metav1.NewTime(
			c.now.Add(-time.Duration(3-i) * time.Minute))
		nodes = append(nodes, n)
	}
	c.list(nodes[0], nodes[1])
	if s.prev == nil {
		s.w = stagingCertController(c)
		c.list(s.w.objects())
	}
	for i := 2; i < 4; i++ {
		c.after(time.Minute)
		n := c.node(fmt.Sprintf("d%dn%d", day, i+1), "zone-a")
		n.CreationTimestamp = metav1.NewTime(c.now)
		c.create(n)
		nodes = append(nodes, n)
	}
	c.after(2 * time.Minute)
	pod := s.w.pod(day, nodes[0].Name,
		createdAt(c.now), notReadySince(c.now))
	c.create(pod)
	c.after(23 * time.Second)
	count := int32(1)
	for range 12 * 12 {
		event := c.warningEvent(pod, "Pod", "Unhealthy",
			"Readiness probe failed: HTTP probe failed with "+
				"statuscode: 500", "kubelet", count)
		event.InvolvedObject.FieldPath = "spec.containers{app}"
		c.warn(event)
		count += 60
		c.after(5 * time.Minute)
	}
	c.remove(pod)
	for _, n := range nodes {
		c.remove(n)
	}
	s.prev = c
	return c.log()
}

func stagingCertController(c *cluster) *workload {
	w := c.deployment("external-secrets", "cert-controller",
		"registry.example.com/cert-controller:1.0", 1)
	w.setReady(0)
	probe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
			Path: "/readyz", Port: intstr.FromInt(8081)}},
		InitialDelaySeconds: 20, PeriodSeconds: 5, FailureThreshold: 3,
	}
	w.deployment.Spec.Template.Spec.Containers[0].ReadinessProbe = probe
	w.replicaSet.Spec.Template.Spec.Containers[0].ReadinessProbe = probe
	w.deployment.Status.Conditions = []appsv1.DeploymentCondition{{
		Type: appsv1.DeploymentAvailable, Status: corev1.ConditionFalse,
		Reason:             "MinimumReplicasUnavailable",
		LastTransitionTime: metav1.NewTime(c.now.Add(-26 * 24 * time.Hour)),
	}}
	return w
}
