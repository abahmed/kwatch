package compose

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/abahmed/kwatch/internal/inventory"
)

// predicate turns a detector summary into what follows the subject:
// "Node is low on memory" becomes "is low on memory".
func predicate(id inventory.EntityID, summary string) string {
	summary = humanizeText(firstClause(summary))
	for _, prefix := range subjectWords(id) {
		if hasPrefixFold(summary, prefix+" ") {
			summary = summary[len(prefix)+1:]
			break
		}
	}
	summary = rewriteSummary(lowerFirst(summary))
	for _, verb := range subjectlessVerbs {
		if hasPrefixFold(summary, verb) {
			return lowerFirst(summary)
		}
	}
	if strings.TrimSpace(summary) == "" {
		return "is failing"
	}
	return "is failing: " + lowerFirst(summary)
}

// subjectWords are the ways a detector summary may start with its own
// subject: the kind, its plain word, or the singleton's name.
func subjectWords(id inventory.EntityID) []string {
	words := []string{string(id.Kind), kindWord(id.Kind)}
	if name, ok := singletonNames[id]; ok {
		words = append(words, name, strings.TrimPrefix(name, "the "))
	}
	return append(words, extraSubjectWords[id.Kind]...)
}

// extraSubjectWords are other words detectors use for a kind.
var extraSubjectWords = map[inventory.Kind][]string{
	"validatingwebhookconfiguration": {"admission webhook"},
	"mutatingwebhookconfiguration":   {"admission webhook"},
	"resourcequota":                  {"namespace quota"},
	"external-endpoint":              {"endpoint"},
}

// subjectlessVerbs start summaries that leave the subject out
// ("Reports phase Failed", "Has been pending").
var subjectlessVerbs = []string{"reports ", "has ", "is ", "keeps ",
	"cannot ", "can't ", "references ", "routes ", "calls ", "stopped ",
	"was ", "allows ", "uses ", "needs ", "selects ", "does ", "targets ",
	"did ", "refused ", "could "}

func hasPrefixFold(text, prefix string) bool {
	return len(text) >= len(prefix) &&
		strings.EqualFold(text[:len(prefix)], prefix)
}

// summaryRewrite turns detector wording into plain English. Rewrites
// see the summary after its subject was removed.
type summaryRewrite struct {
	match   *regexp.Regexp
	replace string
}

// summaryRewrites are applied in order; the first match wins.
var summaryRewrites = []summaryRewrite{
	{regexp.MustCompile(`^keeps crashing and is restarting with back-off$`),
		"keeps crashing"},
	{regexp.MustCompile(`^(?i:init) container keeps crashing.*$`),
		"has an init container that keeps crashing"},
	{regexp.MustCompile(`^(?i:sidecar) container keeps crashing.*$`),
		"has a sidecar container that keeps crashing"},
	{regexp.MustCompile(
		`^is killed for exceeding its memory limit and restarting$`),
		"keeps running out of memory"},
	{regexp.MustCompile(`^CPU is throttled (.+) of the time$`),
		"is throttled on CPU $1 of the time"},
	{regexp.MustCompile(`^TLS certificate (.+)$`),
		"has a TLS certificate that $1"},
	{regexp.MustCompile(`^backend (\S+) has no ready pods$`),
		"has no ready pods behind service $1"},
	{regexp.MustCompile(`^0 of (\d+) replicas are ready$`),
		"has no ready replicas (0 of $1)"},
	{regexp.MustCompile(`^(\d+) of (\d+) replicas are ready$`),
		"has only $1 of $2 replicas ready"},
	{regexp.MustCompile(`^cannot be scheduled \(\w+\) for (.+)$`),
		"has waited $1 for a node"},
	{regexp.MustCompile(
		`^(?i:its) controller has not processed the latest change$`),
		"is waiting for its controller to process the latest change"},
	{regexp.MustCompile(
		`^stopped reporting \(kubelet unreachable\) for (.+)$`),
		"has not reported for $1"},
	{regexp.MustCompile(`^schedule is not a valid cron expression$`),
		"has an invalid schedule"},
	{regexp.MustCompile(`^(?i:kubelet|controller) cannot (.+)$`),
		"cannot $1"},
	{regexp.MustCompile(`^crossed an eviction threshold$`),
		"has crossed its eviction threshold"},
	{regexp.MustCompile(`^selects no pods since its \S+ changed$`),
		"selects no pods"},
	{regexp.MustCompile(
		`^(?:HPA )?\S+ targets (.+), which does not exist\.?$`),
		"targets $1, which does not exist"},
	{regexp.MustCompile(`^kwatch could not reach the kubelet on node \S+ ` +
		`(\d+) times in the last (\d+) hours$`),
		"has a kubelet that kwatch could not reach $1 times in the " +
			"last $2 hours"},
	{regexp.MustCompile(`^last run failed (.+)$`),
		"has a last run that failed $1"},
	{regexp.MustCompile(`^name does not resolve$`),
		"has a name that does not resolve"},
	{regexp.MustCompile(`^DNS lookup failed$`),
		"has a DNS lookup that failed"},
}

