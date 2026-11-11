package scenarios

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/detectors"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification/compose"
	"github.com/abahmed/kwatch/internal/pipeline"
	"github.com/abahmed/kwatch/internal/replay"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// communityScenarios is the subset of the library replayed as the
// Community edition: crash loops, OOM, Pending pods, node loss, a storm
// grouped under one node, a missing Secret, a start mid-outage,
// resolves, grouped recoveries, close rival causes, a log line only Pro
// quotes, and scenarios only Pro detectors see. The full library runs
// as Pro; this run catches a Community regression without doubling the
// suite's time.
var communityScenarios = []string{
	"crash-loop-for-hours",
	"bad-rollout",
	"oom-limit-too-low",
	"unschedulable-taint",
	"node-stays-down",
	"node-lost-notready",
	"missing-secret",
	"start-mid-outage-missing-secret",
	"fix-attempt-works",
	"crash-loop-during-node-replacement",
	"namespace-outage-groups",
	"spot-node-removed",
	"adv-two-causes-stable",
	"hpa-at-max",
	"certificate-expired-unused",
	"ready-never",
	"cache-unreachable-at-start",
	"readiness-error-logged",
}

// proDetectorOnly are the community scenarios whose failure only a Pro
// detector finds. Community sends no alert for them; at most the Pro
// checks note says how many issues it withheld.
var proDetectorOnly = map[string]bool{
	"hpa-at-max": true, "certificate-expired-unused": true,
	"ready-never": true, "readiness-error-logged": true,
}

// withheldReason is the decision reason of the Pro checks note.
const withheldReason = "pro checks"

// proOnlyParts are words only a Pro message carries: incident ties,
// recent changes, impact, investigation output, quotes of a log, the
// dashboard, the close rival cause, the fix (the command and the
// setting) and how often it happened before.
var proOnlyParts = []string{
	"Caused by", "Likely caused by", "Changed just before it recovered",
	"pods were affected", "Evidence", "Logs:", "Last log", "dashboard",
	"two possible causes", "Proof:", "Confidence", "Fix:",
	", run kubectl", "time this week", "It fails every",
	"from its log",
}

// communityCauseLeaks are, for each scenario whose Pro run names a
// cause, the cause's object names, images and wording. A Community
// message tells only the cause's category ("Most likely cause: a recent
// rollout."), so it shows none of them.
var communityCauseLeaks = map[string][]string{
	"crash-loop-for-hours": {"regional"},
	"bad-rollout": {
		"registry.example.com", "payments:2.", "rolled out"},
	"fix-attempt-works": {
		"registry.example.com", "payments:2.", "rolled out"},
	"oom-limit-too-low": {"memory limit is too low"},
	"unschedulable-taint": {
		"untolerated taint", "rejects every node"},
	"missing-secret":                  {"stripe-api"},
	"start-mid-outage-missing-secret": {"invoicer-creds"},
	"spot-node-removed":               {"node n3", "was removed"},
	"adv-two-causes-stable":           {"checkout-flags", "checkout-keys"},
	"cache-unreachable-at-start": {"cannot connect", "Redis address",
		"from its log"},
}

// causeTeaser starts the Community sentence that tells the category.
var causeTeaser = regexp.MustCompile(`(Most likely|Possible) cause: `)

// proLink is in the one Pro line a Community message may carry.
const proLink = "kwatch.dev/pro"

func communityMessagesPath(name string) string {
	return filepath.Join("testdata", "messages-community", name+".golden")
}

// communityDependencies wires the engine the way internal/app does for
// the Community edition: only entitled detectors, the Community
// lifecycle (no flapping, paging, reminders or hand-overs), no runbooks
// and the Community writer.
func communityDependencies(hidden []inventory.Kind) pipeline.Dependencies {
	e := feature.CommunityEntitlements()
	neverSynced := kindSet(hidden)
	synced := func(kind inventory.Kind) bool { return !neverSynced[kind] }
	withheld := feature.NewWithheld()
	model := inventory.NewModel(inventory.Options{})
	return pipeline.Dependencies{
		Entitlements: e,
		Model:        model,
		Detectors: detection.NewRegistry(synced,
			detectors.Entitled(e, withheld)...),
		Withheld: withheld,
		Incidents: incident.NewManager(incident.Config{
			Verifiable: synced,
			NoFlapping: true, NoPaging: true,
			NoReminders: true, NoHandover: true,
			NoJournalDetail: true,
			IDNonce:         "5c3e",
		}, explain.NewSolver()),
		Synced:  synced,
		Writer:  compose.NewWriter(scenarioCluster, nil).ForEdition(e, false),
		Store:   &memoryStore{},
		InScope: pipeline.IncidentScope(model, defaultScope()),
	}
}

