package inventory

import "strings"

// ChangeClass says what kind of modification a Change is. Classes are
// stable strings: they appear in timelines and persisted history.
type ChangeClass string

// Change classes, from ADR 0011 "Changes, correlation and timeline".
const (
	// ClassRollout is a pod template edit (spec.template).
	ClassRollout ChangeClass = "rollout"
	// ClassImage is a container image edit.
	ClassImage ChangeClass = "image"
	// ClassScale is a replica or autoscaler bound edit.
	ClassScale ChangeClass = "scale"
	// ClassConfig is a ConfigMap or Secret data edit.
	ClassConfig ChangeClass = "config"
	// ClassRBAC is a Role, binding or ServiceAccount edit.
	ClassRBAC ChangeClass = "rbac"
	// ClassPolicy is a NetworkPolicy, quota, LimitRange or PDB edit.
	ClassPolicy ChangeClass = "policy"
	// ClassLabels is an edit of labels or selectors that decide which
	// pods an object selects.
	ClassLabels ChangeClass = "labels"
	// ClassTaints is a node taint or cordon edit.
	ClassTaints ChangeClass = "taints"
	// ClassCRDSpec is a custom resource spec edit (a generation bump).
	ClassCRDSpec ChangeClass = "crd-spec"
	// ClassNodeAdded is a node joining the cluster.
	ClassNodeAdded ChangeClass = "node-added"
	// ClassNodeRemoved is a node leaving the cluster.
	ClassNodeRemoved ChangeClass = "node-removed"
	// ClassCreated and ClassDeleted are any other object appearing or
	// disappearing.
	ClassCreated ChangeClass = "created"
	ClassDeleted ChangeClass = "deleted"
	// ClassOther is a meaningful change no rule above describes.
	ClassOther ChangeClass = "other"
)

// kindClasses gives every change of these kinds one class. The kinds are
// the stable lower-case names the Kubernetes source uses.
var kindClasses = map[Kind]ChangeClass{
	"configmap":           ClassConfig,
	"secret":              ClassConfig,
	"role":                ClassRBAC,
	"rolebinding":         ClassRBAC,
	"clusterrole":         ClassRBAC,
	"clusterrolebinding":  ClassRBAC,
	"serviceaccount":      ClassRBAC,
	"networkpolicy":       ClassPolicy,
	"resourcequota":       ClassPolicy,
	"limitrange":          ClassPolicy,
	"poddisruptionbudget": ClassPolicy,
}

// pathRule classifies a field path. Rules are checked in order and the
// first match wins, so the more specific rules come first.
type pathRule struct {
	contains string
	class    ChangeClass
}

var pathRules = []pathRule{
	{"].image", ClassImage},
	{"replicas", ClassScale},
	{"taints", ClassTaints},
	{"unschedulable", ClassTaints},
	{"selector", ClassLabels},
	{"labels", ClassLabels},
	{"template", ClassRollout},
	{"containers[", ClassRollout},
	{"data.", ClassConfig},
}

// Classify returns the class of c. An image edit wins over the rollout
// it causes, because the image is what a reader wants to know.
func (c Change) Classify() ChangeClass {
	if class, ok := lifecycleClass(c); ok {
		return class
	}
	if class, ok := kindClasses[c.Entity.Kind]; ok {
		return class
	}
	best := ClassOther
	for _, field := range c.Fields {
		class := fieldClass(field.Path)
		if classRank(class) > classRank(best) {
			best = class
		}
	}
	if best == ClassOther && c.Entity.Group != "" && specOnly(c) {
		return ClassCRDSpec
	}
	return best
}

// lifecycleClass classifies creation and deletion.
func lifecycleClass(c Change) (ChangeClass, bool) {
	switch {
	case c.Created && c.Entity.Kind == "node":
		return ClassNodeAdded, true
	case c.Deleted && c.Entity.Kind == "node":
		return ClassNodeRemoved, true
	case c.Created:
		return ClassCreated, true
	case c.Deleted:
		return ClassDeleted, true
	}
	return "", false
}

func fieldClass(path string) ChangeClass {
	path = strings.ToLower(path)
	for _, rule := range pathRules {
		if strings.Contains(path, rule.contains) {
			return rule.class
		}
	}
	return ClassOther
}

// classRank orders field classes when one change touches several paths.
func classRank(class ChangeClass) int {
	switch class {
	case ClassImage:
		return 6
	case ClassConfig:
		return 5
	case ClassRollout:
		return 4
	case ClassLabels, ClassTaints:
		return 3
	case ClassScale:
		return 2
	}
	return 0
}

// specOnly reports a change whose only field is the whole spec, which is
// how custom resources report a generation bump.
func specOnly(c Change) bool {
	return len(c.Fields) == 1 && c.Fields[0].Path == "spec"
}
