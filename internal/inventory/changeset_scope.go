package inventory

import "strings"

// entityScope is what an entity is part of and what it uses, for deciding
// whether two changes are related.
type entityScope struct {
	// anchors are the entity and its owner chain.
	anchors []EntityID
	// uses are the objects the anchors reference, scale or select.
	uses []EntityID
}

// useRelations are the relations that make a changed object part of a
// release of the entity that points at it.
var useRelations = []RelationType{References, Scales, Selects, Constrains}

// scopeLocked builds id's scope. The caller holds the model's lock.
func (m *Model) scopeLocked(id EntityID) entityScope {
	scope := entityScope{anchors: []EntityID{id}}
	current := id
	for depth := 0; depth < maxOwnerDepth; depth++ {
		owners := m.edges.neighbors(current, OwnedBy, Outgoing)
		if len(owners) == 0 || containsID(scope.anchors, owners[0]) {
			break
		}
		current = owners[0]
		scope.anchors = append(scope.anchors, current)
	}
	for _, anchor := range scope.anchors {
		for _, relation := range useRelations {
			scope.uses = append(scope.uses,
				m.edges.neighbors(anchor, relation, Outgoing)...)
		}
	}
	return scope
}

// overlaps reports whether two scopes share an anchor or one uses the
// other's anchor.
func (s entityScope) overlaps(other entityScope) bool {
	for _, anchor := range s.anchors {
		if containsID(other.anchors, anchor) || containsID(other.uses, anchor) {
			return true
		}
	}
	for _, anchor := range other.anchors {
		if containsID(s.uses, anchor) {
			return true
		}
	}
	return false
}

// kindTitles are the display names of kinds that appear in labels.
var kindTitles = map[Kind]string{
	"configmap":   "ConfigMap",
	"secret":      "Secret",
	"deployment":  "Deployment",
	"statefulset": "StatefulSet",
	"daemonset":   "DaemonSet",
	"replicaset":  "ReplicaSet",
	"service":     "Service",
	"node":        "node",
}

func kindTitle(kind Kind) string {
	if title, ok := kindTitles[kind]; ok {
		return title
	}
	return string(kind)
}

// describeChange is a change's label item, such as "image v2.3",
// "ConfigMap app-config" or "api scale 2→5".
func describeChange(change Change) string {
	name := kindTitle(change.Entity.Kind) + " " + change.Entity.Name
	switch class := change.Classify(); class {
	case ClassImage:
		return "image " + imageTag(fieldAfter(change, "].image"))
	case ClassConfig:
		return name
	case ClassScale:
		field := firstField(change, "replicas")
		return change.Entity.Name + " scale " + orDash(field.Before) +
			"→" + orDash(field.After)
	case ClassNodeAdded:
		return name + " added"
	case ClassNodeRemoved:
		return name + " removed"
	default:
		return name + " " + string(class)
	}
}

// imageTag shortens "registry/team/api:v2.3" to "v2.3" and a digest
// reference to its first characters.
func imageTag(image string) string {
	if _, digest, ok := strings.Cut(image, "@"); ok {
		_, hash, _ := strings.Cut(digest, ":")
		return "@" + hash[:min(len(hash), 12)]
	}
	slash := strings.LastIndex(image, "/")
	if colon := strings.LastIndex(image, ":"); colon > slash {
		return image[colon+1:]
	}
	return image
}

func fieldAfter(change Change, contains string) string {
	return firstField(change, contains).After
}

func firstField(change Change, contains string) FieldChange {
	for _, field := range change.Fields {
		if strings.Contains(strings.ToLower(field.Path), contains) {
			return field
		}
	}
	return FieldChange{}
}

func orDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
