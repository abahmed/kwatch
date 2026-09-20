// Package network contains Service, Ingress, and NetworkPolicy detection and
// its queue runtime.
//
// It evaluates cached Kubernetes objects and returns observations. Runtime
// owns lister access and sustain windows; incident reconciliation stays
// behind a narrow sink.
package network
