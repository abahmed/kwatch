package handler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
)

func TestDetectCronJobIssueSuspended(t *testing.T) {
	suspend := true
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: "cj1", Namespace: "ns1"},
		Spec:       batchv1.CronJobSpec{Suspend: &suspend},
	}
	sig := DetectCronJobIssue(cj)
	assert.NotNil(t, sig)
	assert.Equal(t, "CronJobSuspended", sig.Reason)
}

func TestDetectCronJobIssueNotScheduled(t *testing.T) {
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: "cj1", Namespace: "ns1", CreationTimestamp: metav1.NewTime(time.Now().Add(-48 * time.Hour))},
		Spec:       batchv1.CronJobSpec{Schedule: "*/5 * * * *"},
	}
	sig := DetectCronJobIssue(cj)
	assert.NotNil(t, sig)
	assert.Equal(t, "CronJobNotScheduled", sig.Reason)
}

func TestDetectCronJobIssueNoIssue(t *testing.T) {
	now := metav1.NewTime(time.Now())
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: "cj1", Namespace: "ns1", CreationTimestamp: now},
		Spec:       batchv1.CronJobSpec{Schedule: "*/5 * * * *"},
		Status:     batchv1.CronJobStatus{LastScheduleTime: &now},
	}
	assert.Nil(t, DetectCronJobIssue(cj))
}

func TestNextFireAfter(t *testing.T) {
	now := time.Now()
	fire := NextFireAfter("*/5 * * * *", nil, now, nil)
	assert.False(t, fire.IsZero())
	assert.True(t, fire.After(now))
}

func TestNextFireAfterInvalidSchedule(t *testing.T) {
	assert.True(t, NextFireAfter("invalid", nil, time.Now(), nil).IsZero())
}

func TestNextFireAfterWithLastSchedule(t *testing.T) {
	now := time.Now()
	last := metav1.NewTime(now.Add(-10 * time.Minute))
	fire := NextFireAfter("*/5 * * * *", &last, time.Time{}, nil)
	assert.False(t, fire.IsZero())
}

func TestNextFireAfterWithTimezone(t *testing.T) {
	now := time.Now()
	last := metav1.NewTime(now.Add(-10 * time.Minute))
	tz := "America/New_York"
	fire := NextFireAfter("*/5 * * * *", &last, time.Time{}, &tz)
	assert.False(t, fire.IsZero())
}

func TestNextFireAfterInvalidTimezone(t *testing.T) {
	now := time.Now()
	last := metav1.NewTime(now.Add(-10 * time.Minute))
	tz := "NotAReal/Timezone"
	fire := NextFireAfter("*/5 * * * *", &last, time.Time{}, &tz)
	assert.False(t, fire.IsZero(), "should fall back to UTC and compute next fire")
}

func TestDefaultNextFire(t *testing.T) {
	now := time.Now()
	fire := DefaultNextFire(nil, now)
	assert.Equal(t, now.Add(24*time.Hour), fire)

	last := metav1.NewTime(now)
	fire2 := DefaultNextFire(&last, now)
	assert.Equal(t, last.Time.Add(24*time.Hour), fire2)
}

func TestProcessCronJobObjectNil(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	assert.NoError(t, h.ProcessCronJobObject(nil, false))
}

func TestProcessCronJobObjectDeleted(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: "cj1", Namespace: "ns1"},
	}
	assert.NoError(t, h.ProcessCronJobObject(cj, true))
}

func TestProcessCronJobObjectSuspended(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	suspend := true
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: "cj1", Namespace: "ns1"},
		Spec:       batchv1.CronJobSpec{Suspend: &suspend},
	}
	assert.NoError(t, h.ProcessCronJobObject(cj, false))
	assert.Equal(t, 1, e.ActiveCount())
}

func TestProcessCronJobObjectNotScheduled(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: "cj1", Namespace: "ns1", CreationTimestamp: metav1.NewTime(time.Now().Add(-48 * time.Hour))},
		Spec:       batchv1.CronJobSpec{Schedule: "*/5 * * * *"},
	}
	assert.NoError(t, h.ProcessCronJobObject(cj, false))
	assert.Equal(t, 1, e.ActiveCount())
}

func TestProcessCronJobObjectHealthy(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	now := metav1.NewTime(time.Now())
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: "cj1", Namespace: "ns1", CreationTimestamp: now},
		Spec:       batchv1.CronJobSpec{Schedule: "*/5 * * * *"},
		Status:     batchv1.CronJobStatus{LastScheduleTime: &now},
	}
	assert.NoError(t, h.ProcessCronJobObject(cj, false))
	assert.Equal(t, 0, e.ActiveCount())
}

func TestProcessCronJobInvalidKey(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	assert.Error(t, h.ProcessCronJob("a/b/c", false))
}

func TestProcessCronJobDeleted(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	assert.NoError(t, h.ProcessCronJob("ns1/cj1", true))
}

func TestProcessCronJobNotFound(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	client := fake.NewSimpleClientset()
	f := informers.NewSharedInformerFactory(client, 0)
	h.SetCronJobLister(f.Batch().V1().CronJobs().Lister())
	assert.NoError(t, h.ProcessCronJob("ns1/missing", false))
}

func TestProcessCronJobSuspended(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	suspend := true
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: "cj1", Namespace: "ns1"},
		Spec:       batchv1.CronJobSpec{Suspend: &suspend},
	}
	client := fake.NewSimpleClientset()
	f := informers.NewSharedInformerFactory(client, 0)
	f.Batch().V1().CronJobs().Informer().GetIndexer().Add(cj)
	h.SetCronJobLister(f.Batch().V1().CronJobs().Lister())
	assert.NoError(t, h.ProcessCronJob("ns1/cj1", false))
	assert.Equal(t, 1, e.ActiveCount())
}

func TestMarkFirstSuspendedCJHit(t *testing.T) {
	e := testCorrelator()
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	t1 := h.markFirstSuspendedCJ("ns1/cj1")
	t2 := h.markFirstSuspendedCJ("ns1/cj1")
	assert.Equal(t, t1, t2, "second call should return existing entry")
}
