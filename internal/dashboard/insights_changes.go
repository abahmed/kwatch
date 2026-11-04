package dashboard

import (
	"cmp"
	"slices"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/redact"
)

// InsightChanges counts the active issues that started right after a
// change, out of Total active issues, and lists the newest of them.
type InsightChanges struct {
	Count  int                  `json:"count"`
	Total  int                  `json:"total"`
	Issues []InsightChangeIssue `json:"issues"`
}

// InsightChangeIssue is an issue and the change made just before it:
// Object is the changed object (Deployment/api), Change what changed
// ("image api:1.2 → api:1.3").
type InsightChangeIssue struct {
	InsightIssue
	Object    string    `json:"object"`
	Change    string    `json:"change"`
	ChangedAt time.Time `json:"changedAt"`
}

// changeBefore is the latest change p stored that happened within
// insightChangeWindow before it started.
func changeBefore(p incident.Incident) (inventory.Change, bool) {
	var best inventory.Change
	found := false
	from := p.Opened.Add(-insightChangeWindow)
	for _, c := range p.Journal.Changes {
		if c.At.Before(from) || c.At.After(p.Opened) {
			continue
		}
		if !found || c.At.After(best.At) {
			best, found = c, true
		}
	}
	return best, found
}

// afterChange lists the active issues with a change just before they
// started, newest start first.
func afterChange(
	live []incident.Incident, red func(string) string,
) InsightChanges {
	out := InsightChanges{Total: len(live),
		Issues: make([]InsightChangeIssue, 0)}
	var hits []InsightChangeIssue
	for _, p := range live {
		c, ok := changeBefore(p)
		if !ok {
			continue
		}
		object := string(c.Entity.Kind) + "/" + c.Entity.Name
		summary, _ := shortText(redact.Evidence(changeSummary(c)),
			maxInsightText)
		hits = append(hits, InsightChangeIssue{
			InsightIssue: insightIssue(p, red), Object: object,
			Change: summary, ChangedAt: c.At})
	}
	slices.SortFunc(hits, func(a, b InsightChangeIssue) int {
		return cmp.Or(b.Since.Compare(a.Since), cmp.Compare(a.ID, b.ID))
	})
	out.Count = len(hits)
	out.Issues = append(out.Issues, hits[:min(len(hits), insightTop)]...)
	return out
}

// InsightDeliveries counts the messages of the last day that did not
// get out (the latest send of an issue's message kind to a provider
// failed or is being retried) and lists the newest.
type InsightDeliveries struct {
	Count    int               `json:"count"`
	Attempts []InsightDelivery `json:"attempts"`
}

// InsightDelivery is one message that did not get out.
type InsightDelivery struct {
	At       time.Time `json:"at"`
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Provider string    `json:"provider"`
	Kind     string    `json:"kind,omitempty"`
	Result   string    `json:"result"`
	Error    string    `json:"error,omitempty"`
}

// undelivered reads the send trail of every incident held.
func undelivered(
	items []incident.Incident, trail deliveryTrailSource, now time.Time,
	red func(string) string,
) InsightDeliveries {
	out := InsightDeliveries{Attempts: make([]InsightDelivery, 0)}
	if trail == nil {
		return out
	}
	var all []InsightDelivery
	for _, p := range items {
		for _, a := range lastSends(trail(p.ID), now) {
			if a.Result != "failed" && a.Result != "retrying" {
				continue
			}
			title, _ := shortText(titleFor(p, red), maxInsightText)
			msg, _ := shortText(redact.Evidence(a.Error), maxInsightText)
			all = append(all, InsightDelivery{At: a.At, ID: p.ID,
				Title: title, Provider: a.Channel, Kind: a.Kind,
				Result: a.Result, Error: msg})
		}
	}
	slices.SortFunc(all, func(a, b InsightDelivery) int {
		return cmp.Or(b.At.Compare(a.At), cmp.Compare(a.ID, b.ID),
			cmp.Compare(a.Provider, b.Provider))
	})
	out.Count = len(all)
	out.Attempts = append(out.Attempts, all[:min(len(all), insightTop)]...)
	return out
}

// lastSends is the latest attempt per provider and message kind within
// insightDeliveryWindow; attempts without a provider (carried by a
// digest, not routed) are left out.
func lastSends(trail []DeliveryAttempt, now time.Time) []DeliveryAttempt {
	from := now.Add(-insightDeliveryWindow)
	latest := map[[2]string]int{}
	var out []DeliveryAttempt
	for _, a := range trail {
		if a.Channel == "" || a.At.Before(from) {
			continue
		}
		key := [2]string{a.Channel, a.Kind}
		if i, ok := latest[key]; ok {
			if !a.At.Before(out[i].At) {
				out[i] = a
			}
			continue
		}
		latest[key] = len(out)
		out = append(out, a)
	}
	return out
}
