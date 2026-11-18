package kube

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

func callAttrs(targets ...inventory.EntityID) map[string]inventory.Value {
	keys := ""
	for i, target := range targets {
		if i > 0 {
			keys += ","
		}
		keys += target.String()
	}
	return map[string]inventory.Value{AttrTemplateCalls: inventory.Text(keys)}
}

func TestLinkJournalRecordsWorkloadThatStopsCalling(t *testing.T) {
	r := newJournalRig(t)
	r.observe(journalAPI, callAttrs(journalSvc))
	r.apply(inventory.Observation{Kind: inventory.Changed,
		Entity: journalAPI, Change: inventory.Change{At: r.now,
			Actor: "alice", Fields: []inventory.FieldChange{
				{Path: "spec.template.spec.containers"}}}})
	r.observe(journalAPI, nil)
	assert.Empty(t, r.due(30*time.Second), "waits out the flap window")
	got := r.due(30 * time.Second)
	require.Len(t, got, 1)
	assert.Equal(t, journalAPI, got[0].Entity)
	assert.Equal(t, "alice", got[0].Change.Actor)
	rel, target, ok := got[0].Change.RemovedLink()
	require.True(t, ok)
	assert.Equal(t, inventory.Calls, rel)
	assert.Equal(t, journalSvc, target)
	assert.Empty(t, r.due(time.Hour), "recorded once")
}

func TestLinkJournalKeepsCallsTheTemplateStillNames(t *testing.T) {
	r := newJournalRig(t)
	other := inventory.CoreID(KindService, "shop", "cache")
	r.observe(journalAPI, callAttrs(journalSvc, other))
	r.observe(journalAPI, callAttrs(other))
	got := r.due(LinkFlapWindow)
	require.Len(t, got, 1)
	_, target, _ := got[0].Change.RemovedLink()
	assert.Equal(t, journalSvc, target, "only the dropped call")
}

// Pods of one revision come and go; the workload's template is the same.
func TestLinkJournalIgnoresPodsRestartingUnderACallingWorkload(
	t *testing.T,
) {
	r := newJournalRig(t)
	r.observe(journalAPI, callAttrs(journalSvc))
	pod := inventory.CoreID(KindPod, "shop", "api-1")
	r.observe(pod, nil)
	r.relate(pod, inventory.Calls, journalSvc)
	r.relate(pod, inventory.Calls)
	r.apply(inventory.Observation{Kind: inventory.Gone, Entity: pod})
	r.observe(journalAPI, callAttrs(journalSvc))
	assert.Empty(t, r.due(time.Hour))
}

func TestLinkJournalIgnoresACallBackWithinAMinute(t *testing.T) {
	r := newJournalRig(t)
	r.observe(journalAPI, callAttrs(journalSvc))
	r.observe(journalAPI, nil)
	r.now = r.now.Add(20 * time.Second)
	r.observe(journalAPI, callAttrs(journalSvc))
	assert.Empty(t, r.due(time.Hour))
}

func TestWorkloadDescribeListsTheCallsOfItsTemplate(t *testing.T) {
	deploy := func(env ...corev1.EnvVar) *appsv1.Deployment {
		d := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
			Name: "api", Namespace: "shop"}}
		d.Spec.Template.Spec.Containers = []corev1.Container{{
			Name: "app", Env: env}}
		return d
	}
	describe := func(d *appsv1.Deployment) map[string]inventory.Value {
		desc, ok := DeploymentSchema().Describe(d)
		require.True(t, ok)
		return desc.Attributes
	}
	attrs := describe(deploy(
		corev1.EnvVar{Name: "PAYMENTS_URL", Value: "http://payments:80"},
		corev1.EnvVar{Name: "DB_URL",
			Value: "postgres://db.example.com:5432/x"}))
	assert.Equal(t, "external-endpoint//db.example.com:5432,"+
		"service/shop/payments", attrs[AttrTemplateCalls].AsText())
	_, has := describe(deploy())[AttrTemplateCalls]
	assert.False(t, has, "absent when the template calls nothing")
}
