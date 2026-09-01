package handler

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/config"
)

func TestPodEventsComeFromIndexWhenAvailable(t *testing.T) {
	e := testCorrelator()
	client := fake.NewSimpleClientset()
	h := NewHandler(
		client,
		&config.Config{MaxRecentLogLines: 10},
		e,
		testAlertMgr,
	)
	f := informers.NewSharedInformerFactory(client, 0)

	// The lister errors, so any events that show up came from the index.
	h.listers.Event = &errorEventLister{f.Core().V1().Events().Lister()}
	indexHits := 0
	h.listers.EventsByPod = func(ns, pod string) ([]*corev1.Event, error) {
		indexHits++
		assert.Equal(t, "default", ns)
		assert.Equal(t, "broken-pod", pod)
		return []*corev1.Event{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ev-from-index",
					Namespace: ns,
				},
				InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: pod},
				Type:           "Warning",
				Reason:         "IndexedReason",
				Message:        "served from the informer index",
			},
		}, nil
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "broken-pod", Namespace: "default"},
		Spec:       corev1.PodSpec{NodeName: "node1"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 5,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "CrashLoopBackOff",
							Message: "backoff restart",
						},
					},
				},
			},
		},
	}
	assert.NoError(t, h.ProcessPodObject(context.Background(), pod, false))
	assert.Equal(t, 1, indexHits, "the index must be consulted exactly once")

	var found bool
	for _, inc := range e.SnapshotAll() {
		if strings.Contains(inc.Events, "IndexedReason") {
			found = true
		}
	}
	assert.True(
		t,
		found,
		"the incident's events must come from the index, not the (failing) "+
			"lister",
	)
}
