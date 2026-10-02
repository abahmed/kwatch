package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestClaimPendingBindingMode(t *testing.T) {
	now := t0.Add(10 * time.Minute)
	tt := []struct {
		name     string
		mode     string
		hasClass bool
		mounted  bool
		want     int
	}{
		{"wait_for_first_consumer_without_pod", "WaitForFirstConsumer",
			true, false, 0},
		{"wait_for_first_consumer_with_pod", "WaitForFirstConsumer",
			true, true, 1},
		{"immediate_without_pod", "Immediate", true, false, 1},
		{"unknown_class_without_pod", "", false, false, 1},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel()
			claim := newID(kube.KindPVC, "default", "data")
			put(m, claim, t0, map[string]inventory.Value{
				kube.AttrPhase: inventory.Text("Pending"),
			})
			if tc.hasClass {
				class := newID(kube.KindStorageClass, "", "fast")
				put(m, class, t0, map[string]inventory.Value{
					kube.AttrBindingMode: inventory.Text(tc.mode),
				})
				link(m, claim, inventory.References, class)
			}
			if tc.mounted {
				pod := newID(kube.KindPod, "default", "app")
				put(m, pod, t0, nil)
				link(m, pod, inventory.Mounts, claim)
			}
			entity, _ := m.Entity(claim)
			got := Claim{}.Detect(testDetectorContext(m, now), entity)
			assert.Len(t, got, tc.want)
		})
	}
}
