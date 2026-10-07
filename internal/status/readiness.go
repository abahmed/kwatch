package status

import (
	"sort"
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/redact"
)

// Classes of upgrade blockers, in the order they are listed.
const (
	ClassAPI     = "deprecated-api"
	ClassSkew    = "version-skew"
	ClassBudget  = "disruption-budget"
	ClassWebhook = "webhook"
)

// maxPerClass is how many blockers of one class are named; the rest
// are counted in one "and N more" item.
const maxPerClass = 3

// classOrder lists the classes as /status and the digest print them.
var classOrder = []string{ClassAPI, ClassSkew, ClassBudget, ClassWebhook}

// classNames word a class for the "and N more" item.
var classNames = map[string]string{
	ClassAPI:     "deprecated APIs",
	ClassSkew:    "nodes outside the version skew",
	ClassBudget:  "disruption budgets that allow none",
	ClassWebhook: "webhooks with an unhealthy backend",
}

// Readiness answers "is it safe to upgrade the cluster?". It is built
// from what kwatch already detects; it finds nothing of its own.
type Readiness struct {
	// Total counts every blocker, named or not.
	Total  int            `json:"blockers"`
	Counts map[string]int `json:"counts,omitempty"`
	Items  []Blocker      `json:"items,omitempty"`
}

// Blocker is one thing that stops, or is wrong for, an upgrade.
type Blocker struct {
	Class string `json:"class"`
	Text  string `json:"text"`
	// Entity is the object it is about; zero for an "and N more" item.
	Entity inventory.EntityID `json:"-"`
	// More is how many blockers an "and N more" item stands for.
	More int `json:"more,omitempty"`
}

// weight is how many blockers b counts for.
func (b Blocker) weight() int {
	if b.More > 0 {
		return b.More
	}
	return 1
}

// Assess collects the blockers from the active findings and from the
// disruption budgets in the model. Everything it lists is true now.
func Assess(
	findings []detection.Finding, model inventory.Reader,
) Readiness {
	found := map[string][]Blocker{}
	for _, f := range findings {
		if b, ok := blockerOf(f); ok {
			found[b.Class] = append(found[b.Class], b)
		}
	}
	if model != nil {
		found[ClassBudget] = append(found[ClassBudget], budgets(model)...)
	}
	out := Readiness{Counts: map[string]int{}}
	for _, class := range classOrder {
		list := found[class]
		if len(list) == 0 {
			continue
		}
		sort.Slice(list, func(i, j int) bool {
			return list[i].Text < list[j].Text
		})
		out.Counts[class] = len(list)
		out.Total += len(list)
		out.Items = append(out.Items, named(class, list)...)
	}
	return out
}

// named keeps the first blockers of a class and counts the rest.
func named(class string, list []Blocker) []Blocker {
	if len(list) <= maxPerClass {
		return list
	}
	rest := len(list) - maxPerClass
	return append(list[:maxPerClass:maxPerClass], Blocker{Class: class,
		More: rest,
		Text: "and " + strconv.Itoa(rest) + " more " + classNames[class]})
}

// blockerOf words a finding as a blocker when it is one.
func blockerOf(f detection.Finding) (Blocker, bool) {
	name := EntityName(f.Entity)
	text := ""
	class := ""
	switch f.Reason {
	case reasons.DeprecatedAPIInUse:
		class = ClassAPI
		text = f.Entity.Name + " is still requested (" + f.Summary + ")"
	case reasons.ClusterVersionSkew:
		class = ClassSkew
		text = "node " + name + ": " + f.Summary
	case reasons.WebhookNoEndpoints:
		class, text = webhook(f, name, "has no ready endpoints")
	case reasons.WebhookBackendNotFound:
		class, text = webhook(f, name, "calls a Service that does not exist")
	}
	if text == "" {
		return Blocker{}, false
	}
	return Blocker{Class: class, Text: redact.Evidence(text),
		Entity: f.Entity}, true
}

// webhook words a webhook whose backend is gone. Only failurePolicy Fail
// blocks requests; the finding is Critical exactly then.
func webhook(f detection.Finding, name, what string) (string, string) {
	if f.Severity < detection.Critical {
		return "", ""
	}
	return ClassWebhook, "webhook " + name + " (Fail) " + what
}

// budgets lists the disruption budgets that allow no eviction now: a
// node drain is held up by them now.
func budgets(model inventory.Reader) []Blocker {
	var out []Blocker
	for _, id := range model.Entities(kube.KindPDB) {
		e, ok := model.Entity(id)
		if !ok {
			continue
		}
		allowed, known := numberOf(e, kube.AttrDisruptionsAllowed)
		expected, _ := numberOf(e, kube.AttrExpectedPods)
		if !known || allowed > 0 || expected == 0 {
			continue
		}
		out = append(out, Blocker{Class: ClassBudget, Entity: id,
			Text: "PDB " + EntityName(id) + " currently allows 0 disruptions"})
	}
	return out
}

func numberOf(e inventory.Entity, name string) (float64, bool) {
	attribute, ok := e.Attribute(name)
	if !ok {
		return 0, false
	}
	return attribute.Value.AsNumber()
}

// EntityName is "namespace/name", or the name alone.
func EntityName(id inventory.EntityID) string {
	if id.Namespace == "" {
		return id.Name
	}
	return id.Namespace + "/" + id.Name
}

// Skipping drops the blockers about entities in skip: the digest lists
// those as incidents already.
func (r Readiness) Skipping(skip map[inventory.EntityID]bool) Readiness {
	out := Readiness{Counts: map[string]int{}}
	for _, b := range r.Items {
		if b.Entity != (inventory.EntityID{}) && skip[b.Entity] {
			continue
		}
		out.Items = append(out.Items, b)
		out.Counts[b.Class] += b.weight()
		out.Total += b.weight()
	}
	return out
}

// Line is one sentence for the digest: "Upgrade readiness: 3 blockers
// - a; b; c." It is empty when nothing blocks.
func (r Readiness) Line() string {
	if r.Total == 0 {
		return ""
	}
	texts := make([]string, 0, len(r.Items))
	for _, b := range r.Items {
		texts = append(texts, b.Text)
	}
	noun := "blockers"
	if r.Total == 1 {
		noun = "blocker"
	}
	return "Upgrade readiness: " + strconv.Itoa(r.Total) + " " + noun +
		" — " + strings.Join(texts, "; ") + "."
}

// Key identifies what the blockers are, so a digest repeats them only
// when they change.
func (r Readiness) Key() string {
	texts := make([]string, 0, len(r.Items))
	for _, b := range r.Items {
		texts = append(texts, b.Text)
	}
	return strings.Join(texts, "\n")
}
