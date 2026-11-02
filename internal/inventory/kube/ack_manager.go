package kube

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// AttrAckBy is the field manager that wrote the ack annotation, such as
// "kubectl-annotate", read from the object's managed fields. It exists
// only next to AttrAck, and only when the managed fields name the
// writer. It is an attribute, not a change: an edit of the annotation
// is not a change to the object, and the change history feeds the
// root-cause reasoning, fix attempts and resolves.
const AttrAckBy = "ack.by"

// annotationAckManager carries AttrAckBy from the informer transform,
// which still sees the field sets of the managed fields, to the
// translator, which no longer does. It only ever exists in kwatch's
// cache: the transform always replaces whatever the cluster object held
// under this key.
const annotationAckManager = "kwatch.abahmed.dev/ack-manager"

// ackField is the key of the ack annotation in a managed-fields set.
const ackField = "f:" + AckAnnotation

// stampAckManager records, on an object that carries the ack annotation,
// the manager that wrote it. It runs in the informer transform before
// the managed fields are trimmed.
func stampAckManager(obj metav1.Object) {
	annotations := obj.GetAnnotations()
	_, spoofed := annotations[annotationAckManager]
	manager := ""
	if _, acked := annotations[AckAnnotation]; acked {
		manager = ackOwner(obj.GetManagedFields())
	}
	if !spoofed && manager == "" {
		return
	}
	kept := make(map[string]string, len(annotations)+1)
	for key, value := range annotations {
		if key != annotationAckManager {
			kept[key] = value
		}
	}
	if manager != "" {
		kept[annotationAckManager] = manager
	}
	obj.SetAnnotations(kept)
}

// ackManager is the writer of obj's ack annotation: from the field sets
// when they are still there, else from the transform's stamp.
func ackManager(obj metav1.Object) string {
	if manager := ackOwner(obj.GetManagedFields()); manager != "" {
		return manager
	}
	return clipManager(obj.GetAnnotations()[annotationAckManager])
}

// ackOwner is the manager whose field set owns the ack annotation, the
// latest one when several do (an apply and an annotate), or "".
func ackOwner(entries []metav1.ManagedFieldsEntry) string {
	owner := -1
	for i, entry := range entries {
		if !ownsAck(entry) || entry.Manager == "" {
			continue
		}
		if owner < 0 || writtenAfter(entry, entries[owner]) {
			owner = i
		}
	}
	if owner < 0 {
		return ""
	}
	return clipManager(entries[owner].Manager)
}

// writtenAfter reports whether a was written after b; an entry without
// a time never was.
func writtenAfter(a, b metav1.ManagedFieldsEntry) bool {
	if a.Time == nil {
		return false
	}
	return b.Time == nil || !a.Time.Before(b.Time)
}

// ownsAck reports a field set that holds the ack annotation:
// metadata.annotations is a map, keyed "f:<annotation>".
func ownsAck(entry metav1.ManagedFieldsEntry) bool {
	if entry.FieldsV1 == nil || len(entry.FieldsV1.Raw) == 0 {
		return false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(entry.FieldsV1.Raw, &fields) != nil {
		return false
	}
	var meta map[string]json.RawMessage
	if json.Unmarshal(fields["f:metadata"], &meta) != nil {
		return false
	}
	var annotations map[string]json.RawMessage
	if json.Unmarshal(meta["f:annotations"], &annotations) != nil {
		return false
	}
	_, ok := annotations[ackField]
	return ok
}

// setAckBy adds AttrAckBy to desc when the writer of the ack annotation
// is known.
func setAckBy(obj metav1.Object, desc *Description) {
	if manager := ackManager(obj); manager != "" {
		setAttribute(desc, AttrAckBy, inventory.Text(manager))
	}
}