func replayCommunity(t *testing.T, name string) replay.Result {
	t.Helper()
	log, expect, err := readScenarioFrom(labelledDir, name)
	if err != nil {
		t.Fatal(err)
	}
	opts := expect.options(log.Start)
	if opts.SyncAt.IsZero() {
		// Community holds every announcement until the sources sync,
		// as the application's informers do at once; a replay without
		// a sync of its own syncs at the log's first entry.
		opts.SyncAt = log.Start
	}
	deps := withSync(communityDependencies(expect.hiddenKinds()), opts)
	result, err := replay.Run(context.Background(), log, deps, opts)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// TestCommunityScenarioMessages replays the Community subset, keeps the
// wording of every message, and checks that no Pro-only part shows.
// Regenerate one golden with:
//
//	go test ./internal/scenarios \
//	  -run 'TestCommunityScenarioMessages/name$' -update
func TestCommunityScenarioMessages(t *testing.T) {
	requireScopedUpdate(t)
	runParallelUnlessUpdating(t)
	for _, name := range communityScenarios {
		t.Run(name, func(t *testing.T) {
			runParallelUnlessUpdating(t)
			result := replayCommunity(t, name)
			checkNoProParts(t, result)
			checkProLineOnFirstAlertOnly(t, result)
			checkCauseCategoryOnly(t, name, result)
			if proDetectorOnly[name] {
				checkOnlyWithheldNote(t, result)
			}
			compareCommunityGolden(t, name, renderNotes(result))
		})
	}
}

// checkOnlyWithheldNote fails on any message but the Pro checks note.
func checkOnlyWithheldNote(t *testing.T, result replay.Result) {
	t.Helper()
	for i, m := range result.Messages {
		if result.Decisions[i].Reason != withheldReason {
			t.Errorf("a Pro detector alerted in Community: %s", m.Note)
		}
	}
}

func checkNoProParts(t *testing.T, result replay.Result) {
	t.Helper()
	for _, m := range result.Messages {
		for _, part := range proOnlyParts {
			if strings.Contains(m.Note, part) {
				t.Errorf("Community note shows %q: %s", part, m.Note)
			}
		}
		if m.Confidence != "" {
			t.Errorf("Community note states confidence %q", m.Confidence)
		}
	}
}

// checkProLineOnFirstAlertOnly fails on a Community update or resolve
// that carries the Pro line, and on a message with more than one: only
// an issue's first alert mentions Pro, once.
func checkProLineOnFirstAlertOnly(t *testing.T, result replay.Result) {
	t.Helper()
	for i, m := range result.Messages {
		d := result.Decisions[i]
		n := strings.Count(m.Note, proLink)
		if d.Reason == withheldReason {
			continue
		}
		if d.Action != incident.Announce && n > 0 {
			t.Errorf("Community %s repeats the Pro line: %s",
				d.Reason, m.Note)
		}
		if n > 1 {
			t.Errorf("Community note has %d Pro lines: %s", n, m.Note)
		}
	}
}

// checkCauseCategoryOnly fails on a Community message that names its
// cause, and on a scenario whose cause Pro names but no Community message
// tells the category of.
func checkCauseCategoryOnly(
	t *testing.T, name string, result replay.Result,
) {
	t.Helper()
	leaks, caused := communityCauseLeaks[name]
	leaks = append(leaks, "shortly before", " because ")
	teased := false
	for _, m := range result.Messages {
		teased = teased || causeTeaser.MatchString(m.Note)
		for _, leak := range leaks {
			if strings.Contains(m.Note, leak) {
				t.Errorf("Community note names the cause %q: %s",
					leak, m.Note)
			}
		}
	}
	if teased != caused {
		t.Errorf("Community says %q: %v, Pro names a cause: %v",
			causeTeaser.String(), teased, caused)
	}
}

func compareCommunityGolden(t *testing.T, name, got string) {
	t.Helper()
	path := communityMessagesPath(name)
	if *update {
		writeGolden(t, path, got)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden file; run with -update: %v", err)
	}
	if got != string(want) {
		t.Errorf("messages changed\n got:\n%s\nwant:\n%s", got, want)
	}
}

// TestCommunityPolicyIsTheDefault guards the wiring above: the default
// configuration has no severity overrides or runbooks that Community
// would have dropped, so only the edition differs from the Pro run.
func TestCommunityPolicyIsTheDefault(t *testing.T) {
	t.Parallel()
	policy := config.RuntimeConfigFor(config.DefaultConfig()).Policy()
	if len(policy.SeverityByReason())+len(policy.SeverityByOwnerKind())+
		len(policy.Runbooks()) > 0 {
		t.Error("the default policy sets Pro options; drop them above")
	}
}
