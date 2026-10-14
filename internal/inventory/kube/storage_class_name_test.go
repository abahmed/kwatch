package kube

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func describeClaim(
	t *testing.T, pvc *corev1.PersistentVolumeClaim,
) Description {
	t.Helper()
	d, ok := PVCSchema{}.Describe(pvc)
	if !ok {
		t.Fatal("claim not described")
	}
	return d
}

func TestClaimWithoutClassIsMarkedUnset(t *testing.T) {
	d := describeClaim(t, &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "data", Namespace: "shop"}})

	if unset, _ := d.Attributes[AttrClassUnset].AsBool(); !unset {
		t.Fatalf("claim with no class is not marked: %v", d.Attributes)
	}
}

func TestClaimWithSelectorIsMarked(t *testing.T) {
	plain := describeClaim(t, &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "shop"}})
	picky := describeClaim(t, &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "shop"},
		Spec: corev1.PersistentVolumeClaimSpec{
			Selector: &metav1.LabelSelector{}}})

	if _, ok := plain.Attributes[AttrHasSelector]; ok {
		t.Error("claim without selector is marked")
	}
	if has, _ := picky.Attributes[AttrHasSelector].AsBool(); !has {
		t.Errorf("claim with selector is not marked: %v", picky.Attributes)
	}
}

func TestClaimNamingAClassIsNotMarkedUnset(t *testing.T) {
	named := "fast"
	byField := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "shop"},
		Spec:       corev1.PersistentVolumeClaimSpec{StorageClassName: &named}}
	byNote := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "shop",
			Annotations: map[string]string{claimClassBetaAnnotation: "fast"}}}
	explicitNone := ""
	optedOut := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "c", Namespace: "shop"},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &explicitNone}}

	for _, pvc := range []*corev1.PersistentVolumeClaim{
		byField, byNote, optedOut,
	} {
		if _, ok := describeClaim(t, pvc).Attributes[AttrClassUnset]; ok {
			t.Errorf("claim %s is marked as setting no class", pvc.Name)
		}
	}
}

func TestDefaultClassIsMarked(t *testing.T) {
	for _, key := range []string{defaultClassAnnotation,
		defaultClassBetaAnnotation} {
		d, _ := StorageClassSchema{}.Describe(&storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{Name: "std",
				Annotations: map[string]string{key: "true"}}})
		if def, _ := d.Attributes[AttrDefaultClass].AsBool(); !def {
			t.Errorf("%s does not mark the default class", key)
		}
	}
	d, _ := StorageClassSchema{}.Describe(&storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{Name: "fast"}})
	if _, ok := d.Attributes[AttrDefaultClass]; ok {
		t.Error("a class without the annotation is marked default")
	}
}
