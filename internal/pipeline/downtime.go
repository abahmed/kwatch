package pipeline

import (
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// snapshotTimeKey stores when the fingerprints were written. Changes found
// at startup happened after it, so they are dated there: that keeps them
// before the failures they may have caused.
const snapshotTimeKey = "~snapshot.time"

// kindTimePrefix keys the snapshot time of one kind's fingerprints, as
// in "~snapshot.time/Secret". Every snapshot writes it for each tracked
// kind, so a kind with the key was covered: an object of it without a
// saved fingerprint was created after the snapshot. A carried kind keeps
// the time its entries were taken at. The key has a single slash, so it
// never parses as an entity.
const kindTimePrefix = snapshotTimeKey + "/"

// DowntimeActor attributes changes that happened while kwatch was not
// running; the real actor is no longer knowable.
const DowntimeActor = "unknown (changed while kwatch was down)"

// fingerprintAttributes are, per kind, the attributes whose change is a
// meaningful change. They mirror what the schemas diff.
var fingerprintAttributes = map[inventory.Kind][]string{
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
// entity, for the state file, with the snapshot time and a time key per
// tracked kind.
func Fingerprints(
	model inventory.Reader, now time.Time,
) map[string]any {
	at := now.UTC().Format(time.RFC3339)
	out := map[string]any{snapshotTimeKey: at}
	for kind, attrs := range fingerprintAttributes {
		out[kindTimePrefix+string(kind)] = at
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

func fingerprint(entity inventory.Entity, attrs []string) string {
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
// returns a Changed observation for every tracked object that was
// edited, deleted or created while kwatch was down, in deterministic
// order. Only kinds synced reports as fully listed are compared; a nil
// synced treats every kind as synced. A creation is reported only for a
// kind the saved snapshot covered, so the first run reports none.
func DowntimeChanges(
	model inventory.Reader, saved map[string]string,
	synced func(inventory.Kind) bool, now time.Time,
) []inventory.Observation {
	current := Fingerprints(model, now)
	var observations []inventory.Observation
	for _, key := range fingerprintKeys(current, saved) {
		id, ok := inventory.ParseEntityID(key)
		if !ok || !kindSynced(synced, id.Kind) {
			continue
		}
		if _, tracked := fingerprintAttributes[id.Kind]; !tracked {
			continue
		}
		change, changed := downtimeChange(saved, current, key, id.Kind)
		if !changed {
			continue
		}
		change.At = changeTime(saved, id.Kind, now)
		change.Actor = DowntimeActor
		observations = append(observations, inventory.Observation{
			Kind: inventory.Changed, Source: kube.ObservationSource, At: now,
			Entity: id, Change: change,
		})
	}
	return observations
}

// downtimeChange classifies one tracked key: an edit when both
// fingerprints exist and differ, a deletion when only the saved one
// exists, and a creation when only the current one exists and the
// snapshot covered the kind. It reports false when nothing changed.
func downtimeChange(
	saved map[string]string, current map[string]any, key string,
	kind inventory.Kind,
) (inventory.Change, bool) {
	before, known := saved[key]
	after, present := current[key].(string)
	switch {
	case known && present && before != after:
		return inventory.Change{
			Fields: fingerprintFields(before, after)}, true
	case known && !present:
		return inventory.Change{Deleted: true}, true
	case !known && present && saved[kindTimePrefix+string(kind)] != "":
		return inventory.Change{Created: true}, true
	}
	return inventory.Change{}, false
}

// fingerprintKeys lists the keys of both fingerprint sets, sorted and
// each once.
func fingerprintKeys(
	current map[string]any, saved map[string]string,
) []string {
	keys := make([]string, 0, len(current)+len(saved))
	for key := range current {
		keys = append(keys, key)
	}
	for key := range saved {
		if _, ok := current[key]; !ok {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

// kindSynced reports whether synced treats kind as fully listed; a nil
// synced treats every kind so.
func kindSynced(synced func(inventory.Kind) bool, kind inventory.Kind) bool {
	return synced == nil || synced(kind)
}

// changeTime dates a downtime change of kind at the snapshot its saved
// fingerprint was taken at, or at now without one.
func changeTime(
	saved map[string]string, kind inventory.Kind, now time.Time,
) time.Time {
	at, err := time.Parse(time.RFC3339, snapshotOf(saved, kind))
	if err != nil {
		return now
	}
	return at
}

// fingerprintFields lists the attributes that differ between two
// fingerprints.
func fingerprintFields(before, after string) []inventory.FieldChange {
	old := parseFingerprint(before)
	var fields []inventory.FieldChange
	for _, part := range strings.Split(after, ";") {
		name, value, _ := strings.Cut(part, "=")
		if old[name] != value {
			fields = append(fields, inventory.FieldChange{
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
