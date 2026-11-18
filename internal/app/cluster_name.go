package app

import (
	"regexp"
	"strings"
)

// nameConfigured is the source text of a name set in the configuration.
const nameConfigured = "configured"

// noNameHint is the setting that names the cluster, for the notice the
// kwatch page and the log show when no name is known.
const noNameHint = "app.clusterName"

// nodeMeta is the part of a node the cluster name is read from.
type nodeMeta struct {
	Labels      map[string]string
	Annotations map[string]string
}

// nameSource is one node label or annotation that carries the name of
// the cluster the node belongs to. A source is listed only when its
// value is the cluster's own name: AKS's kubernetes.azure.com/cluster is
// the node resource group, and GKE's labels name a node pool, so
// neither is read.
type nameSource struct {
	key        string
	annotation bool
}

// nameSources are tried in this order; the first to find a name is the
// one the page names.
var nameSources = []nameSource{
	{key: "alpha.eksctl.io/cluster-name"},
	{key: "cluster.x-k8s.io/cluster-name", annotation: true},
	{key: "cluster.x-k8s.io/cluster-name"},
}

// ownedTagPrefix starts the label key of a node owned by or sharing a
// cluster: kubernetes.io/cluster/<name> with the value owned or shared.
const ownedTagPrefix = "kubernetes.io/cluster/"

// validName is a cluster name that is plainly one: letters, digits, dot,
// dash and underscore, at most 100 characters.
var validName = regexp.MustCompile(
	`^[A-Za-z0-9]([A-Za-z0-9._-]{0,98}[A-Za-z0-9])?$`)

// genericNames are values several tools use for every cluster they
// create, so they tell two clusters apart no better than no name.
var genericNames = map[string]bool{"kubernetes": true}

// usableName is value without spaces around it when it is a clear
// cluster name.
func usableName(value string) (string, bool) {
	v := strings.TrimSpace(value)
	if !validName.MatchString(v) || genericNames[strings.ToLower(v)] {
		return "", false
	}
	return v, true
}

// clusterNameFromNodes reads the cluster name from node labels and
// annotations. It names the cluster only when every node that carries a
// source agrees on one value and every source that is present gives the
// same value; nodes without a source are ignored (a managed control
// plane is not a node). Any disagreement or doubtful value gives no
// name: kwatch never guesses. from says where the name was read.
func clusterNameFromNodes(nodes []nodeMeta) (name, from string) {
	for _, s := range nameSources {
		got, found, ok := sourceValue(nodes, s)
		if !ok {
			return "", ""
		}
		if found && !agree(&name, &from, got, s.describe()) {
			return "", ""
		}
	}
	got, found, ok := ownedTagValue(nodes)
	if !ok {
		return "", ""
	}
	if found && !agree(&name, &from, got,
		"the node label "+ownedTagPrefix+"<name>") {
		return "", ""
	}
	return name, from
}

// describe says what the source is, for the page.
func (s nameSource) describe() string {
	if s.annotation {
		return "the node annotation " + s.key
	}
	return "the node label " + s.key
}

// agree records the first name found and reports false when a later
// source names the cluster differently.
func agree(name, from *string, got, source string) bool {
	if *name == "" {
		*name, *from = got, "from "+source
		return true
	}
	return *name == got
}

// sourceValue is the one value of source s on the nodes: found is false
// when no node has it; ok is false when nodes disagree or a value is not
// a clear name.
func sourceValue(nodes []nodeMeta, s nameSource) (v string, found, ok bool) {
	for _, n := range nodes {
		m := n.Labels
		if s.annotation {
			m = n.Annotations
		}
		raw, has := m[s.key]
		if !has {
			continue
		}
		clean, good := usableName(raw)
		if !good || (found && clean != v) {
			return "", false, false
		}
		v, found = clean, true
	}
	return v, found, true
}

// ownedTagValue is the cluster named by the kubernetes.io/cluster/<name>
// labels (value owned or shared). A node with two such labels, or nodes
// naming different clusters, is ambiguous.
func ownedTagValue(nodes []nodeMeta) (v string, found, ok bool) {
	for _, n := range nodes {
		got, count := "", 0
		for key, val := range n.Labels {
			suffix, has := strings.CutPrefix(key, ownedTagPrefix)
			if !has || (val != "owned" && val != "shared") {
				continue
			}
			got, count = suffix, count+1
		}
		if count == 0 {
			continue
		}
		clean, good := usableName(got)
		if count > 1 || !good || (found && clean != v) {
			return "", false, false
		}
		v, found = clean, true
	}
	return v, found, true
}

// clusterName is the cluster name shown to people: in messages, on the
// dashboard and in self-alerts. It is the name found at start, else the
// configured one. It is never part of an alert key or tracker marker:
// those keep runtime.Application().ClusterName, the configured name.
func (d *serverDeps) clusterName() string {
	if d.displayName != "" {
		return d.displayName
	}
	return d.runtime.Application().ClusterName
}
