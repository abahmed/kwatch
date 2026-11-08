//go:build browser

package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// browserRoundCause is a cause with every part of "What kwatch found":
// an address read from a ConfigMap, workloads that began together, a
// share of the failing pods and a chain of three steps.
func browserRoundCause() *rootcause.CauseRecord {
	db := inventory.CoreID(kube.KindExternalEndpoint, "",
		"db.example.com:5432")
	hop := func(kind inventory.Kind, ns, name string, min int,
	) rootcause.Hop {
		return rootcause.Hop{Entity: inventory.CoreID(kind, ns, name),
			Began: testEpoch.Add(time.Duration(min) * time.Minute)}
	}
	return &rootcause.CauseRecord{
		Root: db, Score: 0.8,
		Source: &rootcause.ConfigSource{ConfigMap: inventory.CoreID(
			kube.KindConfigMap, "shop", "api-config"), Key: "DB_HOST",
			Address: db.Name},
		Proof: []rootcause.Proof{
			{Code: rootcause.ProofCoOnset, Count: 2, Total: 4,
				Supports: true},
			{Code: rootcause.ProofBlastOverlap, Count: 9, Total: 10,
				Supports: true}},
		Hops: []rootcause.Hop{
			hop(kube.KindNode, "", "n3", 0),
			hop(kube.KindDeployment, "shop", "redis", 1),
			hop(kube.KindDeployment, "shop", "checkout", 2)},
	}
}

// The issue page tells where an address was read, the share of failing
// pods, the chain of events and a folded help that remembers if it was
// opened, at both sizes, without scrolling sideways.
func TestBrowserIssueRound(t *testing.T) {
	ctx, srv := browserSetup(t)
	for _, size := range browserSizes {
		tab, problems := browserTab(t, ctx)
		signInBrowser(t, tab, srv.URL)
		name := "/issues/inc-002 round"
		checkPage(t, tab, problems, srv.URL+"/issues/inc-002", size.w,
			size.h, name)
		text, err := chromedp.Run(tab, chromedp.Evaluate[string](
			`document.querySelector(".rd").innerText`))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		helpOpen, err := chromedp.Run(tab, chromedp.Evaluate[bool](`(() => {
		  const d = document.querySelector(".rd-help");
		  const was = d.open;
		  d.open = true;
		  return was;
		})()`))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if helpOpen {
			t.Errorf("%s: the help must start closed", name)
		}
		for _, want := range []string{"What kwatch found",
			"(from ConfigMap api-config, DB_HOST)",
			"9 of 10 failing pods read it", "Chain of events",
			"started failing 4 s apart"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s: missing %q in:\n%s", name, want, text)
			}
		}
		stored, err := chromedp.Run(tab, chromedp.Evaluate[string](
			`localStorage.getItem("kwatch.issueHelp") || ""`))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if stored != "open" {
			t.Errorf("%s: help state not kept: %q", name, stored)
		}
	}
}
