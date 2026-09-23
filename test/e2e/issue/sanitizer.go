package issue

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"k8s.io/apimachinery/pkg/util/yaml"
	sigsyaml "sigs.k8s.io/yaml"
)

var rejectedKinds = map[string]bool{
	"ClusterRole": true, "ClusterRoleBinding": true, "Namespace": true,
	"PersistentVolume": true, "Secret": true,
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
			if err == io.EOF {
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
	if rejectedKinds[kind] {
		return fmt.Errorf("resource kind %q is not allowed", kind)
	}
	if kind == "List" {
		return fmt.Errorf("List resources are not allowed")
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
		for key, child := range typed {
			lower := strings.ToLower(key)
			if lower == "hostpath" || lower == "hostnetwork" ||
				lower == "hostpid" || lower == "hostipc" ||
				lower == "privileged" || lower == "serviceaccounttoken" {
				if truthy(child) || lower == "hostpath" {
					return fmt.Errorf("unsafe field %q is not allowed", key)
				}
			}
			if lower == "image" {
				typed[key] = image
				continue
			}
			if err := sanitizeValue(child, image); err != nil {
				return err
			}
		}
	case []interface{}:
		for _, child := range typed {
			if err := sanitizeValue(child, image); err != nil {
				return err
			}
		}
	case string:
		if err := ValidateInput(typed); err != nil {
			return err
		}
	}
	return nil
}

func truthy(value interface{}) bool {
	flag, ok := value.(bool)
	return ok && flag
}
