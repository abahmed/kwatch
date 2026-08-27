package format

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// ShortImage reduces an image reference to what a reader needs: the image
// name with its tag or digest. The registry host and the repository path
// say where it came from, not what it is, and they triple the line length:
//
//	registry.example.com/team/api:1.2.0        → api:1.2.0
//	ghcr.io/org/tool@sha256:abcdef0123456789…  → tool@abcdef012345
//	nginx:1.25                                 → nginx:1.25
//
// A digest is cut to its first 12 characters, the way container tools show it.
func ShortImage(image string) string {
	image = strings.TrimSpace(image)
	if image == "" {
		return ""
	}
	// Keep only the last path segment: everything before it is registry
	// host and repository path.
	if i := strings.LastIndex(image, "/"); i >= 0 {
		image = image[i+1:]
	}
	if i := strings.Index(image, "@sha256:"); i > 0 {
		digest := image[i+len("@sha256:"):]
		if len(digest) > 12 {
			digest = digest[:12]
		}
		image = image[:i] + "@" + digest
	}
	return image
}

// ShortNode trims a node name to its hostname. Cloud node names carry a
// domain that says nothing a reader needs and triples the line length:
func ShortNode(node string) string {
	node = strings.TrimSpace(node)
	if i := strings.Index(node, "."); i > 0 {
		return node[:i]
	}
	return node
}

// JoinNames lists up to max names, sorted, and summarises the rest as
// "+N more". Names are deduplicated first.
func JoinNames(names []string, max int) string {
	seen := make(map[string]bool, len(names))
	uniq := make([]string, 0, len(names))
	for _, n := range names {
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		uniq = append(uniq, n)
	}
	sort.Strings(uniq)
	if max <= 0 || len(uniq) <= max {
		return strings.Join(uniq, ", ")
	}
	return fmt.Sprintf(
		"%s +%d more",
		strings.Join(uniq[:max], ", "),
		len(uniq)-max,
	)
}

// Plural returns "n word" or "n words".
func Plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// Duration renders d for people: "5s", "2m30s", "2h15m". Sub-second detail is
// dropped — an alert about a pod that has been stuck for 12 minutes does not
// need milliseconds.
func Duration(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}
