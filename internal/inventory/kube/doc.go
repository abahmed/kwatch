// Package kube turns Kubernetes objects into inventory Observations.
// In: informer notifications, kubelet stats and probe results. Out:
// Observations submitted to the pipeline. Each kind has a Schema that
// extracts identity, attributes and relations and diffs versions into
// meaningful changes.
package kube
