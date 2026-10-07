package kube

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func namedMeta(at *time.Time, finalizers ...string) metav1.ObjectMeta {
	meta := metav1.ObjectMeta{Name: "data", Namespace: "shop",
		Finalizers: finalizers}
	if at != nil {
		stamp := metav1.NewTime(*at)
		meta.DeletionTimestamp = &stamp
	}
	return meta
}

func TestDeletingClaimsRecordTheirFinalizers(t *testing.T) {
	at := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: namedMeta(
		&at, "kubernetes.io/pvc-protection", "example.com/hold")}

	d, ok := PVCSchema{}.Describe(pvc)

	assert.True(t, ok)
	assert.Equal(t, "kubernetes.io/pvc-protection,example.com/hold",
		d.Attributes[AttrFinalizers].AsText())
	assert.True(t, d.Attributes[AttrDeletingSince].AsTime().Equal(at))
}

func TestLiveObjectsKeepNoFinalizers(t *testing.T) {
	pvc, _ := PVCSchema{}.Describe(&corev1.PersistentVolumeClaim{
		ObjectMeta: namedMeta(nil, "kubernetes.io/pvc-protection")})
	pv, _ := PVSchema{}.Describe(&corev1.PersistentVolume{
		ObjectMeta: namedMeta(nil, "kubernetes.io/pv-protection")})
	ns, _ := NamespaceSchema{}.Describe(&corev1.Namespace{
		ObjectMeta: namedMeta(nil, "example.com/hold")})

	for _, d := range []Description{pvc, pv, ns} {
		assert.NotContains(t, d.Attributes, AttrFinalizers)
		assert.NotContains(t, d.Attributes, AttrDeletingSince)
	}
}

func TestDeletingNamespaceKeepsConditionMessages(t *testing.T) {
	at := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ns := &corev1.Namespace{ObjectMeta: namedMeta(&at),
		Status: corev1.NamespaceStatus{
			Phase: corev1.NamespaceTerminating,
			Conditions: []corev1.NamespaceCondition{{
				Type:    corev1.NamespaceFinalizersRemaining,
				Status:  corev1.ConditionTrue,
				Message: "finalizers remaining: kubernetes.io/pvc-protection",
			}}}}
	ns.Spec.Finalizers = []corev1.FinalizerName{"kubernetes"}

	d, _ := NamespaceSchema{}.Describe(ns)

	assert.Equal(t, "finalizers remaining: kubernetes.io/pvc-protection",
		d.Attributes[ConditionKey("NamespaceFinalizersRemaining")+
			AttrConditionMessage].AsText())
	assert.Equal(t, "kubernetes", d.Attributes[AttrFinalizers].AsText())
}
