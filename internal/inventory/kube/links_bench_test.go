package kube_test

import (
	"strconv"
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// benchModel builds 50 namespaces of 100 labeled pods (5,000 pods), one
// Deployment and one selecting Service per namespace, and one Widget
// managed by a Deployment name.
func benchModel(b *testing.B) (*inventory.Model, inventory.EntityID) {
	b.Helper()
	model := inventory.NewModel(inventory.Options{})
	observe := func(id inventory.EntityID, attrs map[string]inventory.Value) {
		_, err := model.Apply(inventory.Observation{
			Kind: inventory.Observed, Source: kube.ObservationSource,
			At: fixedTime(), Entity: id, Attributes: attrs,
		})
		if err != nil {
			b.Fatal(err)
		}
	}
	var service inventory.EntityID
	for n := 0; n < 50; n++ {
		ns := "ns" + strconv.Itoa(n)
		for p := 0; p < 100; p++ {
			app := "app" + strconv.Itoa(p%10)
			observe(inventory.CoreID(kube.KindPod, ns, "pod"+strconv.Itoa(p)),
				map[string]inventory.Value{
					kube.AttrLabels: inventory.Text("app=" + app + ",tier=web"),
				})
		}
		observe(inventory.CoreID(kube.KindDeployment, ns, "op"+ns), nil)
		service = inventory.CoreID(kube.KindService, ns, "web")
		observe(service, map[string]inventory.Value{
			kube.AttrSelector: inventory.Text("app=app3"),
		})
	}
	observe(inventory.NewEntityID("example.com", "widget", "ns7", "w"),
		map[string]inventory.Value{
			kube.AttrManagedBy: inventory.Text("opns3"),
		})
	return model, service
}

func BenchmarkLinksServiceSelector5kPods(b *testing.B) {
	model, service := benchModel(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if len(kube.Links(model, service)) != 10 {
			b.Fatal("want 10 selected pods")
		}
	}
}

func BenchmarkLinksManagedBy5kPods(b *testing.B) {
	model, _ := benchModel(b)
	widget := inventory.NewEntityID("example.com", "widget", "ns7", "w")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if len(kube.Links(model, widget)) != 1 {
			b.Fatal("want one manager link")
		}
	}
}

func BenchmarkEntities5kPods(b *testing.B) {
	model, _ := benchModel(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if len(model.Entities(kube.KindPod)) != 5000 {
			b.Fatal("want 5000 pods")
		}
	}
}
