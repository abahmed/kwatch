package issue

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"k8s.io/apimachinery/pkg/util/yaml"
	sigsyaml "sigs.k8s.io/yaml"
)

// allowedKinds is the allowlist of namespaced workload fixture kinds. Every
// other kind, including cluster-scoped and RBAC kinds, is rejected.
var allowedKinds = map[string]bool{
	"Pod": true, "Deployment": true, "ReplicaSet": true,
	"StatefulSet": true, "DaemonSet": true, "Job": true, "CronJob": true,
	"Service": true, "ConfigMap": true, "Ingress": true,
	"HorizontalPodAutoscaler": true, "PodDisruptionBudget": true,
	"PersistentVolumeClaim": true, "NetworkPolicy": true,
	"ServiceAccount": true, "HTTPRoute": true,
}

// truthyUnsafeKeys are rejected only when set to true.
var truthyUnsafeKeys = map[string]bool{
	"hostnetwork": true, "hostpid": true, "hostipc": true,
	"privileged": true, "automountserviceaccounttoken": true,
}

// SanitizeResources validates and rewrites declarative resources for the
// disposable scenario namespace. It replaces every workload image with the
// already-loaded local test image and never contacts a registry.
func SanitizeResources(
	input []byte,
	namespace string,
	image string,
) ([]byte, error) {
	if err := ValidateInput(string(input)); err != nil {
		return nil, err
	}
	if strings.TrimSpace(namespace) == "" {
		return nil, fmt.Errorf("scenario namespace is required")
	}
	if err := ValidateImage(image); err != nil {
		return nil, err
	}
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(input), 4096)
	var documents [][]byte
	for {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("decode resources: %w", err)
		}
		if len(bytes.TrimSpace(raw)) == 0 || string(raw) == "null" {
			continue
		}
		var object map[string]interface{}
		if err := json.Unmarshal(raw, &object); err != nil {
			return nil, fmt.Errorf("resource is not an object: %w", err)
		}
		if err := sanitizeObject(object, namespace, image); err != nil {
			return nil, err
		}
		encoded, err := sigsyaml.Marshal(object)
		if err != nil {
			return nil, fmt.Errorf("encode resource: %w", err)
		}
		documents = append(documents, bytes.TrimSpace(encoded))
	}
	if len(documents) == 0 {
		return nil, fmt.Errorf("resources block contains no objects")
	}
	return bytes.Join(documents, []byte("\n---\n")), nil
}

func sanitizeObject(
	object map[string]interface{},
	namespace string,
	image string,
) error {
	kind, _ := object["kind"].(string)
	if kind == "" {
		return fmt.Errorf("resource kind is required")
	}
	if !allowedKinds[kind] {
		return fmt.Errorf(
			"resource kind %q is not an allowed namespaced workload "+
				"fixture kind", kind,
		)
	}
	metadata, _ := object["metadata"].(map[string]interface{})
	if metadata == nil {
		metadata = map[string]interface{}{}
		object["metadata"] = metadata
	}
	metadata["namespace"] = namespace
	if err := sanitizeValue(object, image); err != nil {
		return fmt.Errorf("resource %q: %w", kind, err)
	}
	return nil
}

func sanitizeValue(value interface{}, image string) error {
	switch typed := value.(type) {
	case map[string]interface{}:
		return sanitizeMap(typed, image)
	case []interface{}:
		for _, child := range typed {
			if err := sanitizeValue(child, image); err != nil {
				return err
			}
		}
	case string:
		return ValidateInput(typed)
	}
	return nil
}

func sanitizeMap(object map[string]interface{}, image string) error {
	if err := rejectUnsafeFields(object); err != nil {
		return err
	}
	_, isContainer := object["image"]
	for key, child := range object {
		lower := strings.ToLower(key)
		if lower == "image" {
			object[key] = image
			continue
		}
		// The image is replaced, so issue commands must never run.
		if isContainer && (lower == "command" || lower == "args") {
			delete(object, key)
			continue
		}
		if err := sanitizeValue(child, image); err != nil {
			return err
		}
	}
	return nil
}

func rejectUnsafeFields(object map[string]interface{}) error {
	for key, child := range object {
		lower := strings.ToLower(key)
		unsafe := lower == "hostpath" ||
			(truthyUnsafeKeys[lower] && truthy(child)) ||
			(lower == "hostport" && nonZero(child)) ||
			(lower == "runasuser" && isZero(child)) ||
			(lower == "capabilities" && addsCapabilities(child)) ||
			(lower == "projected" && projectsToken(child))
		if unsafe {
			return fmt.Errorf("unsafe field %q is not allowed", key)
		}
	}
	return nil
}

func addsCapabilities(value interface{}) bool {
	capabilities, _ := value.(map[string]interface{})
	added, _ := capabilities["add"].([]interface{})
	return len(added) > 0
}

func projectsToken(value interface{}) bool {
	projected, _ := value.(map[string]interface{})
	sources, _ := projected["sources"].([]interface{})
	for _, source := range sources {
		entry, _ := source.(map[string]interface{})
		if _, found := entry["serviceAccountToken"]; found {
			return true
		}
	}
	return false
}

func nonZero(value interface{}) bool {
	number, ok := value.(float64)
	return ok && number != 0
}

func isZero(value interface{}) bool {
	number, ok := value.(float64)
	return ok && number == 0
}

func truthy(value interface{}) bool {
	flag, ok := value.(bool)
	return ok && flag
}
