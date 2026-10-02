package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// buildPod creates a test pod entity with the given attributes applied.
func buildPod(name, namespace string, now time.Time,
	attrs map[string]inventory.Value) inventory.Entity {
	model := inventory.NewModel(inventory.Options{})
	id := inventory.EntityID{Kind: kube.KindPod, Namespace: namespace,
		Name: name}
	if attrs == nil {
		attrs = make(map[string]inventory.Value)
	}
	model.Apply(inventory.Observation{
		Kind:       inventory.Observed,
		Source:     "test",
		At:         now,
		Entity:     id,
		Attributes: attrs,
	})
	entity, _ := model.Entity(id)
	return entity
}

// buildContainer creates a test container entity.
func buildContainer(pod inventory.EntityID, name string,
	now time.Time, attrs map[string]inventory.Value,
) inventory.Entity {
	model := inventory.NewModel(inventory.Options{})
	id := inventory.EntityID{Kind: kube.KindContainer, Namespace: pod.Namespace,
		Name: pod.Name + "/" + name}
	if attrs == nil {
		attrs = make(map[string]inventory.Value)
	}
	model.Apply(inventory.Observation{
		Kind:       inventory.Observed,
		Source:     "test",
		At:         now,
		Entity:     id,
		Attributes: attrs,
	})
	entity, _ := model.Entity(id)
	return entity
}

// buildNode creates a test node entity.
func buildNode(name string, now time.Time,
	attrs map[string]inventory.Value) inventory.Entity {
	model := inventory.NewModel(inventory.Options{})
	id := inventory.EntityID{Kind: kube.KindNode, Name: name}
	if attrs == nil {
		attrs = make(map[string]inventory.Value)
	}
	model.Apply(inventory.Observation{
		Kind:       inventory.Observed,
		Source:     "test",
		At:         now,
		Entity:     id,
		Attributes: attrs,
	})
	entity, _ := model.Entity(id)
	return entity
}

// buildWorkload creates a test workload entity (Deployment, StatefulSet, etc).
func buildWorkload(kind inventory.Kind, name, namespace string,
	now time.Time, attrs map[string]inventory.Value,
) inventory.Entity {
	model := inventory.NewModel(inventory.Options{})
	id := inventory.EntityID{Kind: kind, Namespace: namespace, Name: name}
	if attrs == nil {
		attrs = make(map[string]inventory.Value)
	}
	model.Apply(inventory.Observation{
		Kind:       inventory.Observed,
		Source:     "test",
		At:         now,
		Entity:     id,
		Attributes: attrs,
	})
	entity, _ := model.Entity(id)
	return entity
}

// buildNetwork creates a test network resource (Service, Ingress, etc).
func buildNetwork(kind inventory.Kind, name, namespace string,
	now time.Time, attrs map[string]inventory.Value,
) inventory.Entity {
	model := inventory.NewModel(inventory.Options{})
	id := inventory.EntityID{Kind: kind, Namespace: namespace, Name: name}
	if attrs == nil {
		attrs = make(map[string]inventory.Value)
	}
	model.Apply(inventory.Observation{
		Kind:       inventory.Observed,
		Source:     "test",
		At:         now,
		Entity:     id,
		Attributes: attrs,
	})
	entity, _ := model.Entity(id)
	return entity
}

// buildStorage creates a test storage entity (PVC, StorageClass, etc).
func buildStorage(kind inventory.Kind, name, namespace string,
	now time.Time, attrs map[string]inventory.Value,
) inventory.Entity {
	model := inventory.NewModel(inventory.Options{})
	id := inventory.EntityID{Kind: kind, Namespace: namespace, Name: name}
	if attrs == nil {
		attrs = make(map[string]inventory.Value)
	}
	model.Apply(inventory.Observation{
		Kind:       inventory.Observed,
		Source:     "test",
		At:         now,
		Entity:     id,
		Attributes: attrs,
	})
	entity, _ := model.Entity(id)
	return entity
}

// newTestModel creates a new model for testing.
func newTestModel() *inventory.Model {
	return inventory.NewModel(inventory.Options{})
}

// setCondition sets a condition on the model for an entity.
func setCondition(model *inventory.Model, entityID inventory.EntityID,
	condType, status, reason, message string, since time.Time,
) {
	attrs := make(map[string]inventory.Value)
	key := kube.ConditionKey(condType)
	attrs[key] = inventory.Text(status)
	if reason != "" {
		attrs[key+kube.AttrConditionReason] = inventory.Text(reason)
	}
	if message != "" && status != "True" {
		attrs[key+kube.AttrConditionMessage] = inventory.Text(message)
	}
	if !since.IsZero() {
		attrs[key+kube.AttrConditionSince] = inventory.Time(since)
	}

	// Entities read from the model are detached copies, so the condition
	// is applied as a new observation that keeps the existing attributes.
	entity, _ := model.Entity(entityID)
	for k, attribute := range entity.Attributes {
		if _, set := attrs[k]; !set {
			attrs[k] = attribute.Value
		}
	}
	model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: "test", At: since,
		Entity: entityID, Attributes: attrs,
	})
}

// testDetectorContext creates a context for detector testing.
func testDetectorContext(model inventory.Reader, now time.Time,
) detection.Context {
	return detection.Context{Model: model, Now: now}
}