// conditionPattern matches "reports Ready=False (InvalidBrokerConfig)".
var conditionPattern = regexp.MustCompile(
	`^(?i:reports) (\w+)=(False|True)(?: \((\w+)\))?$`)

// conditionWords say what a condition that went wrong means.
var conditionWords = map[string]string{
	"Ready": "is not ready", "Available": "is not available",
	"Accepted": "is not accepted", "Programmed": "is not programmed",
	"Established": "is not established", "ResolvedRefs": "has " +
		"references that do not resolve",
	"Synced": "is not synced", "Reconciled": "is not reconciled",
	"Stalled": "is stalled", "Degraded": "is degraded",
}

func rewriteSummary(summary string) string {
	if m := conditionPattern.FindStringSubmatch(summary); m != nil {
		if words, ok := conditionWords[m[1]]; ok {
			if m[3] != "" {
				words += " (" + splitCamel(m[3]) + ")"
			}
			return words
		}
	}
	for _, r := range summaryRewrites {
		if r.match.MatchString(summary) {
			return r.match.ReplaceAllString(summary, r.replace)
		}
	}
	return summary
}

// splitCamel writes an API reason as words: "InvalidBrokerConfig"
// becomes "invalid broker config". Runs of capitals stay together, so
// "TLSError" becomes "TLS error".
func splitCamel(text string) string {
	runes := []rune(text)
	var words []string
	start := 0
	for i := 1; i < len(runes); i++ {
		upper := unicode.IsUpper(runes[i])
		nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
		if upper && (unicode.IsLower(runes[i-1]) || nextLower) {
			words = append(words, string(runes[start:i]))
			start = i
		}
	}
	words = append(words, string(runes[start:]))
	for i, word := range words {
		words[i] = lowerFirst(word)
	}
	return strings.Join(words, " ")
}

// firstClause drops what follows a semicolon; detector summaries put
// commentary there ("low on memory; pods may be evicted").
func firstClause(text string) string {
	before, _, _ := strings.Cut(text, ";")
	return strings.TrimSpace(before)
}

// pluralPredicate turns a predicate about one subject into one about
// several: "has not been ready" becomes "have not been ready".
func pluralPredicate(text string) string {
	first, rest, _ := strings.Cut(text, " ")
	if plural, ok := pluralVerbs[first]; ok {
		first = plural
	}
	rest = outsideQuotes(" "+rest, func(part string) string {
		return strings.ReplaceAll(part, " its ", " their ")
	})
	return first + rest
}

var pluralVerbs = map[string]string{
	"is": "are", "has": "have", "keeps": "keep", "reports": "report",
	"was": "were", "references": "reference", "uses": "use",
	"needs": "need", "allows": "allow", "calls": "call", "routes": "route",
	"selects": "select", "runs": "run",
}

// nowState puts "now" into a predicate: "is now not available", "now
// keeps crashing", "now has no ready replicas (0 of 1)". A state that
// says "only" reads badly after "now", so "now has only 1 of 3
// replicas ready" is written "now has 1 of 3 replicas ready".
func nowState(state string) string {
	if rest, ok := strings.CutPrefix(state, "is "); ok {
		return "is now " + rest
	}
	return "now " + strings.Replace(state, "has only ", "has ", 1)
}
