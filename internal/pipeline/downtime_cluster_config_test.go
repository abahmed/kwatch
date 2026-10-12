package pipeline

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func modelWith(
	t *testing.T, schema kube.Schema, obj any, now time.Time,
) *inventory.Model {
	t.Helper()
	m := inventory.NewModel(inventory.Options{})
	for _, o := range kube.NewTranslator(schema).Added(obj, true, now) {
		if _, err := m.Apply(o); err != nil {
			t.Fatal(err)
		}
	}
	return m
}

// What decides which pods a namespace admits and which bounds they live
// within is compared across a restart: an edit made while kwatch was
// down is found, dated at the snapshot and worded as the live diff would.
func TestDowntimeFindsAdmissionAndAccountEdits(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	namespace := func(labels map[string]string) *corev1.Namespace {
		return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name: "shop", UID: "ns", Labels: labels}}
	}
	limits := func(cpu string) *corev1.LimitRange {
		return &corev1.LimitRange{
			ObjectMeta: metav1.ObjectMeta{Name: "bounds", Namespace: "shop",
				UID: "lr"},
			Spec: corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{{
				Type: corev1.LimitTypeContainer,
				Max: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse(cpu)}}}}}
	}
	account := func(secret string) *corev1.ServiceAccount {
		return &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: "default",
				Namespace: "shop", UID: "sa"},
			ImagePullSecrets: []corev1.LocalObjectReference{{Name: secret}}}
	}
	enforce := map[string]string{
		"pod-security.kubernetes.io/enforce": "restricted"}
	cases := map[string]struct {
		schema        kube.Schema
		before, after any
		path          string
	}{
		"pod security label": {kube.NamespaceSchema{}, namespace(nil),
			namespace(enforce), "labels.pod-security"},
		"limit range bounds": {kube.LimitRangeSchema{}, limits("4"),
			limits("1"), "spec.limits"},
		"pull secrets": {kube.ServiceAccountSchema{}, account("reg-a"),
			account("reg-b"), "imagePullSecrets"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			saved := savedFingerprints(t,
				modelWith(t, tc.schema, tc.before, now), now.Add(-time.Hour))

			same := DowntimeChanges(modelWith(t, tc.schema, tc.before, now),
				saved, nil, now)
			got := DowntimeChanges(modelWith(t, tc.schema, tc.after, now),
				saved, nil, now)

			if len(same) != 0 {
				t.Fatalf("unchanged object produced %v", same)
			}
			if len(got) != 1 || len(got[0].Change.Fields) != 1 ||
				got[0].Change.Fields[0].Path != tc.path {
				t.Fatalf("changes = %+v, want one at %s", got, tc.path)
			}
			if !got[0].Change.At.Equal(now.Add(-time.Hour)) {
				t.Errorf("dated %v, want the snapshot time",
					got[0].Change.At)
			}
		})
	}
}
