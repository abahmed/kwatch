package compose

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// restartNoise matches the kubelet's own restart message, which says
// nothing about why the container fails.
var restartNoise = regexp.MustCompile(
	`^(?i:back-off) \S+ restarting failed container`)

// pullNoise matches the kubelet's pull back-off, which only repeats the
// image name.
var pullNoise = regexp.MustCompile(`^(?i:back-off) pulling image `)

// errorIntro introduces the lead finding's quoted error. Under a group
// lead the error belongs to one of the group's nodes, not to the zone or
// pool the sentence is about.
func errorIntro(f caseFacts) string {
	if leadIsGroup(f) {
		if f.lead.Entity.Kind == kube.KindNode {
			return "One of its nodes reports"
		}
		return "One of them reports"
	}
	return errorVerb(f.lead.Entity)
}

// errorVerb introduces a quoted error: a program "fails with" it, a
// node or service "reports" it.
func errorVerb(id inventory.EntityID) string {
	if id.Kind == kube.KindContainer || id.Kind == kube.KindPod {
		return "It fails with"
	}
	return "It reports"
}

// quoted puts a clipped error in quotes. Everything between them is what
// the pod wrote, full stop included; the quote runes (see quotes.go) tell
// the renderers where it ends, whatever double quotes it holds.
func quoted(text string) string {
	return markQuote(clip(text))
}

func lastLine(lines []string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}

// evidence is the first non-empty value under one of labels.
func evidence(s detection.Finding, labels ...string) string {
	for _, label := range labels {
		for _, e := range s.Evidence {
			if e.Label == label && strings.TrimSpace(e.Value) != "" {
				return strings.TrimSpace(e.Value)
			}
		}
	}
	return ""
}

// fieldWords names a changed field: "the image", or "key DB_HOST" for
// config data.
func fieldWords(path string) string {
	if key, ok := dataKey(path); ok {
		return "key " + key
	}
	return "the " + lastSegment(path)
}

// fieldNoun names a changed field without an article: "schedule", or
// "key DB_HOST".
func fieldNoun(path string) string {
	return strings.TrimPrefix(fieldWords(path), "the ")
}

// dataKey is the key of a config data path: "app.yaml" for
// "data.app.yaml". Keys may hold dots, so the whole rest is the key.
func dataKey(path string) (string, bool) {
	for _, prefix := range []string{"data.", "stringData.", "binaryData."} {
		if key, ok := strings.CutPrefix(path, prefix); ok {
			return key, true
		}
	}
	return "", false
}

// lastSegment is the last part of a field path: "image" for
// "spec.template.spec.containers[0].image".
func lastSegment(path string) string {
	if i := strings.LastIndexAny(path, ".]"); i >= 0 && i+1 < len(path) {
		return path[i+1:]
	}
	return path
}
