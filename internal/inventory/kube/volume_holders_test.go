package kube

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
)

func holderModel(t *testing.T) (*inventory.Model, inventory.EntityID) {
	t.Helper()
	m := inventory.NewModel(inventory.Options{})
	apply := func(o inventory.Observation) {
		o.Source = "test"
		if _, err := m.Apply(o); err != nil {
			t.Fatal(err)
		}
	}
	claim := inventory.CoreID(KindPVC, "shop", "data")
	pod := inventory.CoreID(KindPod, "shop", "api-7f")
	old := inventory.CoreID(KindPod, "shop", "api-6d")
	n1 := inventory.CoreID(KindNode, "", "n1")
	n2 := inventory.CoreID(KindNode, "", "n2")
	for _, id := range []inventory.EntityID{claim, pod, n1, n2} {
		apply(inventory.Observation{Kind: inventory.Observed, Entity: id,
			Attributes: map[string]inventory.Value{}})
	}
	apply(inventory.Observation{Kind: inventory.Observed, Entity: old,
		Attributes: map[string]inventory.Value{
			AttrDeleting: inventory.Bool(true)}})
	for from, to := range map[inventory.EntityID]inventory.EntityID{
		pod: n2, old: n1} {
		apply(inventory.Observation{Kind: inventory.Related, Entity: from,
			Relation: inventory.RunsOn, Targets: []inventory.EntityID{to}})
	}
	for _, from := range []inventory.EntityID{pod, old} {
		apply(inventory.Observation{Kind: inventory.Related, Entity: from,
			Relation: inventory.Mounts,
			Targets:  []inventory.EntityID{claim}})
	}
	return m, pod
}

func TestVolumeHoldersNamePodNodeAndEnding(t *testing.T) {
	m, pod := holderModel(t)

	got := VolumeHolders(m, pod)

	if len(got) != 1 || got[0].Pod.Name != "api-6d" ||
		got[0].Node.Name != "n1" || !got[0].Terminating {
		t.Fatalf("got %+v", got)
	}
}

func TestVolumeHoldersIgnorePodsOnTheSameNode(t *testing.T) {
	m, pod := holderModel(t)
	old := inventory.CoreID(KindPod, "shop", "api-6d")
	n2 := inventory.CoreID(KindNode, "", "n2")
	if _, err := m.Apply(inventory.Observation{Kind: inventory.Related,
		Source: "test", Entity: old, Relation: inventory.RunsOn,
		Targets: []inventory.EntityID{n2}}); err != nil {
		t.Fatal(err)
	}

	if got := VolumeHolders(m, pod); len(got) != 0 {
		t.Fatalf("a pod on the same node holds nothing: %+v", got)
	}
}

func TestVolumeHoldersFallBackToTheAttachedNode(t *testing.T) {
	m, pod := holderModel(t)
	old := inventory.CoreID(KindPod, "shop", "api-6d")
	claim := inventory.CoreID(KindPVC, "shop", "data")
	volume := inventory.CoreID(KindPV, "", "pv-1")
	attach := inventory.CoreID(KindVolumeAttachment, "", "csi-1")
	n1 := inventory.CoreID(KindNode, "", "n1")
	for _, id := range []inventory.EntityID{volume, attach} {
		if _, err := m.Apply(inventory.Observation{Kind: inventory.Observed,
			Source: "test", Entity: id,
			Attributes: map[string]inventory.Value{}}); err != nil {
			t.Fatal(err)
		}
	}
	relate := func(from inventory.EntityID, rel inventory.RelationType,
		to inventory.EntityID) {
		if _, err := m.Apply(inventory.Observation{Kind: inventory.Related,
			Source: "test", Entity: from, Relation: rel,
			Targets: []inventory.EntityID{to}}); err != nil {
			t.Fatal(err)
		}
	}
	relate(claim, inventory.References, volume)
	relate(attach, inventory.References, volume)
	relate(attach, inventory.RunsOn, n1)
	// The old pod is gone; only its attachment is left.
	if _, err := m.Apply(inventory.Observation{Kind: inventory.Gone,
		Source: "test", Entity: old}); err != nil {
		t.Fatal(err)
	}

	got := VolumeHolders(m, pod)

	if len(got) != 1 || !got[0].Pod.IsZero() || got[0].Node != n1 {
		t.Fatalf("got %+v", got)
	}
}

func TestVolumeHoldersSkipFinishedAndWaitingPods(t *testing.T) {
	for _, phase := range []string{"Succeeded", "Failed", "Pending"} {
		t.Run(phase, func(t *testing.T) {
			m, pod := holderModel(t)
			old := inventory.CoreID(KindPod, "shop", "api-6d")
			if _, err := m.Apply(inventory.Observation{
				Kind: inventory.Observed, Source: "test", Entity: old,
				Attributes: map[string]inventory.Value{
					AttrPhase: inventory.Text(phase)}}); err != nil {
				t.Fatal(err)
			}

			if got := VolumeHolders(m, pod); len(got) != 0 {
				t.Fatalf("a %s pod holds nothing: %+v", phase, got)
			}
		})
	}
}

func TestVolumeHoldersKeepRunningPods(t *testing.T) {
	m, pod := holderModel(t)
	old := inventory.CoreID(KindPod, "shop", "api-6d")
	if _, err := m.Apply(inventory.Observation{Kind: inventory.Observed,
		Source: "test", Entity: old,
		Attributes: map[string]inventory.Value{
			AttrPhase: inventory.Text("Running")}}); err != nil {
		t.Fatal(err)
	}

	if got := VolumeHolders(m, pod); len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
}
