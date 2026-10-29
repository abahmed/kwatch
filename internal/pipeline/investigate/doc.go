// Package investigate gathers the evidence of an incident for Pro. A
// registry of read-only checks (pod state, logs, events, usage, node,
// volume, Service and Ingress chains, rollout, control plane, config
// keys and more) runs in parallel under one budget; the root-kind
// investigators among them lead the bounded channel summary, every
// check adds a section to the dashboard report, and correlate.go
// concludes across the sections. It plans reads on the decision loop
// without I/O and runs them on the pipeline's pool. It never imports
// the pipeline package.
package investigate
