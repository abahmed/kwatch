package model

// An Observation is one look at one Kubernetes object: what kwatch was
// looking at, and what it found.
//
// It is the only thing a detector or a monitor builds, and the correlation
// engine's only input. Producers used to assemble the pipeline's event
// themselves -- in eight packages, from a struct that flattened subject,
// owner, evidence and finding into twenty-three sibling fields -- and each
// copy forgot something different: the labels silence rules match on, the pod
// UID replacement detection needs, the object's own name. Here the identity
// is typed and filled in by the observe package from the object itself, so
// there is nothing left for a producer to forget.
//
// A nil *Observation means "nothing found": detectors return one so the
// caller cannot mistake an empty finding for a healthy object.
type Observation struct {
	// Subject is the object this is about. Its Kind is kwatch's resource
	// word -- lower-case and singular ("pod", "deployment", "pvc") -- which
	// is what incident keys, silence rules and the config vocabulary use.
	Subject ObjectRef
	// Owner is the workload the subject's incidents are keyed by: a pod's
	// Deployment, or the subject itself for everything that owns itself.
	//
	// Its Kind is the Kubernetes Kind spelling ("Deployment", "StatefulSet"),
	// not the resource word, because that is what an operator writes in
	// severityByOwnerKind and what the alert prints. The two vocabularies
	// already existed; this type is where they meet. An owner with a name but
	// no Kind is deliberate: it owns the incident without being a workload
	// anyone would want named in the alert.
	Owner ObjectRef
	// Pod is the concrete pod instance's identity, for pod subjects. The
	// engine reads it to tell a replacement pod from the original.
	Pod PodIdentity
	// NodeName is where the subject runs, when that is known.
	NodeName string
	// Container narrows the finding to one container of a pod. "." means the
	// pod itself rather than any container in it.
	Container string
	Image     string
	// RestartCount is the container's restart count as observed.
	RestartCount int32

	// Reason is the machine-readable finding, from internal/constant.
	Reason string
	// Message is what Kubernetes said, verbatim.
	Message string
	// Hint is kwatch's explanation. When empty, Message stands in.
	Hint string
	// Facts are the structured details behind Hint; renderers read these
	// rather than parsing the prose.
	Facts Facts
	// Severity overrides what the enricher would infer from the owner kind.
	Severity Severity
	// Labels are the subject's labels, which silence rules match on.
	Labels map[string]string

	Logs   string
	Events string
	// IncludeEvents and IncludeLogs are the producer's statement about
	// whether the evidence above may be shown. Only the pod pipeline
	// collects logs and events, and only its configuration says whether an
	// operator wants them in the alert.
	IncludeEvents bool
	IncludeLogs   bool
	// ContainerState is the pre-computed container status, when the producer
	// already has one. Otherwise the engine derives what it can from
	// RestartCount.
	ContainerState *ContainerState

	// Transient marks a finding kwatch can only ever learn from a
	// point-in-time Kubernetes Event -- a FailedMount, a FailedScheduling, an
	// autoscaler that could not add nodes. Nothing about the object's state
	// says the event will not recur, so the engine must not hold such an
	// incident open merely because the object still exists.
	Transient bool
}

// PodIdentity is what distinguishes one pod instance from its replacement.
type PodIdentity struct {
	// UID is the concrete pod's UID.
	UID string
	// LineageID is an explicit stable lineage for pods that have no owner,
	// carried as an annotation.
	LineageID string
	// GenerateName is evidence only. It is never an authoritative identity:
	// two unrelated pods of the same Deployment share it.
	GenerateName string
}

// OwnerPath is the owner encoding incident keys are built from.
//
// The encoding is not uniform, and cannot be changed without invalidating
// every persisted incident and baseline entry: a pod incident is keyed by the
// bare name of its owning workload, a namespaced object by "namespace/name",
// and a cluster-scoped one by its name alone. Keeping the rule here means
// producers no longer each spell it out.
func (o *Observation) OwnerPath() string {
	if o == nil || o.Owner.Name == "" {
		return ""
	}
	if o.Owner.Namespace == "" || o.Subject.Kind == "pod" {
		return o.Owner.Name
	}
	return o.Owner.Namespace + "/" + o.Owner.Name
}

// OwnerKind is the Kubernetes Kind of the owning workload, or "" when the
// subject owns itself. The engine reads the empty case as "this pod has no
// workload", which is what lets a generated replacement pod fold into the
// incident its predecessor opened.
func (o *Observation) OwnerKind() string {
	if o == nil || o.ownsItself() {
		return ""
	}
	return o.Owner.Kind
}

// ownsItself reports whether the owner is the subject. The namespace
// comparison tolerates an owner with no namespace, which is how a Namespace
// subject -- whose own namespace is itself -- refers to itself.
func (o *Observation) ownsItself() bool {
	if o.Owner.Name == "" || o.Owner.Name != o.Subject.Name {
		return o.Owner.Name == ""
	}
	return o.Owner.Namespace == o.Subject.Namespace ||
		o.Owner.Namespace == ""
}

// State is the container state the observation implies: the pre-computed one
// when the producer had it, otherwise just the restart count it saw.
func (o *Observation) State() *ContainerState {
	if o == nil {
		return nil
	}
	if o.ContainerState != nil {
		return o.ContainerState
	}
	if o.RestartCount > 0 {
		return &ContainerState{RestartCount: o.RestartCount}
	}
	return nil
}

// WithHint sets kwatch's explanation and returns the observation, so a
// detector reads as one expression.
func (o *Observation) WithHint(hint string) *Observation {
	o.Hint = hint
	return o
}

// WithSeverity overrides the severity the enricher would infer.
func (o *Observation) WithSeverity(severity Severity) *Observation {
	o.Severity = severity
	return o
}

// WithFacts attaches the structured details behind the hint.
func (o *Observation) WithFacts(facts Facts) *Observation {
	o.Facts = facts
	return o
}

// WithLabels attaches the labels silence rules match on, for subjects the
// producer did not build from a typed object.
func (o *Observation) WithLabels(labels map[string]string) *Observation {
	o.Labels = labels
	return o
}

// WithMessage records what Kubernetes said, verbatim.
func (o *Observation) WithMessage(message string) *Observation {
	o.Message = message
	return o
}

// WithEvidence attaches the material behind the finding.
func (o *Observation) WithEvidence(
	logs, events string, state *ContainerState,
) *Observation {
	o.Logs = logs
	o.Events = events
	o.ContainerState = state
	return o
}
