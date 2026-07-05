package handler

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDetectJobIssueFailed(t *testing.T) {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "ns1"},
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{
				{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Reason: "BackoffLimitExceeded"},
			},
		},
	}
	sig := DetectJobIssue(job)
	assert.NotNil(t, sig)
	assert.Equal(t, "BackoffLimitExceeded", sig.Reason)
}

func TestDetectJobIssueFailedEmptyReason(t *testing.T) {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "ns1"},
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{
				{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Reason: ""},
			},
		},
	}
	sig := DetectJobIssue(job)
	assert.NotNil(t, sig)
	assert.Equal(t, "JobFailed", sig.Reason)
}

func TestDetectJobIssueSuspended(t *testing.T) {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "ns1"},
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{
				{Type: batchv1.JobSuspended, Status: corev1.ConditionTrue},
			},
		},
	}
	sig := DetectJobIssue(job)
	assert.NotNil(t, sig)
	assert.Equal(t, "JobSuspended", sig.Reason)
}

func TestDetectJobIssueNoIssue(t *testing.T) {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "ns1"},
	}
	assert.Nil(t, DetectJobIssue(job))
}

func TestProcessJobObjectNil(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	assert.NoError(t, h.ProcessJobObject(nil, false))
}

func TestProcessJobObjectDeleted(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "ns1"},
	}
	assert.NoError(t, h.ProcessJobObject(job, true))
}

func TestProcessJobObjectComplete(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "ns1"},
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{
				{Type: batchv1.JobComplete, Status: corev1.ConditionTrue},
			},
		},
	}
	assert.NoError(t, h.ProcessJobObject(job, false))
	assert.Equal(t, 0, e.ActiveCount())
}

func TestProcessJobObjectFailed(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "ns1"},
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{
				{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Reason: "BackoffLimitExceeded"},
			},
		},
	}
	assert.NoError(t, h.ProcessJobObject(job, false))
	assert.Equal(t, 1, e.ActiveCount())
}

func TestProcessJobObjectHealthy(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "ns1"},
	}
	assert.NoError(t, h.ProcessJobObject(job, false))
	assert.Equal(t, 0, e.ActiveCount())
}

func TestProcessJobInvalidKey(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	assert.Error(t, h.ProcessJob("a/b/c", false))
}

func TestProcessJobDeleted(t *testing.T) {
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, testCorrelator(), testAlertMgr)
	assert.NoError(t, h.ProcessJob("ns1/job1", true))
}

func TestProcessJobNotFound(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	client := fake.NewSimpleClientset()
	f := informers.NewSharedInformerFactory(client, 0)
	h.SetJobLister(f.Batch().V1().Jobs().Lister())
	assert.NoError(t, h.ProcessJob("ns1/missing", false))
}

func TestProcessJobFailed(t *testing.T) {
	e := correlation.NewEngine(correlation.Config{Window: 10 * time.Minute})
	h := NewHandler(fake.NewSimpleClientset(), &config.Config{}, e, testAlertMgr)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "ns1"},
		Status: batchv1.JobStatus{
			Conditions: []batchv1.JobCondition{
				{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Reason: "BackoffLimitExceeded"},
			},
		},
	}
	client := fake.NewSimpleClientset()
	f := informers.NewSharedInformerFactory(client, 0)
	f.Batch().V1().Jobs().Informer().GetIndexer().Add(job)
	h.SetJobLister(f.Batch().V1().Jobs().Lister())
	assert.NoError(t, h.ProcessJob("ns1/job1", false))
	assert.Equal(t, 1, e.ActiveCount())
}
