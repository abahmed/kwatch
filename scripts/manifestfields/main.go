// Command manifestfields prints the fields of a multi-document manifest
// that must agree between deploy/deploy.yaml and the Helm chart: the
// kwatch Deployment and the RBAC rules. One "path = value" line per field,
// sorted, so two files can be compared with diff.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
	sigyaml "sigs.k8s.io/yaml"
)

type object = map[string]any

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: manifestfields FILE")
		os.Exit(2)
	}
	lines, err := fields(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "manifestfields: %v\n", err)
		os.Exit(2)
	}
	sort.Strings(lines)
	for _, line := range lines {
		fmt.Println(line)
	}
}

func fields(path string) ([]string, error) {
	// #nosec G304 -- maintainer-selected manifest path
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	decoder := yaml.NewYAMLOrJSONDecoder(file, 4096)
	var lines []string
	for {
		doc := unstructured.Unstructured{}
		err := decoder.Decode(&doc.Object)
		if errors.Is(err, io.EOF) {
			return lines, nil
		}
		if err != nil {
			return nil, err
		}
		switch doc.GetKind() {
		case "Deployment":
			lines = append(lines, deployment(doc.Object)...)
		case "ClusterRole", "Role":
			lines = append(lines, role(doc.Object)...)
		}
	}
}

func get(o any, keys ...string) any {
	for _, key := range keys {
		m, ok := o.(object)
		if !ok {
			return nil
		}
		o = m[key]
	}
	return o
}

func list(o any, keys ...string) []object {
	var out []object
	items, _ := get(o, keys...).([]any)
	for _, item := range items {
		if m, ok := item.(object); ok {
			out = append(out, m)
		}
	}
	return out
}

// js renders a value as canonical one-line JSON (keys sorted).
func js(v any) string {
	out, err := sigyaml.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	one := strings.Join(strings.Fields(string(out)), " ")
	return strings.ReplaceAll(one, "- ", "-")
}

func line(path string, v any) string {
	return path + " = " + js(v)
}

func deployment(d object) []string {
	pod := get(d, "spec", "template", "spec")
	out := []string{
		line("deploy.replicas", get(d, "spec", "replicas")),
		line("deploy.strategy", get(d, "spec", "strategy", "type")),
		line("deploy.grace", get(pod, "terminationGracePeriodSeconds")),
		line("deploy.serviceAccountAutomount",
			get(pod, "automountServiceAccountToken")),
		line("deploy.restartPolicy", get(pod, "restartPolicy")),
		line("deploy.podSecurityContext", get(pod, "securityContext")),
		line("deploy.tolerations", get(pod, "tolerations")),
	}
	for _, key := range keys(get(d, "spec", "template", "metadata",
		"annotations")) {
		if !strings.HasPrefix(key, "checksum/") {
			out = append(out, "deploy.annotation "+key)
		}
	}
	for _, c := range list(pod, "containers") {
		out = append(out, container(c)...)
	}
	for _, v := range list(pod, "volumes") {
		name, _ := v["name"].(string)
		out = append(out, "volume "+name+" source = "+
			strings.Join(keys(withoutName(v)), ","))
	}
	return out
}

func container(c object) []string {
	name, _ := c["name"].(string)
	p := "container " + name + "."
	image, _ := c["image"].(string)
	if i := strings.LastIndex(image, ":"); i > 0 {
		image = image[:i]
	}
	out := []string{
		p + "imageRepo = " + image,
		line(p+"pullPolicy", c["imagePullPolicy"]),
		line(p+"securityContext", c["securityContext"]),
		line(p+"resources", c["resources"]),
		line(p+"args", c["args"]),
		line(p+"command", c["command"]),
	}
	for _, probe := range []string{"startupProbe", "livenessProbe",
		"readinessProbe"} {
		out = append(out, line(p+probe, c[probe]))
	}
	for _, e := range list(c, "env") {
		n, _ := e["name"].(string)
		out = append(out, line(p+"env "+n, withoutName(e)))
	}
	for _, m := range list(c, "volumeMounts") {
		n, _ := m["name"].(string)
		out = append(out, line(p+"mount "+n, withoutName(m)))
	}
	for _, port := range list(c, "ports") {
		n, _ := port["name"].(string)
		out = append(out, line(p+"port "+n, withoutName(port)))
	}
	return out
}

func role(r object) []string {
	kind, _ := r["kind"].(string)
	name, _ := get(r, "metadata", "name").(string)
	ns, _ := get(r, "metadata", "namespace").(string)
	// The chart prefixes the ClusterRole name with the release name.
	if kind == "ClusterRole" {
		name = "(cluster)"
	}
	var out []string
	for _, rule := range list(r, "rules") {
		out = append(out, fmt.Sprintf("%s %s %s rule = %s", kind, name,
			ns, js(rule)))
	}
	return out
}

func withoutName(o object) object {
	out := object{}
	for k, v := range o {
		if k != "name" {
			out[k] = v
		}
	}
	return out
}

func keys(o any) []string {
	m, _ := o.(object)
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
