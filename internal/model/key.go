package model

// IncidentKey is the stable dedup key identifying an incident.
//
// Regular form: "<namespace>:<owner>:<reason>:<container>"
//   - namespace: "" for cluster-scoped resources (nodes, webhooks, ...).
//   - owner: the owning resource name. For pod symptoms this is the bare
//     owner name (or the pod's own name when the pod is ownerless);
//     workload-object detectors may carry the fully-qualified "ns/name"
//     form. See Incident.Name.
//   - reason: the normalized Kubernetes reason; trailing space-separated
//     numeric suffixes are stripped (e.g. "BackOff 5s" -> "BackOff").
//   - container: currently unused (always ""); reserved for granularity.
//
// Two additional forms exist:
//   - Global: "<reason>|global|<scope>" for cross-namespace dedup (e.g.
//     ImagePullBackOff rate-limit/timeout/DNS issues) where the underlying
//     problem is cluster-scoped and must map to one incident.
//   - Group: "__group__:<group key>" for incidents synthesized by smart
//     grouping (one notification summarizing many members).
//
// Keys are built and parsed in the correlation package (BuildKey, ParseKey).
type IncidentKey string
