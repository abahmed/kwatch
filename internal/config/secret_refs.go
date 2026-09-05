package config

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

var sensitiveFieldNames = map[string]bool{
	"accountsid":      true,
	"accesskey":       true,
	"accesskeyid":     true,
	"accesstoken":     true,
	"apikey":          true,
	"apisecret":       true,
	"apitoken":        true,
	"applicationkey":  true,
	"authid":          true,
	"authtoken":       true,
	"clientsecret":    true,
	"credential":      true,
	"credentials":     true,
	"gatewayid":       true,
	"integrationkey":  true,
	"password":        true,
	"privatekey":      true,
	"routingkey":      true,
	"secret":          true,
	"secretaccesskey": true,
	"servicekey":      true,
	"teamsecret":      true,
	"token":           true,
}

// Some provider endpoints contain the credential in the URL itself.
var sensitiveProviderFields = map[string]map[string]bool{
	"discord":       {"webhook": true},
	"feishu":        {"webhook": true},
	"flock":         {"webhook": true},
	"googlechat":    {"webhook": true},
	"ifttt":         {"key": true},
	"incident.io":   {"url": true},
	"incidentio":    {"url": true},
	"mattermost":    {"webhook": true},
	"n8n":           {"url": true},
	"ntfy":          {"topic": true},
	"pushover":      {"user": true},
	"rocketchat":    {"webhook": true},
	"slack":         {"webhook": true},
	"teams":         {"webhook": true},
	"teamsworkflow": {"webhook": true},
	"webhook":       {"url": true},
	"wecom":         {"webhook": true},
	"zapier":        {"url": true},
}

func validateSecretReferences(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &document); err != nil {
		return err
	}
	root := yamlDocumentRoot(&document)
	if root == nil || root.Kind != yaml.MappingNode {
		return nil
	}
	if yamlUsesAliases(root) {
		return errors.New(
			"YAML aliases and merge keys are not allowed in config",
		)
	}

	var errs []error
	validateSensitivePath(
		root,
		[]string{"healthCheck", "diagnosticsToken"},
		&errs,
	)
	validateSensitivePath(
		root,
		[]string{"heartbeatMonitor", "url"},
		&errs,
	)
	alert := yamlMapValue(root, "alert")
	if alert != nil {
		validateAlertSecrets(alert, &errs)
	}
	return errors.Join(errs...)
}

func yamlUsesAliases(node *yaml.Node) bool {
	if node.Kind == yaml.AliasNode {
		return true
	}
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == "<<" {
				return true
			}
		}
	}
	for _, child := range node.Content {
		if yamlUsesAliases(child) {
			return true
		}
	}
	return false
}

func yamlDocumentRoot(document *yaml.Node) *yaml.Node {
	if document.Kind == yaml.DocumentNode && len(document.Content) == 1 {
		return document.Content[0]
	}
	return document
}

func yamlMapValue(node *yaml.Node, wanted string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == wanted {
			return node.Content[i+1]
		}
	}
	return nil
}

func validateSensitivePath(
	root *yaml.Node,
	path []string,
	errs *[]error,
) {
	node := root
	for _, part := range path {
		node = yamlMapValue(node, part)
		if node == nil {
			return
		}
	}
	validateFileReference(node, strings.Join(path, "."), errs)
}

func validateAlertSecrets(alert *yaml.Node, errs *[]error) {
	if alert.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(alert.Content); i += 2 {
		provider := strings.ToLower(alert.Content[i].Value)
		cfg := alert.Content[i+1]
		validateProviderMap(provider, cfg, "alert."+provider, true, errs)
	}
}

func validateProviderMap(
	provider string,
	node *yaml.Node,
	path string,
	topLevel bool,
	errs *[]error,
) {
	if node.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		value := node.Content[i+1]
		fieldPath := path + "." + key
		normalized := normalizeSecretField(key)
		sensitive := sensitiveFieldNames[normalized]
		if fields := sensitiveProviderFields[provider]; topLevel && fields[key] {
			sensitive = true
		}
		if sensitive {
			validateFileReference(value, fieldPath, errs)
		}
		if provider == "webhook" && topLevel && key == "headers" {
			validateWebhookHeaders(value, fieldPath, errs)
		}
		validateProviderMap(provider, value, fieldPath, false, errs)
	}
}

func normalizeSecretField(value string) string {
	value = strings.ToLower(value)
	return strings.NewReplacer(
		"_", "",
		"-", "",
		".", "",
	).Replace(value)
}

func validateWebhookHeaders(
	node *yaml.Node,
	path string,
	errs *[]error,
) {
	if node.Kind != yaml.SequenceNode {
		return
	}
	for i, item := range node.Content {
		value := yamlMapValue(item, "value")
		if value != nil {
			validateFileReference(
				value,
				fmt.Sprintf("%s[%d].value", path, i),
				errs,
			)
		}
	}
}

func validateFileReference(
	node *yaml.Node,
	path string,
	errs *[]error,
) {
	if node.Kind == yaml.ScalarNode && node.Value == "" {
		return
	}
	if node.Kind == yaml.ScalarNode && fileRefRe.MatchString(node.Value) {
		return
	}
	*errs = append(
		*errs,
		fmt.Errorf(
			"%s is sensitive and must use an absolute "+
				"${file:/path} reference",
			path,
		),
	)
}
