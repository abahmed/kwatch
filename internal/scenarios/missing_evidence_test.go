package scenarios

import (
	"context"
	"slices"
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/replay"
)

// Evidence goes missing in real clusters: pods/log is not granted,
// metrics-server or the kubelet stats are not there, RBAC is trimmed to
// a few kinds. The engine may then say less, "unknown" included, but it
// must never state a root the labels do not name. This is the second
// mutation check of the gates: it removes a kind of evidence from every
// labelled scenario, replays it, and fails on any wrong root stated in
// any message.
var evidenceLosses = []struct {
	name string
	// without removes the evidence from a log, and hides from the
	// expectation the kinds a trimmed RBAC does not grant.
	without func(replay.Log) replay.Log
	hidden  []string
}{
	{name: "logs denied", without: withoutSources(logSources...)},
	{name: "no metrics", without: withoutSources(metricSources()...)},
	{name: "restricted RBAC", without: func(l replay.Log) replay.Log {
		return withoutKinds(l, kube.KindSecret, kube.KindConfigMap)
	}, hidden: []string{string(kube.KindSecret),
		string(kube.KindConfigMap)}},
}

// logSources are the observation sources that read container logs.
var logSources = []string{"crash-log", "probe-log", "log-sample"}

// metricSources are the sources of kubelet and metrics data.
func metricSources() []string {
	var out []string
	for _, source := range kube.EnrichmentSources() {
		if !slices.Contains(logSources, source) &&
			source != "service-probe" {
			out = append(out, source)
		}
	}
	return out
}

func withoutSources(sources ...string) func(replay.Log) replay.Log {
	return func(l replay.Log) replay.Log {
		kept := l.Entries[:0:0]
		for _, e := range l.Entries {
			if !slices.Contains(sources, e.Observation.Source) {
				kept = append(kept, e)
			}
		}
		l.Entries = kept
		return l
	}
}

func withoutKinds(l replay.Log, kinds ...inventory.Kind) replay.Log {
	kept := l.Entries[:0:0]
	for _, e := range l.Entries {
		if !slices.Contains(kinds, e.Observation.Entity.Kind) {
			kept = append(kept, e)
		}
	}
	l.Entries = kept
	return l
}

func TestMissingEvidenceNeverStatesAWrongRoot(t *testing.T) {
	t.Parallel()
	for _, loss := range evidenceLosses {
		t.Run(loss.name, func(t *testing.T) {
			t.Parallel()
			for _, s := range library() {
				if s.expect.Quiet || s.expect.SyncAfter > 0 {
					continue
				}
				intact := wrongWithout(t, s, identityLog, nil)
				wrong := wrongWithout(t, s, loss.without, loss.hidden)
				added := notIn(wrong, intact)
				known := toleratedWithoutEvidence[loss.name][s.expect.Name]
				switch {
				case len(added) > 0 && known == "":
					t.Errorf("%s with %s states a root the label does "+
						"not name: %v", s.expect.Name, loss.name, added)
				case len(added) == 0 && known != "":
					t.Errorf("%s with %s no longer states a wrong root: "+
						"remove it from toleratedWithoutEvidence",
						s.expect.Name, loss.name)
				}
			}
		})
	}
}

func identityLog(l replay.Log) replay.Log { return l }

// notIn lists the entries of a that b does not hold.
func notIn(a, b []string) []string {
	var out []string
	for _, id := range a {
		if !slices.Contains(b, id) {
			out = append(out, id)
		}
	}
	return out
}

// wrongWithout replays s with evidence removed and lists the roots its
// messages stated that the label does not allow.
func wrongWithout(
	t *testing.T, s scenario, without func(replay.Log) replay.Log,
	hidden []string,
) []string {
	t.Helper()
	log := without(s.generate(scenarioStart, ""))
	e := s.expect
	e.Hidden = append(slices.Clone(e.Hidden), hidden...)
	opts := e.options(log.Start)
	result, err := replay.Run(context.Background(), log,
		withSync(newDependencies(e.hiddenKinds()...), opts), opts)
	if err != nil {
		t.Fatal(err)
	}
	if e.RevisesRoot {
		return nil
	}
	_, overall := wrongRootsAnnounced(e, ownershipOf(log), result)
	return overall
}

// toleratedWithoutEvidence lists, per loss, the scenarios that state a
// root the label does not name once evidence is missing, which they did
// not with all of it. Each entry is a defect this gate found and this
// round did not fix: the engine states a cause it could only have
// seen with the evidence it lost. The test fails when an entry stops
// being true, so the list only shrinks. A new scenario or change must
// not add to it.
var toleratedWithoutEvidence = map[string]map[string]string{
	"restricted RBAC": {
		"missing-secret":                     hiddenCause,
		"borderline-init-after-rotation":     hiddenCause,
		"registry-login-missing-secret":      hiddenCause,
		"ambiguous-init-secret-edited":       hiddenCause,
		"ambiguous-cron-secret-edited":       hiddenCause,
		"ambiguous-image-and-config-config":  hiddenCause,
		"coupled-config-rejected-value":      hiddenCause,
		"cron-secret-edited-hours-before":    hiddenCause,
		"coupled-silent-config-fault":        hiddenCause,
		"coupled-silent-secret-fault-secret": hiddenCause,
		"coupled-silent-config-fault-state":  hiddenCause,
		"adv-bad-config-and-stuck-drain":     hiddenCause,
		"adv-bad-config-clean-drain":         hiddenCause,
		"adv-two-causes-stable":              hiddenCause,
		"adv-two-causes-six-hours":           hiddenCause,
	},
}

// hiddenCause is the defect behind every entry: the Secret or ConfigMap
// that is the cause is a kind kwatch cannot see, and the engine states
// the failing workload's rollout or own configuration instead of
// leaving the cause unknown beside the "can't see secrets" note.
const hiddenCause = "a hidden Secret or ConfigMap is the cause; the " +
	"workload is stated instead"
