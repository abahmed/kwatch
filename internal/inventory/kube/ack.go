package kube

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// AckAnnotation is the annotation a person puts on an object to say they
// are looking at its incident: kubectl annotate deploy/api
// kwatch.io/ack="looking into it". It has the same prefix as the
// maintenance annotations, and it is read-only for kwatch: kwatch never
// writes to the cluster.
const AckAnnotation = "kwatch.io/ack"

// Attributes recording who looks at an object.
const (
	// AttrAck exists on an object that carries the ack annotation; its
	// text is the annotation value, a note from a person.
	AttrAck = "ack"
	// AttrOwner is the owner of an object: the value of the configured
	// owner label or annotation.
	AttrOwner = "owner"
)

// OwnerKey is the label or annotation that names an object's owner, such
// as a team: kwatch.io/owner=payments. Provider routes match on it.
const OwnerKey = "kwatch.io/owner"

// maxAckNote bounds the text of an acknowledgement kept in the model.
const maxAckNote = 200

// maxOwnerValue is the longest value a label can have.
const maxOwnerValue = 63

// annotateAck records the ack annotation as an attribute. The note is a
// person's text: it loses credentials, line breaks and everything after
// maxAckNote bytes.
func annotateAck(obj metav1.Object, desc *Description) {
	note, ok := obj.GetAnnotations()[AckAnnotation]
	if !ok {
		return
	}
	setAttribute(desc, AttrAck, inventory.Text(ackNote(note)))
}

// ackNote makes an annotation value safe to quote in a message.
func ackNote(value string) string {
	text := strings.Join(strings.Fields(value), " ")
	return boundedLabel(text, maxAckNote)
}

// annotateOwner records the owner label, or else the owner annotation.
func annotateOwner(obj metav1.Object, desc *Description) {
	value, ok := obj.GetLabels()[OwnerKey]
	if !ok {
		value, ok = obj.GetAnnotations()[OwnerKey]
	}
	value = strings.TrimSpace(value)
	if !ok || value == "" {
		return
	}
	setAttribute(desc, AttrOwner,
		inventory.Text(boundedLabel(value, maxOwnerValue)))
}

func setAttribute(desc *Description, name string, value inventory.Value) {
	if desc.Attributes == nil {
		desc.Attributes = make(map[string]inventory.Value)
	}
	desc.Attributes[name] = value
}
