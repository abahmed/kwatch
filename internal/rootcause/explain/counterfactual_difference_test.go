package explain

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

const (
	imgRef  = "registry.example.com/api:latest"
	digestA = "registry.example.com/api@sha256:" +
		"aaaa1111bbbb2222cccc3333dddd4444eeee5555ffff66667777888899990000"
	digestB = "registry.example.com/api@sha256:" +
		"9f1c0a2b44d1eeee5555ffff66667777888899990000aaaa1111bbbb22223333"
)

// runContainer sets what the kubelet reports for a pod's container:
// the image reference, the digest it runs and when that was first seen.
func (f *fixture) runContainer(
	container inventory.EntityID, image, imageID string, seen time.Time,
) {
	f.t.Helper()
	_, err := f.model.Apply(inventory.Observation{
		Kind: inventory.Observed, Entity: container, Source: "test",
		At: seen, Attributes: map[string]inventory.Value{
			kube.AttrImage:   inventory.Text(image),
			kube.AttrImageID: inventory.Text(imageID),
		}})
	if err != nil {
		f.t.Fatal(err)
	}
}

// sidecar adds an injected container to a pod.
func (f *fixture) sidecar(pod inventory.EntityID, name, image string) {
	c := inventory.CoreID(kube.KindContainer, pod.Namespace,
		pod.Name+"/"+name)
	f.add(c)
	f.relate(c, inventory.PartOf, pod)
	f.apply(inventory.Observation{Kind: inventory.Observed, Entity: c,
		Attributes: map[string]inventory.Value{
			kube.AttrImage: inventory.Text(image)}})
}

// zoneFixture runs four replicas on four nodes: failing ones on nodes
// of zone-b, the rest on nodes of zone-a. mixed puts one healthy pod
// in zone-b as well.
func zoneFixture(t *testing.T, mixed bool) *fixture {
	f := newFixture(t)
	a := f.nodes("zone-a", "a1", "a2")
	b := f.nodes("zone-b", "b1", "b2")
	pods := f.workload("shop", "api", 4)
	f.relate(pods[0], inventory.RunsOn, b[0])
	f.relate(pods[1], inventory.RunsOn, b[1])
	f.relate(pods[2], inventory.RunsOn, a[0])
	f.relate(pods[3], inventory.RunsOn, a[1])
	if mixed {
		f.relate(pods[3], inventory.RunsOn, b[0])
	}
	f.ready(pods...)
	f.crash(pods[0], pods[1])
	f.change(deployment("shop", "api"), 1, envPath)
	return f
}

func TestScoreDifferenceZone(t *testing.T) {
	dep := deployment("shop", "api")
	o := scoreOf(t, zoneFixture(t, false), dep, scoreDifference)
	requireWeight(t, o, -DifferenceWeight)
	if o.code != rootcause.ProofReplicasDiffer {
		t.Fatalf("code = %q", o.code)
	}
	want := "2 of 4 pods fail; all run in zone zone-b, the 2 healthy " +
		"ones in zone zone-a"
	if o.text != want {
		t.Fatalf("text = %q, want %q", o.text, want)
	}
	t.Run("a healthy pod in the same zone", func(t *testing.T) {
		requireWeight(t, scoreOf(t, zoneFixture(t, true), dep,
			scoreDifference), 0)
	})
}

func TestScoreDifferenceSilent(t *testing.T) {
	dep := deployment("shop", "api")
	t.Run("one failing pod", func(t *testing.T) {
		g := newFixture(t)
		a := g.nodes("zone-a", "a1")
		b := g.nodes("zone-b", "b1")
		pods := g.workload("shop", "api", 2, b[0])
		g.relate(pods[1], inventory.RunsOn, a[0])
		g.ready(pods...)
		g.crash(pods[0])
		g.change(dep, 1, envPath)
		requireWeight(t, scoreOf(t, g, dep, scoreDifference), 0)
	})
	t.Run("no healthy pod", func(t *testing.T) {
		f := zoneFixture(t, false)
		f.crash(inventory.CoreID(kube.KindPod, "shop", "api-1-2"),
			inventory.CoreID(kube.KindPod, "shop", "api-1-3"))
		requireWeight(t, scoreOf(t, f, dep, scoreDifference), 0)
	})
	t.Run("a zone is not judged", func(t *testing.T) {
		zone := inventory.CoreID(kube.KindZone, "", "zone-b")
		f := zoneFixture(t, false)
		f.fail(zone, "NotReady", failingH, 1, "")
		requireWeight(t, scoreOf(t, f, zone, scoreDifference), 0)
	})
}

// digestFixture runs api:latest on four pods: the two failing ones run
// digestB, first seen at 10:02, the healthy ones digestA.
func digestFixture(t *testing.T, healthyDigest string) *fixture {
	f := newFixture(t)
	nodes := f.nodes("zone-a", "n1", "n2")
	pods := f.workload("shop", "api", 4, nodes...)
	f.ready(pods...)
	f.crash(pods[0], pods[1])
	f.change(deployment("shop", "api"), 1, envPath)
	seen := t0.Add(2 * time.Minute)
	for i, pod := range pods {
		id := healthyDigest
		if i < 2 {
			id = digestB
		}
		f.runContainer(containerOf(pod), imgRef, id, seen)
	}
	return f
}

func TestScoreDifferenceDigest(t *testing.T) {
	dep := deployment("shop", "api")
	o := scoreOf(t, digestFixture(t, digestA), dep, scoreDifference)
	requireWeight(t, o, -DifferenceWeight)
	want := "same tag " + imgRef + ", but the failing pods run " +
		"sha256:9f1c0a2b44d1 (seen 10:02), the healthy ones sha256:aaaa1111bbbb"
	if o.text != want {
		t.Fatalf("text = %q\nwant   %q", o.text, want)
	}
	t.Run("same digest everywhere", func(t *testing.T) {
		requireWeight(t, scoreOf(t, digestFixture(t, digestB), dep,
			scoreDifference), 0)
	})
}

func TestScoreDifferenceSidecar(t *testing.T) {
	dep := deployment("shop", "api")
	f := newFixture(t)
	nodes := f.nodes("zone-a", "n1", "n2")
	pods := f.workload("shop", "api", 4, nodes...)
	f.ready(pods...)
	f.crash(pods[0], pods[1])
	f.change(dep, 1, envPath)
	for _, pod := range pods {
		f.runContainer(containerOf(pod), imgRef, digestA, t0)
	}
	for _, pod := range pods[:2] {
		f.sidecar(pod, "istio-proxy", "istio/proxyv2:1.20")
	}
	o := scoreOf(t, f, dep, scoreDifference)
	requireWeight(t, o, -DifferenceWeight)
	if !strings.Contains(o.text, "extra container istio-proxy") {
		t.Fatalf("text = %q", o.text)
	}
}

func TestDigestShort(t *testing.T) {
	if got := shortDigest(digestB); got != "sha256:9f1c0a2b44d1" {
		t.Fatalf("got %q", got)
	}
	if got := shortDigest("not-a-digest"); got != "" {
		t.Fatalf("got %q", got)
	}
}
