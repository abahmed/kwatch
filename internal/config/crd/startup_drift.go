package crd

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	specSignaturePrefix = "spec:"
	// manyObjects is the signature of a namespace that holds more than one
	// KwatchConfig, which startup refuses.
	manyObjects = "many"
)

// StartupSignature identifies what the startup overlay read: spec is the
// spec (as JSON) of the KwatchConfig it found, applied or refused, and nil
// when there was none. refused without a spec means it found more than one
// object. The watcher compares it with its first synced list (see
// ExpectStartup).
func StartupSignature(spec []byte, refused bool) string {
	if len(spec) > 0 {
		sum := sha256.Sum256(spec)
		return specSignaturePrefix + hex.EncodeToString(sum[:])
	}
	if refused {
		return manyObjects
	}
	return ""
}

// ExpectStartup tells the watcher what the startup overlay read. The
// watcher starts later than the overlay is applied (after the Lease is
// acquired, which a standby may wait for indefinitely), so a KwatchConfig
// created, edited or deleted in between would otherwise be taken as
// already applied. Call it before Start.
func (w *Watcher) ExpectStartup(signature string) {
	w.mu.Lock()
	w.startupKnown = true
	w.startupSignature = signature
	w.mu.Unlock()
}

// listSignature is the StartupSignature of a synced list. ok is false when
// the list cannot be compared.
func listSignature(objects []interface{}) (signature string, ok bool) {
	switch len(objects) {
	case 0:
		return "", true
	case 1:
	default:
		return manyObjects, true
	}
	u, isObject := objects[0].(*unstructured.Unstructured)
	if !isObject {
		return "", false
	}
	// An object without a spec configures nothing, the same as no object.
	if _, found := u.Object["spec"]; !found {
		return "", true
	}
	signature = configFingerprint(u)
	return signature, strings.HasPrefix(signature, specSignaturePrefix)
}

// restartForStartupDrift restarts kwatch when the first synced list is not
// what the startup overlay read: an object appeared, changed or was
// deleted before the watcher started. A changed object is validated like
// a live edit, so an invalid one keeps the running configuration.
func (w *Watcher) restartForStartupDrift(objects []interface{}) {
	w.mu.Lock()
	known, expected := w.startupKnown, w.startupSignature
	w.mu.Unlock()
	if !known || w.restart == nil {
		return
	}
	current, ok := listSignature(objects)
	if !ok || current == expected {
		return
	}
	for _, obj := range objects {
		if err := w.validateOverlay(obj, len(objects)); err != nil {
			w.rejectOverlay(obj, err)
			return
		}
	}
	w.requestRestart("KwatchConfig changed while kwatch was starting; " +
		"restarting to apply configuration")
}
