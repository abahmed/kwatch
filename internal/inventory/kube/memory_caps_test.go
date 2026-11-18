package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/abahmed/kwatch/internal/inventory"
)

func TestLimitMaxMemoryReadsTheContainerMax(t *testing.T) {
	lr := &corev1.LimitRange{Spec: corev1.LimitRangeSpec{
		Limits: []corev1.LimitRangeItem{
			{Type: corev1.LimitTypePod, Max: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("2Gi")}},
			{Type: corev1.LimitTypeContainer, Max: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("512Mi")}},
		}}}
	attrs := map[string]inventory.Value{}
	addLimitMaxMemory(attrs, lr)
	assert.Equal(t, 512.0*mib, attrNumber(attrs, AttrLimitMaxMemory))

	none := map[string]inventory.Value{}
	addLimitMaxMemory(none, &corev1.LimitRange{})
	assert.NotContains(t, none, AttrLimitMaxMemory)
}

func TestQuotaMemoryLeft(t *testing.T) {
	q := &corev1.ResourceQuota{Status: corev1.ResourceQuotaStatus{
		Hard: corev1.ResourceList{
			corev1.ResourceLimitsMemory: resource.MustParse("4Gi")},
		Used: corev1.ResourceList{
			corev1.ResourceLimitsMemory: resource.MustParse("3Gi")},
	}}
	attrs := map[string]inventory.Value{}
	addQuotaMemoryLeft(attrs, q)
	assert.Equal(t, 1024.0*mib, attrNumber(attrs, AttrQuotaMemoryLeft))

	none := map[string]inventory.Value{}
	addQuotaMemoryLeft(none, &corev1.ResourceQuota{})
	assert.NotContains(t, none, AttrQuotaMemoryLeft)
}
