package detect

import (
	"time"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

// buildPod creates a test pod entity with the given attributes applied.
func buildPod(name, namespace string, now time.Time,
	attrs map[string]knowledge.Value) knowledge.Entity {
	model := knowledge.NewModel(knowledge.Options{})
	id := knowledge.EntityID{Kind: kube.KindPod, Namespace: namespace,
		Name: name}
	if attrs == nil {
		attrs = make(map[string]knowledge.Value)
	}
	model.Apply(knowledge.Fact{
		Kind:       knowledge.Observed,
		Source:     "test",
		At:         now,
		Entity:     id,
		Attributes: attrs,
	})
	entity, _ := model.Entity(id)
	return entity
}

// buildContainer creates a test container entity.
func buildContainer(pod knowledge.EntityID, name string,
	now time.Time, attrs map[string]knowledge.Value,
) knowledge.Entity {
	model := knowledge.NewModel(knowledge.Options{})
	id := knowledge.EntityID{Kind: kube.KindContainer, Namespace: pod.Namespace,
		Name: pod.Name + "/" + name}
	if attrs == nil {
		attrs = make(map[string]knowledge.Value)
	}
	model.Apply(knowledge.Fact{
		Kind:       knowledge.Observed,
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
	attrs map[string]knowledge.Value) knowledge.Entity {
	model := knowledge.NewModel(knowledge.Options{})
	id := knowledge.EntityID{Kind: kube.KindNode, Name: name}
	if attrs == nil {
		attrs = make(map[string]knowledge.Value)
	}
	model.Apply(knowledge.Fact{
		Kind:       knowledge.Observed,
		Source:     "test",
		At:         now,
		Entity:     id,
		Attributes: attrs,
	})
	entity, _ := model.Entity(id)
	return entity
}

// buildWorkload creates a test workload entity (Deployment, StatefulSet, etc).
func buildWorkload(kind knowledge.Kind, name, namespace string,
	now time.Time, attrs map[string]knowledge.Value,
) knowledge.Entity {
	model := knowledge.NewModel(knowledge.Options{})
	id := knowledge.EntityID{Kind: kind, Namespace: namespace, Name: name}
	if attrs == nil {
		attrs = make(map[string]knowledge.Value)
	}
	model.Apply(knowledge.Fact{
		Kind:       knowledge.Observed,
		Source:     "test",
		At:         now,
		Entity:     id,
		Attributes: attrs,
	})
	entity, _ := model.Entity(id)
	return entity
}

// buildNetwork creates a test network resource (Service, Ingress, etc).
func buildNetwork(kind knowledge.Kind, name, namespace string,
	now time.Time, attrs map[string]knowledge.Value,
) knowledge.Entity {
	model := knowledge.NewModel(knowledge.Options{})
	id := knowledge.EntityID{Kind: kind, Namespace: namespace, Name: name}
	if attrs == nil {
		attrs = make(map[string]knowledge.Value)
	}
	model.Apply(knowledge.Fact{
		Kind:       knowledge.Observed,
		Source:     "test",
		At:         now,
		Entity:     id,
		Attributes: attrs,
	})
	entity, _ := model.Entity(id)
	return entity
}

// buildStorage creates a test storage entity (PVC, StorageClass, etc).
func buildStorage(kind knowledge.Kind, name, namespace string,
	now time.Time, attrs map[string]knowledge.Value,
) knowledge.Entity {
	model := knowledge.NewModel(knowledge.Options{})
	id := knowledge.EntityID{Kind: kind, Namespace: namespace, Name: name}
	if attrs == nil {
		attrs = make(map[string]knowledge.Value)
	}
	model.Apply(knowledge.Fact{
		Kind:       knowledge.Observed,
		Source:     "test",
		At:         now,
		Entity:     id,
		Attributes: attrs,
	})
	entity, _ := model.Entity(id)
	return entity
}

// newTestModel creates a new model for testing.
func newTestModel() *knowledge.Model {
	return knowledge.NewModel(knowledge.Options{})
}

// setCondition sets a condition on the model for an entity.
func setCondition(model *knowledge.Model, entityID knowledge.EntityID,
	condType, status, reason, message string, since time.Time,
) {
	attrs := make(map[string]knowledge.Value)
	key := kube.ConditionKey(condType)
	attrs[key] = knowledge.Text(status)
	if reason != "" {
		attrs[key+kube.AttrConditionReason] = knowledge.Text(reason)
	}
	if message != "" && status != "True" {
		attrs[key+kube.AttrConditionMessage] = knowledge.Text(message)
	}
	if !since.IsZero() {
		attrs[key+kube.AttrConditionSince] = knowledge.Time(since)
	}

	// Entities read from the model are detached copies, so the condition
	// is applied as a new observation that keeps the existing attributes.
	entity, _ := model.Entity(entityID)
	for k, attribute := range entity.Attributes {
		if _, set := attrs[k]; !set {
			attrs[k] = attribute.Value
		}
	}
	model.Apply(knowledge.Fact{
		Kind: knowledge.Observed, Source: "test", At: since,
		Entity: entityID, Attributes: attrs,
	})
}

// testDetectorContext creates a context for detector testing.
func testDetectorContext(model knowledge.Reader, now time.Time,
) signal.Context {
	return signal.Context{Model: model, Now: now}
}
