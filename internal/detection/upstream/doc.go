// Package upstream holds the reasons, phases and condition types
// Kubernetes itself emits. In: nothing. Out: lists read by the coverage
// tests of detection and detectors, which fail when a reason in the
// corpus is neither classified by kwatch nor listed as deliberately
// ignored.
//
// Generated and curated. states_generated.go and events_generated.go
// are written by cmd/upstreamreasons from the kubernetes/kubernetes
// source at the tag in Tag (the release that matches k8s.io/api in
// go.mod): the event reasons of the kubelet, scheduler and controller
// manager, the container waiting and terminated reasons, pod status and
// pod condition reasons, and the reasons of the HPA conditions. The
// generator resolves event calls with go/ast and reads the files named
// in cmd/upstreamreasons/sources.go, which says where each list comes
// from.
// make upstream-reasons rewrites the files, make upstream-check
// compares them with the tag (both need network); make verify only uses
// the committed files, and a test here fails when Tag and go.mod
// disagree.
//
// Curated by hand, because the source has no list of them: PodPhases,
// the runtime-reported terminated reasons (states.go) and the condition
// types with the status that means trouble (conditions.go). The typed
// constants k8s.io/api exports are cross-checked against the module by
// this package's own test.
package upstream
