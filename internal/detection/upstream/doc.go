// Package upstream is a checked-in snapshot of the reasons, phases and
// condition types Kubernetes itself emits. In: nothing. Out: lists read
// by the coverage tests of detection and detectors, which fail when a
// reason in the snapshot is neither classified by kwatch nor listed as
// deliberately ignored.
//
// Source and version. k8s.io/kubernetes is not a dependency, so the
// lists are written out by hand from the upstream source files named in
// each list's comment, as of Kubernetes 1.37 (the release that matches
// k8s.io/api v0.37.1 in go.mod). The typed constants that k8s.io/api
// does export (pod phases, pod and node condition types, scheduler and
// Deployment, Job, HPA and PVC condition constants) are cross-checked
// against the module by this package's own test, so a rename there
// fails CI. Reasons that exist only in kubelet, scheduler and
// controller-manager source cannot be checked that way: when a new
// Kubernetes release adds one, add it to the list and let the coverage
// test say what kwatch needs.
package upstream
