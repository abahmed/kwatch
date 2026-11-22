package scenarios

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wordingTerms are the words the first message of a scenario must and
// must not carry (SCENARIO_GAPS_24.md). A term with "|" is satisfied by
// any of its parts. Terms in not match whole words, case-insensitively,
// outside quoted log text and the kubectl command; a trailing "*" makes
// a prefix. Checks run on the committed message golden, so a wording
// change shows up here as well as in the golden diff.
type wordingTerms struct {
	name     string
	contain  []string
	not      []string
	reminder []string
}

// wordingGlobalNot is banned in the first message of every scenario that
// has terms: hedges, forecasts, internal words and banned product words.
var wordingGlobalNot = []string{
	"might", "maybe", "possibly", "probably", "perhaps",
	"seems", "appears", "could be", "should be", "try to", "I think",
	"unclear",
	"will", "soon", "going to", "about to", "expected to", "eventually",
	"within the next", "in the coming", "guaranteed", "free forever", "best effort", "predict", "forecast", "trend",
	"rootcause", "root cause engine", "incident id", "tier", "heuristic",
	"confidence", "score", "edge", "graph", "informer", "reconcile",
	"signal", "evidence", "candidate", "suppressed", "dedup",
	"trial", "open source", "SLA", "24/7",
}

var wordingTable = []wordingTerms{
	{name: "wording-oom-limit-too-low",
		contain: []string{"api in shop", "OOMKilled|running out of memory",
			"256Mi",
			"Fix: raise the memory limit of container app in deployment/api",
			"kubectl logs deploy/api -c app -n shop --previous",
			"used more memory than allowed|memory limit is too low"},
		not: []string{"memory leak", "increase memory", "node"}},
	{name: "wording-image-typo",
		contain: []string{"worker is failing in billing",
			"registry.example/worker:v2.1O",
			"can't pull|cannot pull|cannot be pulled",
			"Fix: change the image of container",
			"kubectl describe pods worker- -n billing"},
		not: []string{"network", "registry is down", "credentials"}},
	// Moved from wording-missing-secret-mount (twin: missing-secret).
	{name: "missing-secret",
		contain: []string{"invoicer in billing", "secret \"stripe-api\"",
			"not found",
			"Fix: correct the envFrom reference|" +
				"Fix: create secret stripe-api in namespace billing"},
		not: []string{"permission", "RBAC"}},
	{name: "wording-bad-rollout-config-key",
		contain: []string{"the new version (api:2.3) can't start",
			"the previous version (api:2.2) is still serving",
			"missing key DB_PASSWORD_V2", "Fix:",
			"kubectl rollout status deployment/api -n shop"},
		not: []string{"rollback will"}},
	// Moved from wording-node-lost-pinned-database (twin:
	// node-lost-pinned-tenants).
	{name: "node-lost-pinned-tenants",
		contain: []string{"n1", "stopped reporting",
			"check the machine behind node n1"},
		not: []string{"restart pg", "delete the pod", "network issue"}},
	// Moved from wording-pvc-full (twin: claim-full-with-volume).
	{name: "claim-full-with-volume",
		contain: []string{"logstore in observability", "full|out of space",
			"50Gi",
			"Fix: expand persistentvolumeclaim/chunks|" +
				"Fix: raise the storage request of persistentvolumeclaim/chunks",
			"kubectl describe pvc chunks -n observability"},
		not: []string{"delete", "logs filling"}},
	{name: "wording-quota-forbids-pods",
		contain: []string{"compute in shop", "requests.cpu",
			"Fix: raise requests.cpu in resourcequota/compute|" +
				"lower the requests of workloads in namespace shop"},
		not: []string{"cluster is full", "no nodes", "capacity"}},
	{name: "wording-probe-port-mismatch",
		contain: []string{"web is failing in shop", "port 80|:80", "8080",
			"Fix: set the readiness probe of container",
			"kubectl rollout status deployment/web -n shop"},
		not: []string{"restart", "the app is slow", "increase timeout"}},
	// Moved from wording-cronjob-failing-secret-edit (twin:
	// cron-secret-edited-hours-before).
	{name: "cron-secret-edited-hours-before",
		contain: []string{"sync-key", "billing",
			"kubectl describe secret sync-key -n billing"},
		not: []string{"rotated by"}},
	{name: "wording-certificate-expired-handshake",
		contain: []string{"Affected: payments", "bank-client-tls",
			"expired",
			"Fix: renew the certificate in secret/bank-client-tls"},
		not: []string{"will expire", "almost"}},
	// Moved from wording-webhook-down-fail-closed (twin:
	// webhook-no-endpoints).
	{name: "webhook-no-endpoints",
		contain: []string{"policy-webhook", "no ready pods|no ready endpoints",
			"shop"},
		not: []string{"webhook is broken", "disable the webhook"}},
	// Moved from wording-unschedulable-taint (twin: unschedulable-taint).
	{name: "unschedulable-taint",
		contain: []string{"report in analytics", "dedicated: gpu",
			"Fix: add the toleration the node taint needs to deployment/report",
			"kubectl describe pods report- -n analytics"},
		not: []string{"add nodes", "scale the cluster", "not enough CPU"}},
	{name: "wording-registry-rate-limited",
		contain: []string{"docker.io", "rate limiting pulls", "shop",
			"billing", "kubectl describe pods api- -n billing"},
		not: []string{"retry later", "wait", "usually clears"}},
	// Moved from wording-ingress-backend-missing (twin:
	// ingress-backend-missing).
	{name: "ingress-backend-missing",
		contain: []string{"Ingress shop in shop", "service web-v2",
			"does not exist", "Fix: point ingress/shop at a service that exists",
			"kubectl describe ingress shop -n shop"},
		not: []string{"502", "controller", "load balancer"}},
	{name: "wording-first-message-ongoing-chronic",
		not: []string{"again", "still no fix", "getting worse", "escalat*",
			"urgent"}},
	{name: "long-page-reminder",
		contain:  []string{"Node n1"},
		reminder: []string{"still down"}},
}

