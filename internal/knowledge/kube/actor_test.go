package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/knowledge/kube"
)

func TestActor(t *testing.T) {
	tt := []struct {
		name   string
		fields []metav1.ManagedFieldsEntry
		want   string
	}{
		{
			name: "single_manager",
			fields: []metav1.ManagedFieldsEntry{
				{Manager: "kubectl"},
			},
			want: "kubectl",
		},
		{
			name: "multiple_managers_latest_first",
			fields: []metav1.ManagedFieldsEntry{
				{Manager: "kubelet",
					Time: timePtr(fixedTime())},
				{Manager: "kubectl"},
			},
			want: "kubelet",
		},
		{
			name: "ignore_nil_times",
			fields: []metav1.ManagedFieldsEntry{
				{Manager: "old"},
				{Manager: "kubectl", Time: timePtr(fixedTime())},
			},
			want: "kubectl",
		},
		{
			name:   "empty_fields",
			fields: []metav1.ManagedFieldsEntry{},
			want:   "",
		},
		{
			name: "all_nil_times_falls_back",
			fields: []metav1.ManagedFieldsEntry{
				{Manager: "a"},
				{Manager: "b"},
			},
			want: "b", // Falls back to last manager
		},
		{
			name: "picks_latest_timestamp",
			fields: []metav1.ManagedFieldsEntry{
				{
					Manager: "old",
					Time:    timePtr(fixedTime()),
				},
				{
					Manager: "newer",
					Time: timePtr(
						fixedTime().Add(60 * 60)),
				},
			},
			want: "newer",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			obj := &metav1.ObjectMeta{}
			obj.SetManagedFields(tc.fields)
			got := kube.Actor(obj)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestTrimToLatestManager(t *testing.T) {
	tt := []struct {
		name      string
		obj       any
		checkFn   func(*testing.T, any)
		wantError bool
	}{
		{
			name: "single_entry_kept",
			obj: func() any {
				p := pod("p1")
				p.SetManagedFields(
					[]metav1.ManagedFieldsEntry{
						{Manager: "kubectl",
							Time: timePtr(
								fixedTime())},
					})
				return p
			}(),
			checkFn: func(t *testing.T, obj any) {
				p := obj.(*corev1.Pod)
				meta := metav1.Object(p)
				fields := meta.GetManagedFields()
				assert.Len(t, fields, 1)
				assert.Equal(t, "kubectl",
					fields[0].Manager)
				assert.Nil(t, fields[0].FieldsV1)
			},
		},
		{
			name: "keeps_latest_time",
			obj: func() any {
				p := pod("p1")
				p.SetManagedFields(
					[]metav1.ManagedFieldsEntry{
						{Manager: "old",
							Time: timePtr(
								fixedTime())},
						{Manager: "kubectl",
							Time: timePtr(
								fixedTime().Add(
									60 *
										60))},
					})
				return p
			}(),
			checkFn: func(t *testing.T, obj any) {
				p := obj.(*corev1.Pod)
				fields := p.GetManagedFields()
				assert.Len(t, fields, 1)
				assert.Equal(t, "kubectl",
					fields[0].Manager)
			},
		},
		{
			name: "removes_fieldsv1",
			obj: func() any {
				p := pod("p1")
				fieldsV1 := &metav1.FieldsV1{}
				p.SetManagedFields(
					[]metav1.ManagedFieldsEntry{
						{Manager: "kubectl",
							Time: timePtr(
								fixedTime()),
							FieldsV1: fieldsV1},
					})
				return p
			}(),
			checkFn: func(t *testing.T, obj any) {
				p := obj.(*corev1.Pod)
				fields := p.GetManagedFields()
				assert.Len(t, fields, 1)
				assert.Nil(t, fields[0].FieldsV1)
			},
		},
		{
			name: "empty_fields_returns_nil",
			obj: func() any {
				p := pod("p1")
				p.SetManagedFields(nil)
				return p
			}(),
			checkFn: func(t *testing.T, obj any) {
				p := obj.(*corev1.Pod)
				fields := p.GetManagedFields()
				assert.Nil(t, fields)
			},
		},
		{
			name: "all_nil_times_sets_nil",
			obj: func() any {
				p := pod("p1")
				p.SetManagedFields(
					[]metav1.ManagedFieldsEntry{
						{Manager: "a"},
						{Manager: "b"},
					})
				return p
			}(),
			checkFn: func(t *testing.T, obj any) {
				p := obj.(*corev1.Pod)
				fields := p.GetManagedFields()
				assert.Nil(t, fields)
			},
		},
		{
			name: "non_meta_object_untouched",
			obj:  "not a metadata object",
			checkFn: func(t *testing.T, obj any) {
				assert.Equal(t, "not a metadata object",
					obj)
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			result, err := kube.TrimToLatestManager(tc.obj)
			if tc.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				tc.checkFn(t, result)
			}
		})
	}
}
