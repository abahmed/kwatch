package kube

import (
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
)

// maxProvisionerWorkloads bounds the workloads one provisioner lists.
const maxProvisionerWorkloads = 5

// inTreeProvisioner starts the name of a provisioner built into
// Kubernetes: the controller manager runs it, so no workload serves it.
const inTreeProvisioner = "kubernetes.io/"

// Evidence levels, weakest first. A workload is listed only at the
// strongest level any workload reaches.
const (
	evidenceNone = iota
	evidenceTokens
	evidenceFullName
)

// ProvisionerWorkloads are the Deployments and StatefulSets that serve
// the StorageClass provisioner called name, sorted, at the strongest
// evidence found. No driver name is built in. The strongest evidence is
// the full provisioner name in the workload's name or image, or in a
// driver-name argument of one of its pods. The weaker one is word
// tokens: the workload's name or image holds the provisioner's first
// distinctive word and says "csi" or "provisioner" ("aws-ebs-csi-driver"
// for "ebs.csi.example.com"). Words are whole tokens, never fragments,
// and generic ones ("csi", "driver", "file") do not count. A workload
// scaled to zero is matched by its name alone. Built-in provisioners
// and node plugins have no match.
func ProvisionerWorkloads(
	model inventory.Reader, name string,
) []inventory.EntityID {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || strings.HasPrefix(name, inTreeProvisioner) {
		return nil
	}
	best := evidenceNone
	var out []inventory.EntityID
	for _, kind := range []inventory.Kind{KindDeployment,
		KindStatefulSet} {
		for _, workload := range model.Entities(kind) {
			level := workloadEvidence(model, workload, name)
			switch {
			case level == evidenceNone || level < best:
			case level > best:
				best, out = level, []inventory.EntityID{workload}
			default:
				out = append(out, workload)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].String() < out[j].String()
	})
	if len(out) > maxProvisionerWorkloads {
		out = out[:maxProvisionerWorkloads]
	}
	return out
}

// workloadEvidence is the strongest evidence that workload serves the
// provisioner, from its own name and its pods' images and arguments.
func workloadEvidence(
	model inventory.Reader, workload inventory.EntityID, name string,
) int {
	if isNodePlugin(workload.Name) {
		return evidenceNone
	}
	names := []string{strings.ToLower(workload.Name)}
	level := evidenceNone
	for _, pod := range OwnedPods(model, workload) {
		for _, image := range model.Related(pod, inventory.Pulls,
			inventory.Outgoing) {
			names = append(names, strings.ToLower(image.Name))
		}
		if e, ok := model.Entity(pod); ok && namesIn(
			attrString(e, AttrDriverNames), name) {
			level = evidenceFullName
		}
	}
	for _, candidate := range names {
		switch {
		case isNodePlugin(candidate):
		case strings.Contains(candidate, name):
			return evidenceFullName
		case level == evidenceNone && tokensMatch(candidate, name):
			level = evidenceTokens
		}
	}
	return level
}

// namesIn reports name as one of the space-separated names.
func namesIn(names, name string) bool {
	for _, have := range strings.Fields(names) {
		if have == name {
			return true
		}
	}
	return false
}

func attrString(e inventory.Entity, name string) string {
	if a, ok := e.Attribute(name); ok {
		return a.Value.AsText()
	}
	return ""
}

// genericWords say nothing about which driver a name is for: they are
// parts of every driver or domain name.
var genericWords = map[string]bool{
	"csi": true, "driver": true, "file": true, "io": true, "com": true,
	"org": true, "net": true, "k8s": true, "kubernetes": true,
	"storage": true, "disk": true, "blob": true, "cloud": true,
	"volume": true, "plugin": true, "provisioner": true, "app": true,
}

// words splits a name into lowercase tokens at ".", "-", "/", "_", ":"
// and "@".
func words(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return strings.ContainsRune(".-/_:@", r)
	})
}

// hasWord reports a token of s equal to word.
func hasWord(s []string, word string) bool {
	for _, w := range s {
		if w == word {
			return true
		}
	}
	return false
}

// firstDistinctive is the first token of the provisioner that is neither
// generic nor shorter than three letters, or "".
func firstDistinctive(provisioner string) string {
	for _, w := range words(provisioner) {
		if len(w) >= 3 && !genericWords[w] {
			return w
		}
	}
	return ""
}

// tokensMatch reports a workload or image name that holds the
// provisioner's first distinctive token and says "csi" or "provisioner".
func tokensMatch(candidate, provisioner string) bool {
	key := firstDistinctive(provisioner)
	tokens := words(candidate)
	return key != "" && hasWord(tokens, key) &&
		(hasWord(tokens, "csi") || hasWord(tokens, "provisioner"))
}

// isNodePlugin reports a workload or image named as the node plugin of a
// driver, which shares the driver's image but does not provision.
func isNodePlugin(name string) bool {
	return hasWord(words(name), "node")
}