var (
	goldenHeader = regexp.MustCompile(`^\d{2}:\d{2}:\d{2} .+$`)
	quotedText   = regexp.MustCompile("\"[^\"]*\"|`[^`]*`")
	commandTail  = regexp.MustCompile(`run kubectl.*$`)
	fixSentence  = regexp.MustCompile(`Fix: [^.]*\.`)
	severity     = map[rune]bool{'🔴': true, '🟠': true, '🟡': true}
)

// goldenNotes returns the full note of every message of a golden file.
func goldenNotes(golden string) []string {
	lines := strings.Split(golden, "\n")
	var notes []string
	for i, line := range lines {
		if goldenHeader.MatchString(line) && i+1 < len(lines) {
			notes = append(notes, lines[i+1])
		}
	}
	return notes
}

// hasTerm reports whether text carries term, or any "|" part of it.
func hasTerm(text, term string) bool {
	for _, part := range strings.Split(term, "|") {
		if strings.Contains(text, part) {
			return true
		}
	}
	return false
}

// bannedWord is the pattern of a not term.
func bannedWord(term string) *regexp.Regexp {
	word := regexp.QuoteMeta(strings.TrimSuffix(term, "*"))
	if strings.HasSuffix(term, "*") {
		return regexp.MustCompile(`(?i)\b` + word)
	}
	return regexp.MustCompile(`(?i)\b` + word + `\b`)
}

// prose is the note without quoted log text and the kubectl command.
func prose(note string) string {
	return commandTail.ReplaceAllString(quotedText.ReplaceAllString(note, ""), "")
}

// checkBanned reports every banned word and stray emoji of note.
func checkBanned(t *testing.T, note string, banned []string) {
	t.Helper()
	text := prose(note)
	for _, term := range banned {
		assert.False(t, bannedWord(term).MatchString(text),
			"message must not contain %q: %s", term, note)
	}
	for i, r := range []rune(text) {
		if unicode.Is(unicode.So, r) && !severity[r] {
			t.Errorf("message has the emoji %q at %d: %s", r, i, note)
		}
	}
}

func TestWordingTermsNamesAreScenarios(t *testing.T) {
	t.Parallel()
	known := map[string]bool{}
	for _, s := range library() {
		known[s.expect.Name] = true
	}
	for _, terms := range wordingTable {
		assert.True(t, known[terms.name], "unknown scenario %s", terms.name)
	}
}

// TestWordingTerms holds the first message of the wording scenarios to
// their must_contain and must_not_contain terms.
func TestWordingTerms(t *testing.T) {
	t.Parallel()
	for _, terms := range wordingTable {
		t.Run(terms.name, func(t *testing.T) {
			t.Parallel()
			raw, err := os.ReadFile(messagesPath(terms.name))
			if err != nil {
				t.Skipf("no message golden yet: %v", err)
			}
			notes := goldenNotes(string(raw))
			require.NotEmpty(t, notes, "golden has no message")
			first := notes[0]
			for _, term := range terms.contain {
				assert.True(t, hasTerm(first, term),
					"message must contain %q: %s", term, first)
			}
			checkBanned(t, first, wordingGlobalNot)
			checkBanned(t, first, terms.not)
			if len(terms.reminder) == 0 {
				return
			}
			require.Len(t, notes, 2, "first message and one reminder")
			for _, term := range terms.reminder {
				assert.True(t, hasTerm(notes[1], term),
					"reminder must contain %q: %s", term, notes[1])
			}
			fix := fixSentence.FindString(first)
			assert.Contains(t, notes[1], fix,
				"reminder keeps the Fix line of the first message")
			checkBanned(t, notes[1], terms.not)
		})
	}
}
