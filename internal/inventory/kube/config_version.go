package kube

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// ConfigAge says which content of a changed ConfigMap or Secret a pod's
// containers read when they started.
type ConfigAge int

// Config ages.
const (
	// ConfigUnknown: the pod's start times do not say.
	ConfigUnknown ConfigAge = iota
	// ConfigBefore: every container started before the change, so the
	// pod still runs the old content.
	ConfigBefore
	// ConfigAfter: the pod was created after the change, so it started
	// with the new content.
	ConfigAfter
)

// ConfigChangedAt is when a ConfigMap or Secret last changed content, as
// kwatch saw it: the moment its value digest took its current value.
// It reports false for an object whose content never changed while
// kwatch watched: the first digest is not a change. The digest holds no
// value, so this never touches Secret data.
func ConfigChangedAt(e inventory.Entity) (time.Time, bool) {
	digest, ok := e.Attribute(AttrDataDigest)
	if !ok || !digest.Since.After(e.FirstSeen) {
		return time.Time{}, false
	}
	return digest.Since, true
}

// PodFreezes reports whether the pod reads the object only when a
// container starts (see AttrFrozenConfig).
func PodFreezes(pod inventory.Entity, config inventory.EntityID) bool {
	attr, ok := pod.Attribute(AttrFrozenConfig)
	if !ok {
		return false
	}
	want := string(config.Kind) + "/" + config.Name
	for _, ref := range strings.Split(attr.Value.AsText(), ",") {
		if ref == want {
			return true
		}
	}
	return false
}

// PodConfigAge compares the pod's start with a change of its config. A
// pod whose containers restarted since the change may have read either
// content, so it is unknown rather than guessed.
func PodConfigAge(pod inventory.Entity, changed time.Time) ConfigAge {
	// Pod times have second precision.
	changed = changed.Truncate(time.Second)
	created, ok := timeAttr(pod, AttrCreated)
	if ok && !created.Before(changed) {
		return ConfigAfter
	}
	started, found := timeAttr(pod, AttrContainersStarted)
	if ok && found && started.Before(changed) {
		return ConfigBefore
	}
	return ConfigUnknown
}

func timeAttr(e inventory.Entity, name string) (time.Time, bool) {
	attr, ok := e.Attribute(name)
	if !ok {
		return time.Time{}, false
	}
	at := attr.Value.AsTime()
	return at, !at.IsZero()
}
