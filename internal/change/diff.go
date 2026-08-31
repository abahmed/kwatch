package change

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type FieldChange struct {
	Path   string `json:"path"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
	Action string `json:"action"`
}

type Result struct {
	Fields     []FieldChange `json:"fields,omitempty"`
	BeforeHash string        `json:"beforeHash,omitempty"`
	AfterHash  string        `json:"afterHash,omitempty"`
	Additional int           `json:"additional,omitempty"`
}

const maxFields = 12

// Diff compares meaningful Kubernetes spec/metadata fields while discarding
// controller churn. Secret values are represented by a hash, never plaintext.
func Diff(oldObj, newObj interface{}) Result {
	oldMap := normalize(oldObj)
	newMap := normalize(newObj)
	result := Result{BeforeHash: hash(oldMap), AfterHash: hash(newMap)}
	var fields []FieldChange
	walk(oldMap, newMap, "", &fields)
	sort.Slice(fields, func(i, j int) bool { return fields[i].Path < fields[j].Path })
	if len(fields) > maxFields {
		result.Additional = len(fields) - maxFields
		fields = fields[:maxFields]
	}
	result.Fields = fields
	return result
}

func normalize(obj interface{}) map[string]interface{} {
	if obj == nil {
		return nil
	}
	if ro, ok := obj.(runtime.Object); ok {
		if value, err := runtime.DefaultUnstructuredConverter.ToUnstructured(ro); err == nil {
			strip(value, isSecret(ro))
			return value
		}
	}
	return nil
}

func isSecret(obj runtime.Object) bool {
	_, ok := obj.(*corev1.Secret)
	return ok
}

func strip(value map[string]interface{}, secret bool) {
	meta, _ := value["metadata"].(map[string]interface{})
	for _, key := range []string{"managedFields", "resourceVersion", "generation", "creationTimestamp", "uid"} {
		delete(meta, key)
	}
	delete(value, "status")
	if kind, _ := value["kind"].(string); secret || strings.EqualFold(kind, "Secret") {
		for _, key := range []string{"data", "stringData"} {
			if fields, ok := value[key].(map[string]interface{}); ok {
				for name := range fields {
					fields[name] = "<redacted>"
				}
			}
		}
	}
}

func hash(value map[string]interface{}) string {
	if value == nil {
		return ""
	}
	b, _ := json.Marshal(value)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

func walk(oldValue, newValue interface{}, path string, out *[]FieldChange) {
	if oldList, ok := oldValue.([]interface{}); ok {
		if newList, ok := newValue.([]interface{}); ok {
			walkNamedList(oldList, newList, path, out)
			return
		}
	}
	om, ook := oldValue.(map[string]interface{})
	nm, nok := newValue.(map[string]interface{})
	if ook || nok {
		keys := map[string]bool{}
		for k := range om {
			keys[k] = true
		}
		for k := range nm {
			keys[k] = true
		}
		ordered := make([]string, 0, len(keys))
		for k := range keys {
			ordered = append(ordered, k)
		}
		sort.Strings(ordered)
		for _, k := range ordered {
			walk(om[k], nm[k], join(path, k), out)
		}
		return
	}
	if fmt.Sprint(oldValue) == fmt.Sprint(newValue) {
		return
	}
	action := "changed"
	if oldValue == nil {
		action = "added"
	}
	if newValue == nil {
		action = "removed"
	}
	*out = append(*out, FieldChange{Path: path, Before: stringify(oldValue), After: stringify(newValue), Action: action})
}

func walkNamedList(oldList, newList []interface{}, path string, out *[]FieldChange) {
	byName := func(items []interface{}) map[string]interface{} {
		result := make(map[string]interface{}, len(items))
		for i, item := range items {
			if object, ok := item.(map[string]interface{}); ok {
				if name, ok := object["name"].(string); ok && name != "" {
					result[name] = item
					continue
				}
			}
			result[fmt.Sprintf("#%d", i)] = item
		}
		return result
	}
	oldByName, newByName := byName(oldList), byName(newList)
	keys := map[string]bool{}
	for key := range oldByName {
		keys[key] = true
	}
	for key := range newByName {
		keys[key] = true
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	for _, key := range ordered {
		walk(oldByName[key], newByName[key], join(path, "name="+key), out)
	}
}

func join(base, field string) string {
	if base == "" {
		return field
	}
	return base + "." + field
}
func stringify(value interface{}) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	b, _ := json.Marshal(value)
	return string(b)
}
