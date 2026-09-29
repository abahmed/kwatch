package core

import (
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
)

// snapshotTimeKey stores when the fingerprints were written. Changes found
// at startup happened after it, so they are dated there: that keeps them
// before the failures they may have caused.
const snapshotTimeKey = "~snapshot.time"

// DowntimeActor attributes changes that happened while kwatch was not
// running; the real actor is no longer knowable.
const DowntimeActor = "unknown (changed while kwatch was down)"

// fingerprintAttributes are, per kind, the attributes whose change is a
// meaningful change. They mirror what the schemas diff.
var fingerprintAttributes = map[knowledge.Kind][]string{
	kube.KindDeployment:    {kube.AttrTemplateHash, kube.AttrReplicas},
	kube.KindStatefulSet:   {kube.AttrTemplateHash, kube.AttrReplicas},
	kube.KindDaemonSet:     {kube.AttrTemplateHash},
	kube.KindCronJob:       {kube.AttrTemplateHash},
	kube.KindSecret:        {kube.AttrDataDigest},
	kube.KindConfigMap:     {kube.AttrDataDigest},
	kube.KindService:       {kube.AttrSelector, kube.AttrPorts},
	kube.KindNetworkPolicy: {kube.AttrPolicyDigest},
	kube.KindNode:          {kube.AttrKubelet, kube.AttrTaints},
}

// Fingerprints summarises the tracked objects of the model, keyed by
// entity, for the state file.
func Fingerprints(
	model knowledge.Reader, now time.Time,
) map[string]any {
	out := map[string]any{snapshotTimeKey: now.UTC().Format(time.RFC3339)}
	for kind, attrs := range fingerprintAttributes {
		for _, id := range model.Entities(kind) {
			entity, ok := model.Entity(id)
			if !ok {
				continue
			}
			out[id.String()] = fingerprint(entity, attrs)
		}
	}
	return out
}

func fingerprint(entity knowledge.Entity, attrs []string) string {
	parts := make([]string, 0, len(attrs))
	for _, name := range attrs {
		value := ""
		if attribute, ok := entity.Attribute(name); ok {
			value = attribute.Value.AsText()
		}
		parts = append(parts, name+"="+value)
	}
	return strings.Join(parts, ";")
}

// DowntimeChanges compares saved fingerprints with the current model and
// returns a Changed fact for every tracked object that changed while
// kwatch was down, in deterministic order.
func DowntimeChanges(
	model knowledge.Reader, saved map[string]string, now time.Time,
) []knowledge.Fact {
	current := Fingerprints(model, now)
	delete(current, snapshotTimeKey)
	at := now
	if saved, err := time.Parse(time.RFC3339,
		saved[snapshotTimeKey]); err == nil {
		at = saved
	}
	keys := make([]string, 0, len(current))
	for key := range current {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var facts []knowledge.Fact
	for _, key := range keys {
		before, known := saved[key]
		after, _ := current[key].(string)
		if !known || before == after {
			continue
		}
		id, ok := knowledge.ParseEntityID(key)
		if !ok {
			continue
		}
		facts = append(facts, knowledge.Fact{
			Kind: knowledge.Changed, Source: kube.FactSource, At: now,
			Entity: id,
			Change: knowledge.Change{
				At:     at,
				Actor:  DowntimeActor,
				Fields: fingerprintFields(before, after),
			},
		})
	}
	return facts
}

// fingerprintFields lists the attributes that differ between two
// fingerprints.
func fingerprintFields(before, after string) []knowledge.FieldChange {
	old := parseFingerprint(before)
	var fields []knowledge.FieldChange
	for _, part := range strings.Split(after, ";") {
		name, value, _ := strings.Cut(part, "=")
		if old[name] != value {
			fields = append(fields, knowledge.FieldChange{
				Path: downtimePath(name), Before: old[name], After: value,
			})
		}
	}
	return fields
}

// downtimePath maps a template hash to the path rollout rules recognise.
func downtimePath(attribute string) string {
	if attribute == kube.AttrTemplateHash {
		return "spec.template"
	}
	return attribute
}

func parseFingerprint(value string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(value, ";") {
		name, v, _ := strings.Cut(part, "=")
		out[name] = v
	}
	return out
}
