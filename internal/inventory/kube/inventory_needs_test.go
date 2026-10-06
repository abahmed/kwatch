package kube_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestQuotaNamesZeroHardResources(t *testing.T) {
	q := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Name: "q", Namespace: "ns"},
		Status: corev1.ResourceQuotaStatus{
			Hard: corev1.ResourceList{
				"pods": mustQuantity("10"), "secrets": mustQuantity("0"),
				"services.nodeports": mustQuantity("0"),
			},
			Used: corev1.ResourceList{
				"pods": mustQuantity("1"), "secrets": mustQuantity("0"),
				"services.nodeports": mustQuantity("0"),
			},
		},
	}
	d, _ := kube.QuotaSchema{}.Describe(q)
	assert.Equal(t, "secrets,services.nodeports",
		text(d, kube.AttrQuotaZeroHard))
}

func TestPodRecordsDeletionTimeAndGracePeriod(t *testing.T) {
	p := pod("p")
	grace := int64(45)
	p.Spec.TerminationGracePeriodSeconds = &grace
	d, _ := kube.PodSchema{}.Describe(p)
	assert.NotContains(t, d.Attributes, kube.AttrDeletionTime)
	grace45, _ := d.Attributes[kube.AttrTerminationGrace].AsNumber()
	assert.Equal(t, 45.0, grace45)

	when := metav1.NewTime(fixedTime())
	p.DeletionTimestamp = &when
	d, _ = kube.PodSchema{}.Describe(p)
	assert.True(t, fixedTime().Equal(
		d.Attributes[kube.AttrDeletionTime].AsTime()))
}

func TestFinishedAndPendingCallersDoNotCountAsFailingUsers(t *testing.T) {
	for phase, want := range map[string]int{
		"Succeeded": 1, "Pending": 1, "Running": 3,
	} {
		model := podCalling(t, "a.example.com:443", "b.example.com:443",
			"c.example.com:443")
		_, err := model.(*inventory.Model).Apply(inventory.Observation{
			Kind: inventory.Observed, Source: kube.ObservationSource,
			At: fixedTime(), Entity: inventory.CoreID(
				kube.KindPod, "shop", "api"),
			Attributes: map[string]inventory.Value{
				kube.AttrPhase: inventory.Text(phase),
				kube.AttrReady: inventory.Bool(false)},
		})
		require.NoError(t, err)
		observations := runActive(t, kube.ActiveProbeConfig{
			AutoDependencies: true, Model: model,
			Dial: failWith(context.DeadlineExceeded),
		})
		assert.Len(t, observations, want, phase)
	}
}

func TestCronJobSuspendedAttributeKnowsSinceWhen(t *testing.T) {
	m := inventory.NewModel(inventory.Options{})
	cron := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: "c", Namespace: "ns"},
		Spec:       batchv1.CronJobSpec{Schedule: "* * * * *"},
	}
	id := inventory.CoreID(kube.KindCronJob, "ns", "c")
	observe := func(suspend bool, at time.Time) {
		cron.Spec.Suspend = &suspend
		d, _ := kube.CronJobSchema{}.Describe(cron)
		_, err := m.Apply(inventory.Observation{
			Kind: inventory.Observed, Source: kube.ObservationSource,
			At: at, Entity: id, Attributes: d.Attributes})
		require.NoError(t, err)
	}
	t0 := fixedTime()
	observe(true, t0)
	observe(true, t0.Add(time.Hour))
	observe(false, t0.Add(2*time.Hour))
	observe(false, t0.Add(3*time.Hour))
	e, _ := m.Entity(id)
	attr, _ := e.Attribute(kube.AttrSuspended)
	assert.True(t, attr.Since.Equal(t0.Add(2*time.Hour)),
		"Since is when kwatch saw the CronJob resume")
}
