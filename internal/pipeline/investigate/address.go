package investigate

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// ipv4Address is a dotted address with the port that may follow it.
// Group 1 is the address.
var ipv4Address = regexp.MustCompile(
	`\b(\d{1,3}(?:\.\d{1,3}){3})\b(?::\d{1,5}\b)?`)

// annotateAddresses appends " (name)" after each address the lookup
// knows: "dial tcp 10.0.3.4:5432: refused" becomes "dial tcp
// 10.0.3.4:5432 (db/postgres): refused". The log itself is not
// reworded. An address is named once per line, and not again when its
// name already follows it.
func annotateAddresses(
	text string, lookup func(ip string) (string, bool),
) string {
	var out strings.Builder
	done := map[string]bool{}
	last := 0
	for _, m := range ipv4Address.FindAllStringSubmatchIndex(text, -1) {
		start, end := m[0], m[1]
		ip := text[m[2]:m[3]]
		if insideLongerNumber(text, start, end) {
			continue
		}
		name, ok := lookup(ip)
		suffix := " (" + name + ")"
		if !ok || done[ip] || strings.HasPrefix(text[end:], suffix) {
			continue
		}
		done[ip] = true
		out.WriteString(text[last:end])
		out.WriteString(suffix)
		last = end
	}
	out.WriteString(text[last:])
	return out.String()
}

// insideLongerNumber reports whether text[start:end] is part of a longer
// dotted number such as the version "1.2.3.4.5", not an address.
func insideLongerNumber(text string, start, end int) bool {
	if start > 0 && text[start-1] == '.' {
		return true
	}
	return end+1 < len(text) && text[end] == '.' &&
		text[end+1] >= '0' && text[end+1] <= '9'
}

// addressOwners returns a lookup from an address to the Service
// (ClusterIP), Pod (pod IP) or Node (InternalIP) that owns it. It reads
// the model on first use, so a result with no address costs nothing.
func addressOwners(model inventory.Reader) func(string) (string, bool) {
	var owners map[string]string
	build := func() {
		owners = map[string]string{}
		for _, own := range []struct {
			kind inventory.Kind
			attr string
		}{
			{kube.KindNode, kube.AttrNodeIP},
			{kube.KindPod, kube.AttrPodIP},
			{kube.KindService, kube.AttrClusterIP},
		} {
			for _, id := range model.Entities(own.kind) {
				ip := attributeText(model, id, own.attr)
				if ip != "" {
					owners[ip] = ownerName(id)
				}
			}
		}
	}
	return func(ip string) (string, bool) {
		if owners == nil {
			build()
		}
		name, ok := owners[ip]
		return name, ok
	}
}

// ownerName is "namespace/name", or just the name for a Node.
func ownerName(id inventory.EntityID) string {
	if id.Namespace == "" {
		return id.Name
	}
	return id.Namespace + "/" + id.Name
}

// annotateResult names the addresses in a result's quoted text.
func annotateResult(r Result, model inventory.Reader) Result {
	lookup := addressOwners(model)
	out := Result{}
	for _, line := range r.Output {
		out.Output = append(out.Output, annotateAddresses(line, lookup))
	}
	for _, e := range r.Evidence {
		e.Text = annotateAddresses(e.Text, lookup)
		out.Evidence = append(out.Evidence, e)
	}
	return out
}
