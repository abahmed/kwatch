package inventory

// RelationType names a directed relation. A relation points from the
// dependent entity to what it depends on, so failure propagates against
// the arrow: a pod "runs-on" a node, and a failing node affects the pod.
type RelationType string

// Relation types shared by all sources. New sources may add types; the
// reasoning engine treats every type generically through its rules.
const (
	// OwnedBy points from an object to the controller that owns it: a
	// pod is owned by its ReplicaSet.
	OwnedBy RelationType = "owned-by"
	// RunsOn points from a workload unit to where it runs: a pod runs
	// on a node.
	RunsOn RelationType = "runs-on"
	// PartOf points from a piece to the whole it belongs to: a
	// container is part of its pod, a node part of its node pool.
	PartOf RelationType = "part-of"
	// Selects points from an object to the pods its label selector
	// matches: a Service, PodDisruptionBudget or NetworkPolicy selects
	// pods.
	Selects RelationType = "selects"
	// Backs points from what provides endpoints to what it backs: an
	// EndpointSlice backs its Service.
	Backs RelationType = "backs"
	// RoutesTo points from a route or ingress to the backend it sends
	// traffic to.
	RoutesTo RelationType = "routes-to"
	// References points from an object to another object it uses by
	// name: a Secret, ConfigMap, ServiceAccount, StorageClass or any
	// object named through a *Ref field. It is the "uses" relation.
	References RelationType = "references"
	// Mounts points from a pod to a volume claim it mounts.
	Mounts RelationType = "mounts"
	// Scales points from an autoscaler to the workload it scales.
	Scales RelationType = "scales"
	// Intercepts points from something in the request path, such as an
	// admission webhook, to what it intercepts.
	Intercepts RelationType = "intercepts"
	// Serves points from an object to the Service that serves it: an
	// APIService, an admission webhook or a CRD conversion webhook is
	// served by a Service. It is the "served-by" relation.
	Serves RelationType = "serves"
	// Constrains points from a policy to what it limits, such as a
	// quota to its namespace.
	Constrains RelationType = "constrains"
	// Authorizes points from a binding to the subject it grants access.
	Authorizes RelationType = "authorizes"
	// ResolvesVia points from a client to the resolver it uses for
	// names, such as a pod to cluster DNS.
	ResolvesVia RelationType = "resolves-via"
	// Pulls points from a pod or container to the image it pulls.
	Pulls RelationType = "pulls"
	// Calls points from a caller to a dependency it calls at runtime.
	Calls RelationType = "calls"
	// Schedules points from a scheduler, such as a CronJob, to what it
	// starts.
	Schedules RelationType = "schedules"
	// Heartbeats points from a component to the lease or record it
	// renews to show it is alive.
	Heartbeats RelationType = "heartbeats"
	// ManagedBy points from an object to the controller workload that
	// last wrote its spec, inferred from the field manager name. It is
	// medium confidence: a manager name is a convention, not a
	// reference.
	ManagedBy RelationType = "managed-by"
)

// Relation is one directed edge.
type Relation struct {
	From EntityID
	Type RelationType
	To   EntityID
}

// Direction selects which side of a relation a lookup follows.
type Direction uint8

const (
	// Outgoing follows From → To: what an entity depends on.
	Outgoing Direction = iota
	// Incoming follows To → From: what depends on an entity.
	Incoming
)
