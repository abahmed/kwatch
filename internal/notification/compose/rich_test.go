package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

func digestMessage() notification.Message {
	now := time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC)
	node := inventory.CoreID(kube.KindNode, "", "ip-10-0-67-211")
	svc := inventory.CoreID(kube.KindService, "kube-addons", "ingress-nginx")
	open := []incident.Decision{
		announce(incident.Incident{ID: "n", Root: node,
			Tier: incident.Digest, State: incident.Open, RepeatCount: 2,
			Members: members(detection.Finding{Entity: node,
				Reason: "NodeNotReady", Severity: detection.Warning,
				Summary: "Node is failing again"})}),
		announce(incident.Incident{ID: "s", Root: svc,
			Tier: incident.Digest, State: incident.Open,
			Members: members(detection.Finding{Entity: svc,
				Reason:   "FailedDeployModel",
				Severity: detection.Warning,
				Summary: "Kubernetes reported FailedDeployModel " +
					"3 times in the last quarter hour"})}),
	}
	open[0].Action = incident.Update
	open[0].Reason = incident.ReasonFailingAgain
	var resolved []incident.Decision
	for _, n := range []string{"accounts", "assets", "comms", "d", "e"} {
		resolved = append(resolved, podDecision(n, incident.Digest))
	}
	var risks []detection.Finding
	add := func(reason, name string) {
		risks = append(risks, detection.Finding{Advisory: true,
			Reason: reason,
			Entity: inventory.CoreID(kube.KindDeployment, "shop", name)})
	}
	// Configuration advice is hidden; only pods that never become ready
	// are announced.
	for _, n := range []string{"accounts", "web", "app", "x", "y"} {
		add(reasons.RiskSingleReplica, n)
	}
	add(reasons.RiskMutableImageTag, "website")
	add(reasons.WorkloadNeverReady, "cart")
	return Writer{Cluster: "staging"}.Digest(open, resolved, risks, now)
}

func TestDigestIsAScannableList(t *testing.T) {
	msg := digestMessage()
	slack := msg.Render(notification.SlackDialect())
	want := "🟡 *kwatch digest* · staging — 2 problems · 5 resolved · " +
		"1 workload not ready\n\n" +
		"*Problems*\n" +
		"• Node *ip-10-0-67-211* — failing again (2nd time in 2h)\n" +
		"• Service *ingress-nginx* (*kube-addons*) — can't deploy its " +
		"load balancer: FailedDeployModel ×3 in 15 min\n\n" +
		"✅ 5 resolved since last digest: accounts, assets, comms +2\n\n" +
		"*Running but not ready*\n" +
		"• Pods never ready — 1: cart"
	if slack != want {
		t.Fatalf("slack digest:\n%s\nwant:\n%s", slack, want)
	}
	plain := msg.Plain()
	if strings.ContainsAny(plain, "*`") || !strings.Contains(plain,
		"\n- Node ip-10-0-67-211 — failing again (2nd time in 2h)\n") {
		t.Fatalf("plain digest:\n%s", plain)
	}
	if !strings.Contains(msg.Note, "kwatch (staging) has two") {
		t.Fatalf("the Note keeps its one-paragraph wording: %q", msg.Note)
	}
}

func TestIncidentLinesAndEmphasis(t *testing.T) {
	msg := Writer{Cluster: "prod-eu-1"}.Write(announce(badRollout()),
		at(5, 0))
	got := msg.Render(notification.SlackDialect())
	for _, want := range []string{
		"🔴 *payments* is down in *shop* (*prod-eu-1*) after the 14:02",
		"\nOnly pods of the new revision fail.\n",
		"Service *payments* and ingress *storefront* can't serve traffic.",
		"*checkout* is affected as well.",
		"(changes the cluster):\n```\nkubectl rollout undo " +
			"deployment/payments -n shop --to-revision=13\n```",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(msg.Note, "\n") || strings.Contains(msg.Note, "*") {
		t.Errorf("the Note stays one plain paragraph: %q", msg.Note)
	}
}

func TestQuotedPodTextBecomesCode(t *testing.T) {
	spans := podSpans(`It said "panic: *boom* <!channel>" twice.`)
	if len(spans) != 3 || spans[1].Style != notification.Code ||
		spans[1].Text != "panic: *boom* <!channel>" {
		t.Fatalf("spans = %+v", spans)
	}
	if got := podSpans(`unbalanced " quote`); len(got) != 1 ||
		got[0].Style != notification.Plain {
		t.Fatalf("unbalanced quotes stay plain: %+v", got)
	}
}

func TestBoldNamesOnlyWholeWords(t *testing.T) {
	blocks := []notification.Block{{Kind: notification.Para,
		Spans: []notification.Span{{Text: "payments:2.3 of payments, " +
			"mypayments and shop/payments in shop."}}}}
	boldNames(blocks, []string{"payments", "shop"})
	var bold []string
	for _, s := range blocks[0].Spans {
		if s.Style == notification.Bold {
			bold = append(bold, s.Text)
		}
	}
	if strings.Join(bold, ",") != "payments,shop" {
		t.Fatalf("bold = %v", bold)
	}
}

func TestStartupSummaryIsAList(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	msg := Writer{Cluster: "prod"}.StartupSummary([]incident.Decision{
		podDecision("alpha", incident.Notify)}, now)
	got := msg.Render(notification.SlackDialect())
	want := "🟠 *kwatch started* · prod — 1 problem began before it was " +
		"watching\n\n*Problems*\n• Pod *alpha* (ns) — crash looping" +
		"\n\n_" + eachOwnMessage + "_"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestLongQuotedPodTextGetsACodeBlock(t *testing.T) {
	long := strings.Repeat("panic: boom ", 10)
	blocks := paragraphs(`It fails with "` + long + `".`)
	if len(blocks) != 2 || blocks[0].Text() != "It fails with" ||
		blocks[1].Kind != notification.CodeBlock || blocks[1].Text() != long {
		t.Fatalf("blocks = %+v", blocks)
	}
	short := paragraphs(`It fails with "boom".`)
	if len(short) != 1 || short[0].Spans[1].Style != notification.Code {
		t.Fatalf("short quotes stay inline: %+v", short)
	}
}

func TestWorkloadListShowsRepeatedNamesOnce(t *testing.T) {
	id := func(ns, name string) inventory.EntityID {
		return inventory.CoreID(kube.KindDeployment, ns, name)
	}
	got := workloadList([]inventory.EntityID{id("a", "web"), id("b", "web"),
		id("a", "api"), id("a", "x"), id("a", "y")})
	if got != "api, web (a, b), x +1" {
		t.Fatalf("got %q", got)
	}
	twice := []inventory.EntityID{id("", "n"), id("", "n")}
	if got := workloadList(twice); got != "n ×2" {
		t.Fatalf("got %q", got)
	}
}
