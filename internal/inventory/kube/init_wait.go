package kube

import (
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// AttrInitSeconds is how long an init container's successful run took,
// in seconds. It is the history a stuck init container is compared with.
const AttrInitSeconds = "init.run.seconds"

// setInitAttributes records what an init container is configured to wait
// for and, once it has completed, how long it took. The Services it names
// are read with the same rules as a pod's calls (see serviceCallIn).
func setInitAttributes(
	attrs map[string]inventory.Value, c corev1.Container,
	status *corev1.ContainerStatus, namespace string,
) {
	if calls := initServiceCalls(c, namespace); len(calls) > 0 {
		attrs[AttrServiceCalls] = inventory.Text(serviceCallsText(calls))
	}
	if status == nil {
		return
	}
	done := status.State.Terminated
	if done == nil || done.ExitCode != 0 || done.StartedAt.IsZero() ||
		done.FinishedAt.Before(&done.StartedAt) {
		return
	}
	took := done.FinishedAt.Sub(done.StartedAt.Time)
	attrs[AttrInitSeconds] = inventory.Number(took.Seconds())
}

// initServiceCalls lists the Services the container's command, arguments
// and environment name. A word is read as an address when it is a URL, a
// host:port, or NAME=value (or --name=value) where the value names a
// Service and the name says it is an address.
func initServiceCalls(c corev1.Container, namespace string) []serviceCall {
	seen := map[string]bool{}
	var out []serviceCall
	add := func(name, value string) {
		call, ok := serviceCallIn(name, value, namespace)
		if ok && !seen[call.String()] {
			seen[call.String()] = true
			out = append(out, call)
		}
	}
	for _, env := range c.Env {
		add(env.Name, env.Value)
	}
	for _, word := range append(append([]string{}, c.Command...), c.Args...) {
		for _, field := range strings.Fields(word) {
			addWord(field, add)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].String() < out[j].String()
	})
	if len(out) > maxPodDependencies {
		out = out[:maxPodDependencies]
	}
	return out
}

// addWord offers one word to add, as it is and, when it has the form
// NAME=value, as a name and a value.
func addWord(word string, add func(name, value string)) {
	word = trimWord(word)
	if name, value, ok := strings.Cut(word, "="); ok {
		add(strings.TrimLeft(name, "-"), trimWord(value))
		return
	}
	add("", word)
}

// trimWord removes the quotes and trailing punctuation that shells and
// log lines put around an address.
func trimWord(word string) string {
	return strings.Trim(word, "\"'`;()[]{}<>,.…")
}

// ServiceRefsIn reads the in-cluster Services that free text names, such
// as a log line "waiting for db:5432...", with the rules used for a pod's
// configuration. namespace resolves bare Service names.
func ServiceRefsIn(text, namespace string) []ServiceRef {
	var out []ServiceRef
	seen := map[inventory.EntityID]bool{}
	for _, field := range strings.Fields(text) {
		field = trimWord(field)
		call, ok := serviceCallIn("", field, namespace)
		if !ok || seen[call.id] {
			continue
		}
		seen[call.id] = true
		ref := ServiceRef{Service: call.id}
		ref.Port, _ = strconv.Atoi(call.port)
		out = append(out, ref)
		if len(out) == maxPodDependencies {
			break
		}
	}
	return out
}
