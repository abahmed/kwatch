package kube

import (
	"encoding/json"
	"slices"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// AttrFinalizerManagers says which field manager owns each finalizer
// that holds a deletion: one "finalizer=manager" line per owner, lines
// joined with a newline. Only an object being deleted has it. A manager
// that put a finalizer on an object is the one that removes it, so this
// is the strongest proof of who handles a finalizer. Read it with
// FinalizerManagers.
const AttrFinalizerManagers = "finalizer.managers"

// annotationFinalizerManagers carries AttrFinalizerManagers from the
// informer transform, which still sees the field sets of the managed
// fields, to the translator, which no longer does. It only ever exists
// in kwatch's cache: the transform always replaces whatever the cluster
// object held under this key.
const annotationFinalizerManagers = "kwatch.abahmed.dev/finalizer-managers"

// Bounds on what is kept per object.
const (
	maxFinalizerManagers   = 16
	maxFinalizerManagerLen = 128
)

// stampFinalizerManagers records, on an object that is being deleted,
// the owner of each finalizer that holds it. It runs in the informer
// transform before the managed fields are trimmed.
func stampFinalizerManagers(obj metav1.Object) {
	annotations := obj.GetAnnotations()
	_, spoofed := annotations[annotationFinalizerManagers]
	owners := finalizerOwnerLines(obj)
	if !spoofed && len(owners) == 0 {
		return
	}
	kept := make(map[string]string, len(annotations)+1)
	for key, value := range annotations {
		if key != annotationFinalizerManagers {
			kept[key] = value
		}
	}
	if len(owners) > 0 {
		kept[annotationFinalizerManagers] = strings.Join(owners, "\n")
	}
	obj.SetAnnotations(kept)
}

// setFinalizerManagers adds AttrFinalizerManagers to the attributes of an
// object that is being deleted. A cached object carries the owners in
// its stamp; an object read straight from the cluster still has the
// field sets.
func setFinalizerManagers(
	attrs map[string]inventory.Value, obj metav1.Object,
) {
	if obj.GetDeletionTimestamp() == nil {
		return
	}
	lines := finalizerOwnerLines(obj)
	if len(lines) == 0 {
		stamp := obj.GetAnnotations()[annotationFinalizerManagers]
		if stamp != "" {
			lines = strings.Split(stamp, "\n")
		}
	}
	if len(lines) > 0 {
		attrs[AttrFinalizerManagers] = inventory.Text(
			strings.Join(lines, "\n"))
	}
}

// FinalizerManagers reads AttrFinalizerManagers: the managers that own
// each finalizer of the entity, by finalizer name.
func FinalizerManagers(e inventory.Entity) map[string][]string {
	value, ok := e.Attribute(AttrFinalizerManagers)
	if !ok {
		return nil
	}
	out := map[string][]string{}
	for _, line := range strings.Split(value.Value.AsText(), "\n") {
		finalizer, manager, found := strings.Cut(line, "=")
		if found && finalizer != "" && manager != "" {
			out[finalizer] = append(out[finalizer], manager)
		}
	}
	return out
}

// finalizerOwnerLines lists "finalizer=manager" for every finalizer that
// holds the deletion of obj and whose owner its managed fields name.
func finalizerOwnerLines(obj metav1.Object) []string {
	if obj.GetDeletionTimestamp() == nil || len(obj.GetFinalizers()) == 0 {
		return nil
	}
	holding := map[string]bool{}
	for _, name := range obj.GetFinalizers() {
		holding[name] = true
	}
	var lines []string
	for _, entry := range obj.GetManagedFields() {
		for _, name := range ownedFinalizers(entry) {
			line := name + "=" + clipManager(entry.Manager)
			if holding[name] && entry.Manager != "" &&
				!slices.Contains(lines, line) {
				lines = append(lines, line)
			}
		}
	}
	sort.Strings(lines)
	if len(lines) > maxFinalizerManagers {
		lines = lines[:maxFinalizerManagers]
	}
	return lines
}

func clipManager(manager string) string {
	manager = strings.ReplaceAll(strings.TrimSpace(manager), "\n", " ")
	if len(manager) > maxFinalizerManagerLen {
		return manager[:maxFinalizerManagerLen]
	}
	return manager
}

// ownedFinalizers are the finalizers a managed-fields entry owns:
// metadata.finalizers is a set, and each member is the key
// `v:"<finalizer>"` in the entry's field set.
func ownedFinalizers(entry metav1.ManagedFieldsEntry) []string {
	if entry.FieldsV1 == nil || len(entry.FieldsV1.Raw) == 0 {
		return nil
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(entry.FieldsV1.Raw, &fields) != nil {
		return nil
	}
	var meta map[string]json.RawMessage
	if json.Unmarshal(fields["f:metadata"], &meta) != nil {
		return nil
	}
	var set map[string]json.RawMessage
	if json.Unmarshal(meta["f:finalizers"], &set) != nil {
		return nil
	}
	var out []string
	for key := range set {
		var name string
		if rest, ok := strings.CutPrefix(key, "v:"); ok &&
			json.Unmarshal([]byte(rest), &name) == nil && name != "" {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}
