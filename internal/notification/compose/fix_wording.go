package compose

import (
	"path"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
)

// This file words the fix line: what to change, said as an action with
// the value it holds now. "Fix: raise the memory limit of container app
// in deployment/api above 256Mi." A field kwatch has no words for keeps
// its path, still with a verb: "Fix: change spec.ports of service/web."

// fixLabel starts the fix line.
const fixLabel = "Fix: "

// fixRule words the fix for the fields whose path matches. words says
// the action: "{t}" is the container in its object (or the object when
// the field is the object's own), "{o}" the object. value says how the
// field's value follows: " above 256Mi", " below 2" or " (now 8081)".
type fixRule struct {
	match func(path string) bool
	words string
	value func(string) string
}

// fixRules are tried in order; the first that matches words the fix.
var fixRules = []fixRule{
	{fixExact("resources.limits.memory"),
		"raise the memory limit of {t}", valueAbove},
	{fixExact("resources.limits.cpu"), "raise the CPU limit of {t}", valueAbove},
	{fixExact("resources.limits.ephemeral-storage"),
		"raise the ephemeral storage limit of {t}", valueAbove},
	{fixExact("spec.resources.requests.storage"),
		"raise the storage request of {o}", valueAbove},
	{fixExact("spec.maxReplicas"), "raise maxReplicas of {o}", valueAbove},
	{fixExact("spec.minAvailable"), "lower minAvailable of {o}", valueBelow},
	{fixExact("spec.maxUnavailable"), "raise maxUnavailable of {o}",
		valueAbove},
	{fixExact("spec.replicas"), "scale up {o}", valueReplicas},
	{fixExact("image"), "change the image of {t}", valueNow},
	{probePort, "set the {p} probe of {t} to a port the app listens on",
		valueNow},
	{probeTiming, "give the {p} probe of {t} more time", valueNow},
	{fixPrefix("envFrom."), "correct the envFrom reference of {t}", valueNow},
	{fixExact("spec.schedule"), "correct the schedule of {o}", valueNow},
	{fixSuffix("tolerations"),
		"add the toleration the node taint needs to {o}", valueUnlessNone},
	{fixSuffix("nodeSelector"), "correct the node selector of {o}", valueNow},
	{fixExact("spec.selector"),
		"correct the selector of {o} so it matches the pods", valueNow},
	{fixExact("spec.ports"), "correct the ports of {o}", valuePorts},
	{fixExact("spec.rules"), "correct the rules of {o}", valueNow},
	{fixExact("spec.tls[].secretName"),
		"renew or replace the TLS secret of {o}", valueNow},
	{fixExact("spec.storageClassName"),
		"set a storage class that exists on {o}", valueNow},
	{fixSuffix("backend.service.name"),
		"point {o} at a service that exists", valueNow},
	{fixSuffix("backendRefs[].name"),
		"point {o} at a service that exists", valueNow},
}

// fixText is the fix line of a place. valueSaid drops the value when an
// earlier sentence of the note already names it.
func fixText(p detection.Place, valueSaid bool) string {
	object := strings.ToLower(string(p.Object.Kind)) + "/" + p.Object.Name
	target := object
	if p.Container != "" {
		target = "container " + p.Container + " in " + object
	}
	value := p.Value
	if valueSaid {
		value = ""
	}
	if key, ok := envKey(p.Path); ok {
		return fixLabel + envWords(key, p.Path) + " of " + target +
			valueNow(value) + "."
	}
	for _, rule := range fixRules {
		if rule.match(p.Path) {
			kind, _ := probeKind(p.Path)
			words := strings.NewReplacer("{t}", target, "{o}", object,
				"{p}", kind).Replace(rule.words)
			return fixLabel + words + rule.value(value) + "."
		}
	}
	return fixLabel + "change " + p.Path + " of " + target +
		valueNow(value) + "."
}

// valueNow is " (now 256Mi)", or nothing without a value.
func valueNow(value string) string {
	if value == "" {
		return ""
	}
	return " (now " + value + ")"
}

func valueAbove(value string) string { return valueWord(" above ", value) }

func valueBelow(value string) string { return valueWord(" below ", value) }

func valueWord(word, value string) string {
	if value == "" {
		return ""
	}
	return word + value
}

func valueReplicas(value string) string {
	if value == "" {
		return ""
	}
	if value == "1" {
		return " (now 1 replica)"
	}
	return " (now " + value + " replicas)"
}

// valuePorts words a Service's ports as the inventory writes them
// ("80/TCP->web,443/TCP->https") for a reader: " (now port 80 → web)",
// " (now ports 80 → web, 443 → https)". TCP, the default, is left out.
func valuePorts(value string) string {
	if value == "" {
		return ""
	}
	parts := strings.Split(value, ",")
	words := make([]string, 0, len(parts))
	for _, part := range parts {
		port, target, ok := strings.Cut(part, "->")
		if !ok {
			return valueNow(value)
		}
		number, protocol, _ := strings.Cut(port, "/")
		if protocol != "" && protocol != "TCP" {
			number += "/" + protocol
		}
		words = append(words, number+" → "+target)
	}
	noun := "port "
	if len(words) > 1 {
		noun = "ports "
	}
	return " (now " + noun + strings.Join(words, ", ") + ")"
}

func valueUnlessNone(value string) string {
	if value == "none" {
		return ""
	}
	return valueNow(value)
}

func fixExact(want string) func(string) bool {
	return func(path string) bool { return path == want }
}

func fixPrefix(want string) func(string) bool {
	return func(path string) bool { return strings.HasPrefix(path, want) }
}

func fixSuffix(want string) func(string) bool {
	return func(path string) bool { return strings.HasSuffix(path, want) }
}

// probeKind is "readiness" for "readinessProbe.httpGet.port".
func probeKind(field string) (string, bool) {
	head, _, _ := strings.Cut(field, ".")
	kind, ok := strings.CutSuffix(head, "Probe")
	return kind, ok && kind != ""
}

func probePort(field string) bool {
	_, ok := probeKind(field)
	return ok && strings.HasSuffix(field, ".port")
}

func probeTiming(field string) bool {
	_, ok := probeKind(field)
	return ok && !strings.Contains(field, ".")
}

// envKey is the variable of an env field: DB_HOST for
// "env[DB_HOST]" and "env[DB_HOST].valueFrom.secretKeyRef".
func envKey(field string) (string, bool) {
	rest, ok := strings.CutPrefix(field, "env[")
	if !ok {
		return "", false
	}
	key, _, found := strings.Cut(rest, "]")
	return key, found && key != ""
}

// envWords names the variable, or the object it reads: "correct env
// var DB_HOST", "correct the Secret that env var STRIPE_KEY reads".
func envWords(key, field string) string {
	switch {
	case strings.Contains(field, "secretKeyRef"):
		return "correct the Secret that env var " + key + " reads"
	case strings.Contains(field, "configMapKeyRef"):
		return "correct the ConfigMap that env var " + key + " reads"
	}
	return "correct env var " + key
}

// valueNamed reports a note text that already names the image value,
// whole or by its last path part: "payments:2.3" names
// "registry.example.com/payments:2.3".
func valueNamed(text, value string) bool {
	if value == "" {
		return false
	}
	short := path.Base(value)
	if !strings.Contains(short, ":") {
		// Only an image is said in full elsewhere; a short value such
		// as "6" would match any number.
		return false
	}
	return strings.Contains(text, value) || strings.Contains(text, short)
}
