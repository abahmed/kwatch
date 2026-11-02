package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/pipeline/announce"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// communityLifecycleStart is communityStart with the incident lifecycle
// Community runs: no flapping, paging, reminders or hand-over.
func communityLifecycleStart(
	t *testing.T, start time.Time,
) (*harness, *corev1.Pod) {
	t.Helper()
	h := newHarnessWith(t, start, func(d *Dependencies) {
		d.Entitlements = feature.CommunityEntitlements()
		d.Incidents = incident.NewManager(incident.Config{
			IDNonce: harnessNonce, NoFlapping: true, NoPaging: true,
			NoReminders: true, NoHandover: true,
		}, explain.NewSolver())
	})
	require.NoError(t, h.engine.restore())
	h.add(kube.NodeSchema{}, node("n1", time.Time{}))
	d, rs := deployment("queue")
	d.Status.ReadyReplicas = *d.Spec.Replicas
	h.add(kube.DeploymentSchema(), d)
	h.add(kube.ReplicaSetSchema(), rs)
	crashing := crashingPod("queue-a", rs.Name, "n1", start)
	h.add(kube.PodSchema{}, crashing)
	h.engine.reconcileDowntime()
	return h, crashing
}

// crashAgain makes the healed pod crash again at the harness's time.
func crashAgain(h *harness, crashing *corev1.Pod) {
	healthy := pod("queue-a", crashing.OwnerReferences[0].Name, "n1",
		true, h.now)
	healthy.ResourceVersion = "next"
	again := crashing.DeepCopy()
	again.ResourceVersion = "again"
	h.engine.Submit(context.Background(), kube.NewTranslator(
		kube.PodSchema{}).Updated(healthy, again, h.now)...)
}

// A failure that exists when Community starts and is recovering when the
// startup window closes stays quiet. When it fails again inside its
// recovery hold, the manager makes no decision (it is the same incident),
// so the relapse is found from its live state: it is announced once, and
// its later recovery is sent too.
func TestCommunityAnnouncesARelapseAfterTheStartupWindow(t *testing.T) {
	start := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	h, crashing := communityLifecycleStart(t, start)

	h.run(start.Add(announce.StartupWindow-30*time.Second),
		10*time.Second)
	heal(h, crashing)
	h.run(start.Add(announce.StartupWindow+10*time.Second),
		10*time.Second)
	incidents := h.engine.deps.Incidents.Incidents()
	require.Len(t, incidents, 1)
	id := incidents[0].ID
	require.Equal(t, incident.Recovering, incidents[0].State)
	require.True(t, h.engine.announcer.collect.Startup.Baselined(id))
	require.Empty(t, h.messages, "recovering at window end: quiet")

	crashAgain(h, crashing)
	h.run(h.now.Add(2*time.Minute), 10*time.Second)
	sent := announcements(h)
	require.Len(t, sent, 1, "the relapse is announced: %v",
		messageKeys(h))
	assert.Equal(t, id, sent[0].Incident.ID)
	assert.False(t, h.engine.announcer.collect.Startup.Baselined(id))

	heal(h, crashing)
	h.run(h.now.Add(30*time.Minute), 10*time.Second)
	assert.Len(t, announcements(h), 1, "announced once")
	last := h.messages[len(h.messages)-1]
	assert.Equal(t, notification.StatusResolved, last.Status,
		"its later recovery is sent")
}
