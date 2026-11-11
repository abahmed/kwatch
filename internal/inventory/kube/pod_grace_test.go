package kube_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A pod being deleted with its own grace (a drain, --grace-period=5)
// stops within that grace, not its spec's: the deletion's grace is the
// one recorded while it is deleted.
func TestPodGraceIsTheDeletionsWhileDeleting(t *testing.T) {
	spec, short := int64(30), int64(5)
	at := metav1.NewTime(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	for name, tc := range map[string]struct {
		deleting bool
		want     float64
	}{
		"running":         {false, 30},
		"deleted with 5s": {true, 5},
	} {
		t.Run(name, func(t *testing.T) {
			p := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
				Name: "p", Namespace: "ns"},
				Spec: corev1.PodSpec{TerminationGracePeriodSeconds: &spec}}
			if tc.deleting {
				p.DeletionTimestamp = &at
				p.DeletionGracePeriodSeconds = &short
			}
			desc, ok := kube.PodSchema{}.Describe(p)
			assert.True(t, ok)
			got, _ := desc.Attributes[kube.AttrTerminationGrace].AsNumber()
			assert.Equal(t, tc.want, got)
		})
	}
}
