package replay_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/replay"
)

var secretWords = []string{
	"shop", "payments", "n1", "registry.corp", "2.3", "alice",
	"DB_PASSWORD", "panic", "prod", "argocd", "acme-operator",
	"hotfix", "acme-billing", "9f3a-secret", "acme-origin",
}

// mustSanitizer builds a sanitizer or fails the test.
func mustSanitizer(
	t *testing.T, opts replay.SanitizeOptions,
) *replay.Sanitizer {
	t.Helper()
	sanitizer, err := replay.NewSanitizer(opts)
	if err != nil {
		t.Fatal(err)
	}
	return sanitizer
}

func sensitiveObservations() []inventory.Observation {
	at := time.Date(2026, 9, 29, 14, 0, 0, 0, time.UTC)
	pod := inventory.CoreID("pod", "shop", "payments-1")
	container := inventory.CoreID("container", "shop", "payments-1/app")
	return []inventory.Observation{
		{Kind: inventory.Observed, Source: "kubernetes", At: at,
			Entity: container, UID: "uid-payments",
			Attributes: map[string]inventory.Value{
				"image":        inventory.Text("registry.corp/payments:2.3"),
				"reason":       inventory.Text("CrashLoopBackOff"),
				"last.message": inventory.Text("panic: DB_PASSWORD"),
				"restarts":     inventory.Number(4),
				"ready":        inventory.Bool(false),
				"since":        inventory.Time(time.Time{}),
				"node.host":    inventory.Text("n1"),
			}},
		{Kind: inventory.Observed, Source: "kubernetes", At: at,
			Entity: pod, Attributes: map[string]inventory.Value{
				"labels": inventory.Text("app=payments,env=prod"),
			}},
		{Kind: inventory.Observed, Source: "kubernetes", At: at,
			Entity: inventory.CoreID("service", "shop", "payments"),
			Attributes: map[string]inventory.Value{
				"selector": inventory.Text("app=payments,env!=prod," +
					"tier in (payments,shop)"),
			}},
		{Kind: inventory.Related, Source: "kubernetes", At: at,
			Entity: pod, Relation: inventory.RunsOn,
			Targets: []inventory.EntityID{
				inventory.CoreID("node", "", "n1"),
			}},
		{Kind: inventory.Changed, Source: "kubernetes", At: at,
			Entity: pod, Change: inventory.Change{
				Entity: pod, At: at, Actor: "alice", Revision: "3",
				App: "argocd/payments", Cause: "hotfix for acme-billing",
				Fields: []inventory.FieldChange{
					{Path: "containers[app].image",
						Before: "registry.corp/payments:2.2",
						After:  "registry.corp/payments:2.3"},
					{Path: "spec.note", Before: "was prod",
						After: "now shop"},
				},
			}},
		{Kind: inventory.Noted, Source: "events", At: at, Entity: pod,
			Note: inventory.Note{
				At: at, Source: "acme-operator", Reason: "BackOff",
				Message: "Back-off restarting payments", Count: 2,
				UID: "uid-9f3a-secret", Origin: "events/acme-origin-77",
			}},
	}
}

func TestSanitizerHidesIdentifyingData(t *testing.T) {
	sanitizer := mustSanitizer(t, replay.SanitizeOptions{Salt: "s"})
	for _, in := range sensitiveObservations() {
		out := sanitizer.Observation(in)
		data, err := json.Marshal(out)
		if err != nil {
			t.Fatal(err)
		}
		for _, word := range secretWords {
			if strings.Contains(string(data), word) {
				t.Errorf("%s leaks %q: %s", in.Kind, word, data)
			}
		}
	}
}

// The change cause is free text; the UID and origin of a note identify
// real objects. A note keeps its link to its object: the same UID gives
// the same pseudonym as the observation's own UID.
func TestSanitizerHidesChangeCauseAndNoteIdentity(t *testing.T) {
	sanitizer := mustSanitizer(t, replay.SanitizeOptions{Salt: "s"})
	obs := sensitiveObservations()
	if got := sanitizer.Observation(obs[4]).Change.Cause; got != "" {
		t.Fatalf("change cause kept: %q", got)
	}
	note := sanitizer.Observation(obs[5]).Note
	if note.UID == "" || note.UID == obs[5].Note.UID ||
		note.Origin == "" || note.Origin == obs[5].Note.Origin {
		t.Fatalf("note identity not hidden: %+v", note)
	}
	again := sanitizer.Observation(obs[5]).Note
	if again.UID != note.UID || again.Origin != note.Origin {
		t.Fatal("pseudonyms must be stable")
	}
	kept := mustSanitizer(t, replay.SanitizeOptions{Salt: "s",
		KeepMessages: true})
	if got := kept.Observation(obs[4]).Change.Cause; got !=
		"hotfix for acme-billing" {
		t.Fatalf("kept cause = %q", got)
	}
}

