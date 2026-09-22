package issue

import (
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

var forbiddenFields = []string{
	"password",
	"token",
	"secret",
	"credential",
	"apikey",
	"api_key",
	"accesskey",
	"access_key",
	"secretkey",
	"secret_key",
	"bearer",
	"authorization",
	"privatekey",
	"hostpath",
	"privileged",
	"hostnetwork",
	"hostpid",
	"hostipc",
	"command",
	"args",
}

func Validate(blocks Blocks) error {
	for name, payload := range map[string]string{
		"config": blocks.Config, "resources": blocks.Resources,
		"expectation": blocks.Expectation,
	} {
		lower := strings.ToLower(payload)
		for _, field := range forbiddenFields {
			if strings.Contains(lower, field) {
				return fmt.Errorf("issue %s block contains forbidden field %q", name, field)
			}
		}
		if strings.Contains(lower, "kubectl") ||
			strings.Contains(lower, "docker") ||
			strings.Contains(lower, "curl") ||
			strings.Contains(lower, "http://") ||
			strings.Contains(lower, "https://") {
			return fmt.Errorf("issue %s block contains an executable command", name)
		}
	}
	return nil
}

var allowedKinds = map[string]bool{
	"ConfigMap": true, "DaemonSet": true, "Deployment": true,
	"Job": true, "PersistentVolumeClaim": true, "Pod": true,
	"PodDisruptionBudget": true, "ReplicaSet": true, "Service": true,
	"StatefulSet": true,
}

func SanitizeResources(
	resources, namespace, image string,
) (string, error) {
	if strings.TrimSpace(namespace) == "" || strings.TrimSpace(image) == "" {
		return "", fmt.Errorf("namespace and image are required")
	}
	decoder := yaml.NewDecoder(strings.NewReader(resources))
	var documents []string
	for {
		var document yaml.Node
		if err := decoder.Decode(&document); err != nil {
			if err == io.EOF {
				break
			}
			return "", fmt.Errorf("decode issue resources: %w", err)
		}
		if len(document.Content) == 0 {
			continue
		}
		root := document.Content[0]
		kind, err := mappingValue(root, "kind")
		if err != nil {
			return "", err
		}
		if !allowedKinds[kind] {
			return "", fmt.Errorf("resource kind %q is not allowed", kind)
		}
		if err := ensureNamespace(root, namespace); err != nil {
			return "", err
		}
		if err := rewriteResource(root, namespace, image); err != nil {
			return "", err
		}
		var builder strings.Builder
		encoder := yaml.NewEncoder(&builder)
		if err := encoder.Encode(root); err != nil {
			return "", fmt.Errorf("encode sanitized resource: %w", err)
		}
		_ = encoder.Close()
		documents = append(documents, strings.TrimSpace(builder.String()))
	}
	if len(documents) == 0 {
		return "", fmt.Errorf("issue resources are empty")
	}
	return strings.Join(documents, "\n---\n") + "\n", nil
}

func ensureNamespace(root *yaml.Node, namespace string) error {
	metadata, ok := mappingNode(root, "metadata")
	if !ok {
		return fmt.Errorf("resource is missing metadata")
	}
	if _, ok := mappingNode(metadata, "namespace"); ok {
		return nil
	}
	metadata.Content = append(metadata.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "namespace"},
		&yaml.Node{Kind: yaml.ScalarNode, Value: namespace},
	)
	return nil
}

func mappingNode(node *yaml.Node, key string) (*yaml.Node, bool) {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, false
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		if node.Content[index].Value == key {
			return node.Content[index+1], true
		}
	}
	return nil, false
}

func rewriteResource(node *yaml.Node, namespace, image string) error {
	if node.Kind == yaml.MappingNode {
		for index := 0; index+1 < len(node.Content); index += 2 {
			key, value := node.Content[index], node.Content[index+1]
			switch key.Value {
			case "namespace":
				value.Value = namespace
			case "image":
				value.Value = image
			}
			if err := rewriteResource(value, namespace, image); err != nil {
				return err
			}
		}
	}
	for _, child := range node.Content {
		if err := rewriteResource(child, namespace, image); err != nil {
			return err
		}
	}
	return nil
}

func mappingValue(node *yaml.Node, key string) (string, error) {
	if node.Kind != yaml.MappingNode {
		return "", fmt.Errorf("resource must be a mapping")
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		if node.Content[index].Value == key {
			return node.Content[index+1].Value, nil
		}
	}
	return "", fmt.Errorf("resource is missing %q", key)
}
