// Package kube translates Kubernetes objects into knowledge facts.
//
// Each supported kind has a Schema that describes an object (identity,
// attributes, relations) and diffs two versions of it into meaningful
// changes. The Translator turns informer notifications into facts: the
// initial list only observes objects, so a restart never reports the whole
// cluster as "created"; status-only updates refresh attributes without
// recording a change.
package kube