func TestSanitizerKeepsCodesAndStructure(t *testing.T) {
	sanitizer := mustSanitizer(t, replay.SanitizeOptions{Salt: "s"})
	obs := sensitiveObservations()
	container := sanitizer.Observation(obs[0])
	if container.Entity.Kind != "container" || container.Source !=
		"kubernetes" {
		t.Fatalf("kind or source changed: %+v", container)
	}
	for name, want := range map[string]string{
		"reason": "CrashLoopBackOff", "restarts": "4", "ready": "false",
		"since": "0001-01-01T00:00:00Z",
	} {
		if got := container.Attributes[name].AsText(); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if _, ok := container.Attributes["last.message"]; ok {
		t.Error("free-text message was kept")
	}
	if !isTimeValue(container.Attributes["since"]) {
		t.Error("time attribute lost its type")
	}
	note := sanitizer.Observation(obs[5]).Note
	if note.Reason != "BackOff" || note.Message != "" || note.Count != 2 {
		t.Fatalf("note %+v", note)
	}
	change := sanitizer.Observation(obs[4]).Change
	if change.Revision != "3" || len(change.Fields) != 2 {
		t.Fatalf("change %+v", change)
	}
	free := change.Fields[1]
	if free.Before == free.After || strings.Contains(free.Before, " ") {
		t.Fatalf("free-text change %+v should differ and be hidden", free)
	}
}

func isTimeValue(v inventory.Value) bool {
	return v.Equal(inventory.Time(v.AsTime()))
}

// The same input maps to the same pseudonym everywhere in a log, so owner
// chains, containers, images and selectors still line up.
func TestSanitizerPseudonymsAreConsistent(t *testing.T) {
	sanitizer := mustSanitizer(t, replay.SanitizeOptions{Salt: "s"})
	obs := sensitiveObservations()
	container := sanitizer.Observation(obs[0])
	pod := sanitizer.Observation(obs[1])
	service := sanitizer.Observation(obs[2])
	related := sanitizer.Observation(obs[3])
	change := sanitizer.Observation(obs[4]).Change

	podName, containerName, _ := strings.Cut(container.Entity.Name, "/")
	if podName != pod.Entity.Name || pod.Entity != related.Entity ||
		pod.Entity.Namespace != service.Entity.Namespace {
		t.Fatalf("pod pseudonyms differ: %s %s %s", container.Entity,
			pod.Entity, related.Entity)
	}
	if change.Fields[0].Path != "containers["+containerName+"].image" {
		t.Fatalf("path %q does not name container %q",
			change.Fields[0].Path, containerName)
	}
	image := container.Attributes["image"].AsText()
	if change.Fields[0].After != image {
		t.Fatalf("image %q vs change %q", image, change.Fields[0].After)
	}
	repo, _, _ := strings.Cut(image, ":")
	before, _, _ := strings.Cut(change.Fields[0].Before, ":")
	if repo != before || change.Fields[0].Before == image {
		t.Fatalf("rollout should keep the repository and change the tag: "+
			"%q -> %q", change.Fields[0].Before, image)
	}
	labels := pod.Attributes["labels"].AsText()
	selector := service.Attributes["selector"].AsText()
	app := strings.Split(labels, ",")[0]
	if !strings.HasPrefix(app, "app=") ||
		!strings.HasPrefix(selector, app+",env!=") ||
		!strings.Contains(selector, "tier in (") {
		t.Fatalf("labels %q and selector %q no longer match", labels,
			selector)
	}
	again := mustSanitizer(t, replay.SanitizeOptions{Salt: "s"})
	if again.Observation(obs[1]).Entity != pod.Entity {
		t.Fatal("the same salt must give the same pseudonyms")
	}
	other := mustSanitizer(t, replay.SanitizeOptions{Salt: "t"})
	if other.Observation(obs[1]).Entity == pod.Entity {
		t.Fatal("a different salt must give different pseudonyms")
	}
}

func TestSanitizerKeepsExplicitlyAllowedText(t *testing.T) {
	sanitizer := mustSanitizer(t, replay.SanitizeOptions{Salt: "s",
		KeepText: []string{"last.message"}, KeepMessages: true,
	})
	obs := sensitiveObservations()
	container := sanitizer.Observation(obs[0])
	if got := container.Attributes["last.message"].AsText(); got !=
		"panic: DB_PASSWORD" {
		t.Fatalf("allowed text = %q", got)
	}
	if got := sanitizer.Observation(obs[5]).Note.Message; got !=
		"Back-off restarting payments" {
		t.Fatalf("allowed message = %q", got)
	}
}

func TestSanitizerLogKeepsTimesAndOrder(t *testing.T) {
	in := sampleLog()
	out := mustSanitizer(t, replay.SanitizeOptions{Salt: "s"}).Log(in)
	if !out.Start.Equal(in.Start) || len(out.Entries) != len(in.Entries) {
		t.Fatalf("log %+v", out)
	}
	for i := range in.Entries {
		if !out.Entries[i].At.Equal(in.Entries[i].At) {
			t.Fatalf("entry %d moved", i)
		}
	}
	if in.Entries[0].Observation.Entity.Name != "n1" {
		t.Fatal("sanitizing must not modify the input")
	}
}

func TestSanitizerRequiresASalt(t *testing.T) {
	if _, err := replay.NewSanitizer(replay.SanitizeOptions{}); err !=
		replay.ErrNoSalt {
		t.Fatalf("err = %v, want ErrNoSalt", err)
	}
}

func TestSanitizerHidesAppAndNoteSourceConsistently(t *testing.T) {
	sanitizer := mustSanitizer(t, replay.SanitizeOptions{Salt: "s"})
	obs := sensitiveObservations()
	change := sanitizer.Observation(obs[4]).Change
	note := sanitizer.Observation(obs[5]).Note
	if change.App == "" || note.Source == "" {
		t.Fatalf("app %q and source %q must stay set", change.App,
			note.Source)
	}
	if again := sanitizer.Observation(obs[4]).Change; again.App !=
		change.App {
		t.Fatal("the same app must keep its pseudonym")
	}
}
