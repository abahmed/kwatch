package compose

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
)

// eventPhrases say in a few plain words what a Kubernetes event reason
// means for the object it is about. A reason not listed is shown as is.
var eventPhrases = map[string]string{
	"FailedDeployModel":  "can't deploy its load balancer",
	"FailedBuildModel":   "can't build its load balancer",
	"FailedMount":        "can't mount a volume",
	"FailedAttachVolume": "can't attach a volume",
	"FailedCreate":       "can't create its pods",
	"FailedScheduling":   "can't be scheduled",
}

var (
	eventCount = regexp.MustCompile(
		`^Kubernetes reported (\w+) (\d+) times in (.+)$`)
	eventAgain = regexp.MustCompile(`^Kubernetes reported (\w+) again$`)
	hoursMins  = regexp.MustCompile(
		`\b(\w+) (hour|minute)s?\b`)
)

// windowWords write a window of time in a few characters.
var windowWords = strings.NewReplacer(
	"the last quarter hour", "15 min", "the last half hour", "30 min",
	"the last hour", "1h", "an hour", "1h", "a minute", "1 min",
)

// compactWindow shortens the time spans of a title: "two hours" is
// "2h", "seven minutes" is "7 min".
func compactWindow(text string) string {
	text = windowWords.Replace(text)
	return hoursMins.ReplaceAllStringFunc(text, func(m string) string {
		parts := hoursMins.FindStringSubmatch(m)
		n := wordNumber(parts[1])
		if n == 0 {
			return m
		}
		if parts[2] == "hour" {
			return strconv.Itoa(n) + "h"
		}
		return strconv.Itoa(n) + " min"
	})
}

// wordNumber is the number a spelled-out or written count stands for,
// or 0.
func wordNumber(word string) int {
	if n, err := strconv.Atoi(word); err == nil {
		return n
	}
	for n, w := range numberWords {
		if w == word && n > 0 {
			return n
		}
	}
	return 0
}

// shortWhat is what is wrong, in a few words, from the predicate of a
// title: an event count becomes "FailedDeployModel ×3 in 15 min" (with
// its plain meaning first when it has one) and a repeat becomes
// "failing again (2nd time in 2h)".
func shortWhat(rest string) string {
	rest = strings.TrimPrefix(rest, "failing: ")
	if m := eventCount.FindStringSubmatch(rest); m != nil {
		what := m[1] + " ×" + m[2] + " in " + compactWindow(m[3])
		if phrase, ok := eventPhrases[m[1]]; ok {
			return phrase + ": " + what
		}
		return what
	}
	if m := eventAgain.FindStringSubmatch(rest); m != nil {
		if phrase, ok := eventPhrases[m[1]]; ok {
			return phrase + " again"
		}
		return m[1] + " again"
	}
	if head, tail, found := strings.Cut(rest, ": "); found {
		return head + " (" + compactWindow(tail) + ")"
	}
	return compactWindow(rest)
}

// workloadList lists workloads for a digest line: names that repeat across
// namespaces show once with their namespaces, "web (shop, data)", or
// "web ×2" when there is no namespace to tell them apart. At most three
// entries are shown and the rest is counted as "+N" workloads.
func workloadList(ids []inventory.EntityID) string {
	byName := map[string][]string{}
	var names []string
	for _, id := range ids {
		if _, seen := byName[id.Name]; !seen {
			names = append(names, id.Name)
		}
		byName[id.Name] = append(byName[id.Name], id.Namespace)
	}
	sort.Strings(names)
	var shown []string
	covered := 0
	for _, name := range names {
		if len(shown) == maxRiskExamples {
			break
		}
		spaces := byName[name]
		covered += len(spaces)
		shown = append(shown, nameEntry(name, spaces))
	}
	text := strings.Join(shown, ", ")
	if rest := len(ids) - covered; rest > 0 {
		text += " +" + strconv.Itoa(rest)
	}
	return text
}

// nameEntry is one name and where it runs.
func nameEntry(name string, spaces []string) string {
	if len(spaces) == 1 {
		return name
	}
	sort.Strings(spaces)
	var distinct []string
	for _, ns := range spaces {
		if ns != "" && (len(distinct) == 0 ||
			distinct[len(distinct)-1] != ns) {
			distinct = append(distinct, ns)
		}
	}
	if len(distinct) < 2 {
		return name + " ×" + strconv.Itoa(len(spaces))
	}
	return name + " (" + strings.Join(distinct, ", ") + ")"
}
