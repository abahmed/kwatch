package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

var podNodeNow = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

// observeEntity records attributes of id at the given time.
func observeEntity(
	model *inventory.Model, id inventory.EntityID, at time.Time,
	attrs map[string]inventory.Value,
) {
	model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: "test", At: at, Entity: id,
		Attributes: attrs,
	})
}

// relateEntity declares one relation from id.
func relateEntity(
	model *inventory.Model, id inventory.EntityID,
	relation inventory.RelationType, targets ...inventory.EntityID,
) {
	model.Apply(inventory.Observation{
		Kind: inventory.Related, Source: "test", At: podNodeNow,
		Entity: id, Relation: relation, Targets: targets,
	})
}

// noteEntity records a Warning event note on id.
func noteEntity(
	model *inventory.Model, id inventory.EntityID, reason, message string,
	count int, at time.Time,
) {
	model.Apply(inventory.Observation{
		Kind: inventory.Noted, Source: "test", At: at, Entity: id,
		Note: inventory.Note{
			At: at, Source: "kubelet", Reason: reason, Message: message,
			Count: count, Warning: true,
		},
	})
}

// conditionAttrs returns the attributes of one status condition.
func conditionAttrs(
	attrs map[string]inventory.Value, conditionType, status, reason string,
	since time.Time,
) map[string]inventory.Value {
	if attrs == nil {
		attrs = make(map[string]inventory.Value)
	}
	key := kube.ConditionKey(conditionType)
	attrs[key] = inventory.Text(status)
	attrs[key+kube.AttrConditionReason] = inventory.Text(reason)
	attrs[key+kube.AttrConditionSince] = inventory.Time(since)
	return attrs
}

func entityOf(
	model *inventory.Model, id inventory.EntityID,
) inventory.Entity {
	entity, _ := model.Entity(id)
	return entity
}

// classified runs the registry's completion on detector output.
func classified(found []detection.Finding) []detection.Finding {
	for i := range found {
		found[i] = detection.Classify(found[i])
	}
	return found
}

func findingReasons(found []detection.Finding) []string {
	out := make([]string, 0, len(found))
	for _, f := range found {
		out = append(out, f.Reason)
	}
	return out
}
