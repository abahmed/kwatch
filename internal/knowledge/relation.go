package knowledge

// RelationType names a directed relation. A relation points from the
// dependent entity to what it depends on, so failure propagates against
// the arrow: a pod "runs-on" a node, and a failing node affects the pod.
type RelationType string

// Relation types shared by all sources. New sources may add types; the
// reasoning engine treats every type generically through its rules.
const (
	OwnedBy     RelationType = "owned-by"
	RunsOn      RelationType = "runs-on"
	PartOf      RelationType = "part-of"
	Selects     RelationType = "selects"
	Backs       RelationType = "backs"
	RoutesTo    RelationType = "routes-to"
	References  RelationType = "references"
	Mounts      RelationType = "mounts"
	Scales      RelationType = "scales"
	Intercepts  RelationType = "intercepts"
	Serves      RelationType = "serves"
	Constrains  RelationType = "constrains"
	Authorizes  RelationType = "authorizes"
	ResolvesVia RelationType = "resolves-via"
	Pulls       RelationType = "pulls"
	Calls       RelationType = "calls"
	Schedules   RelationType = "schedules"
	Heartbeats  RelationType = "heartbeats"
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
