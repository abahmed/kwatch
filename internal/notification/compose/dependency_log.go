package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/redact"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// dependencyLogCause reports a cause read from a dependency the log
// names (explain.DependencyLogRow).
func dependencyLogCause(cause *rootcause.CauseRecord) bool {
	return cause != nil && cause.Rule == explain.DependencyLogRow
}

// dependencyOfCause is the dependency the cause's log line names.
func dependencyOfCause(
	cause *rootcause.CauseRecord,
) (explain.Dependency, bool) {
	if !dependencyLogCause(cause) {
		return explain.Dependency{}, false
	}
	d, ok := explain.DependencyIn(cause.SharedError)
	// The host is shown outside the quote (which clip redacts) and put
	// in commands: no credential of the log line may come with it.
	d.Host = redact.Credentials(d.Host)
	return d, ok
}

// dependencyWords is the clause of a dependency cause about its
// workload: "it cannot connect to Redis (from its log: "…")".
func dependencyWords(cause *rootcause.CauseRecord) string {
	d, ok := dependencyOfCause(cause)
	if !ok {
		return "it cannot connect to a service it depends on"
	}
	return "it cannot connect to " + d.Label() + " (from its log: " +
		quoted(cause.SharedError) + ")"
}

// dependencySteps suggest checking the dependency: the Service and its
// endpoints when the host is one of the cluster, else the address the
// workload is configured with. Nothing is run against the dependency.
func dependencySteps(
	cause *rootcause.CauseRecord, namespace, workload string,
) []notification.Step {
	d, ok := dependencyOfCause(cause)
	if !ok {
		return nil
	}
	label := d.Name
	if label == "" {
		label = "the dependency"
	}
	if cause.Mode.Within(explain.ModeDependencyService) {
		name, ns := serviceOfHost(d.Host, namespace)
		return []notification.Step{{
			Text: "Check that " + label + " is reachable from " +
				namespace + ": see its Service and endpoints",
			Command: "kubectl get svc,endpoints " + quote(name) +
				namespaceFlag(ns),
		}}
	}
	return []notification.Step{{Text: "Check the " + label +
		" address in " + workload + "'s configuration and that " +
		addressOf(d) + " is reachable from " + namespace}}
}

// addressOf is the host the log names, or words for it.
func addressOf(d explain.Dependency) string {
	if d.Host != "" {
		return d.Host
	}
	return "it"
}

// serviceOfHost splits a Service host ("db", "db.data",
// "db.data.svc.cluster.local", with or without a port) into its name and
// namespace; a bare name is in namespace.
func serviceOfHost(host, namespace string) (name, ns string) {
	host, _, _ = strings.Cut(host, ":")
	parts := strings.Split(host, ".")
	if len(parts) > 1 {
		return parts[0], parts[1]
	}
	return parts[0], namespace
}
