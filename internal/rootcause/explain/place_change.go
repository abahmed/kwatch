package explain

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// settingFields are the object-level fields of a change that are a
// setting someone can set back.
var settingFields = map[string]bool{
	"spec.replicas": true, "spec.minReplicas": true,
	"spec.maxReplicas": true, "spec.selector": true, "spec.ports": true,
	"spec.rules": true, "spec.schedule": true,
}

// containerFields are the fields of a container, below
// "containers[name].", that are a setting someone can set back.
var containerFields = []string{
	"image", "resources.", "readinessProbe", "livenessProbe",
	"startupProbe", "command", "args", "ports",
}

// placeOfChange names the field a blamed change set: "containers[app].
// resources.limits.memory" becomes container app, field
// resources.limits.memory, and the value it was set to. Data of a
// ConfigMap or Secret is no setting of this kind, and a Secret never
// gives a value. An environment variable is named without its value.
func placeOfChange(
	root inventory.EntityID, change inventory.Change,
) *detection.Place {
	if root.Kind == kube.KindSecret || root.Kind == kube.KindConfigMap ||
		change.Created || change.Deleted {
		return nil
	}
	for _, field := range change.Fields {
		if place := placeOfField(root, field); place != nil {
			return place
		}
	}
	return nil
}

func placeOfField(
	root inventory.EntityID, field inventory.FieldChange,
) *detection.Place {
	if settingFields[field.Path] {
		return objectPlace(root, field.Path, field.After)
	}
	name, rest, ok := containerPath(field.Path)
	if !ok {
		return nil
	}
	if key, isEnv := strings.CutPrefix(rest, "env."); isEnv {
		return &detection.Place{Object: root, Container: name,
			Path: "env[" + key + "]"}
	}
	for _, prefix := range containerFields {
		if strings.HasPrefix(rest, prefix) {
			return &detection.Place{Object: root, Container: name,
				Path: rest, Value: field.After}
		}
	}
	return nil
}

// containerPath splits a change path "containers[app].image" into the
// container name and the field below it.
func containerPath(path string) (name, rest string, ok bool) {
	body, found := strings.CutPrefix(path, "containers[")
	if !found {
		return "", "", false
	}
	name, rest, found = strings.Cut(body, "].")
	return name, rest, found && name != "" && rest != ""
}
