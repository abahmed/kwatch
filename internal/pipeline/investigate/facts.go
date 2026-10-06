package investigate

import (
	"context"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// readScheduling counts, from the scheduler's own message, how many
// nodes each blocker rejects: "Insufficient memory on 3 of 5 nodes".
func readScheduling(
	_ context.Context, s Sources, p incident.Incident,
) Result {
	for _, m := range sortedMembers(p) {
		message := evidenceValue(m, "scheduler")
		if message == "" {
			message = attributeText(s.Model, m.Entity,
				kube.ConditionKey("PodScheduled")+kube.AttrConditionMessage)
		}
		if text := blockerCounts(message); text != "" {
			return Result{Evidence: []incident.Fact{
				{Kind: incident.FactScheduler, Text: text}}}
		}
	}
	return Result{}
}

func blockerCounts(message string) string {
	blockers, total := kube.ParseSchedulerMessage(message)
	var parts []string
	for _, b := range blockers {
		nodes := strconv.Itoa(b.Nodes) + " nodes"
		if total > 0 {
			nodes = strconv.Itoa(b.Nodes) + " of " + strconv.Itoa(total) +
				" nodes"
		}
		parts = append(parts, b.Reason+" on "+nodes)
	}
	return strings.Join(parts, ", ")
}

// configLookback is how far before the incident opened a config change
// still counts.
const configLookback = 30 * time.Minute

// maxConfigObjects bounds the ConfigMaps and Secrets read per incident.
const maxConfigObjects = 5

// readConfig names the keys that recent changes of the incident's
// ConfigMaps and Secrets touched. Values are never read.
func readConfig(_ context.Context, s Sources, p incident.Incident) Result {
	var r Result
	since := p.Opened.Add(-configLookback)
	for _, obj := range configObjects(s.Model, p) {
		keys := changedKeys(s.Model.Changes(obj, since))
		if len(keys) > 0 {
			r.Evidence = append(r.Evidence, incident.Fact{
				Kind: incident.FactKeys, Subject: string(obj.Kind) + " " +
					obj.Name, Text: strings.Join(keys, ", ")})
		}
	}
	return r
}

// configObjects are the root when it is a ConfigMap or Secret, else the
// ones the incident's pods reference.
func configObjects(
	model inventory.Reader, p incident.Incident,
) []inventory.EntityID {
	if isConfigKind(p.Root.Kind) {
		return []inventory.EntityID{p.Root}
	}
	var out []inventory.EntityID
	seen := map[inventory.EntityID]bool{}
	for _, pod := range memberPods(model, p) {
		for _, ref := range model.Related(pod, inventory.References,
			inventory.Outgoing) {
			if isConfigKind(ref.Kind) && !seen[ref] &&
				len(out) < maxConfigObjects {
				seen[ref] = true
				out = append(out, ref)
			}
		}
	}
	return out
}

// memberPods are the pods of the incident's members: pods themselves
// and the pods of containers.
func memberPods(
	model inventory.Reader, p incident.Incident,
) []inventory.EntityID {
	var pods []inventory.EntityID
	for _, m := range sortedMembers(p) {
		switch m.Entity.Kind {
		case kube.KindPod:
			pods = append(pods, m.Entity)
		case kube.KindContainer:
			pods = append(pods, model.Related(m.Entity, inventory.PartOf,
				inventory.Outgoing)...)
		}
	}
	return pods
}

// configKeyPrefixes are the field paths of config data.
var configKeyPrefixes = []string{"data.", "stringData.", "binaryData."}

// changedKeys names the data keys the changes touched, sorted, each
// once. Only paths are read, never Before or After.
func changedKeys(changes []inventory.Change) []string {
	seen := map[string]bool{}
	var keys []string
	for _, c := range changes {
		for _, f := range c.Fields {
			for _, prefix := range configKeyPrefixes {
				key, ok := strings.CutPrefix(f.Path, prefix)
				if ok && key != "" && !seen[key] {
					seen[key] = true
					keys = append(keys, key)
				}
			}
		}
	}
	sort.Strings(keys)
	return keys
}

// readAdmission counts the ready endpoints of the webhook's Services,
// from the model when it has their EndpointSlices and from the API
// otherwise, and quotes the admission failure the API server returned.
func readAdmission(
	ctx context.Context, s Sources, p incident.Incident,
) Result {
	var r Result
	for _, svc := range s.Model.Related(p.Root, inventory.Serves,
		inventory.Outgoing) {
		ready, ok := modelEndpoints(s.Model, svc)
		if !ok && s.Endpoints != nil {
			ready, ok = s.Endpoints(ctx, svc)
		}
		if ok {
			r.Evidence = append(r.Evidence, incident.Fact{
				Kind: incident.FactEndpoints, Subject: svc.Name,
				Text: strconv.Itoa(ready)})
		}
	}
	if text := admissionFailure(s.Model, p); text != "" {
		r.Evidence = append(r.Evidence,
			incident.Fact{Kind: incident.FactWebhook, Text: text})
	}
	return r
}

// modelEndpoints sums ready endpoints across the Service's slices in
// the model. ok is false when the model has none.
func modelEndpoints(
	model inventory.Reader, service inventory.EntityID,
) (int, bool) {
	slices := model.Related(service, inventory.Backs, inventory.Incoming)
	ready := 0.0
	for _, id := range slices {
		if slice, ok := model.Entity(id); ok {
			ready += number(slice, kube.AttrEndpointsReady)
		}
	}
	return int(ready), len(slices) > 0
}

var admissionText = regexp.MustCompile(`(?i)webhook|admission`)

// admissionFailure is the first failure text mentioning a webhook in
// the members' evidence or in their objects' messages.
func admissionFailure(model inventory.Reader, p incident.Incident) string {
	for _, m := range sortedMembers(p) {
		for _, e := range m.Evidence {
			if admissionText.MatchString(e.Value) {
				return strings.TrimSpace(e.Value)
			}
		}
		if text := messageMatching(model, m.Entity, admissionText); text != "" {
			return text
		}
	}
	return ""
}

// messageMatching is the first message attribute of id, by name, that
// matches pattern.
func messageMatching(
	model inventory.Reader, id inventory.EntityID, pattern *regexp.Regexp,
) string {
	e, ok := model.Entity(id)
	if !ok {
		return ""
	}
	var names []string
	for name := range e.Attributes {
		if name == kube.AttrMessage ||
			strings.HasSuffix(name, kube.AttrConditionMessage) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		if text := e.Attributes[name].Value.AsText(); pattern.MatchString(text) {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

// Pull error classes, first match wins. Image-level answers come first:
// a registry says "pull access denied, repository does not exist" for a
// typo.
var pullClasses = []struct {
	class   string
	pattern *regexp.Regexp
}{
	{"image", regexp.MustCompile(`not found|manifest unknown|` +
		`name unknown|does not exist|invalid reference`)},
	{"rate-limit", regexp.MustCompile(`toomanyrequests|too many requests|` +
		`rate limit|\b429\b`)},
	{"auth", regexp.MustCompile(`unauthorized|authentication required|` +
		`no basic auth|failed to authorize|\b401\b|\b403\b|forbidden`)},
	{"server", regexp.MustCompile(`\b50[0234]\b|internal server error|` +
		`bad gateway|service unavailable|gateway timeout`)},
	{"tls", regexp.MustCompile(`x509|tls:|certificate`)},
	{"network", regexp.MustCompile(`timeout|timed out|connection refused|` +
		`connection reset|no such host|no route to host|` +
		`network is unreachable|dial tcp`)},
}

// readRegistry classifies the pull error of the incident's members.
func readRegistry(_ context.Context, s Sources, p incident.Incident) Result {
	for _, m := range sortedMembers(p) {
		text := evidenceValue(m, "error", "message")
		if text == "" {
			text = attributeText(s.Model, m.Entity, kube.AttrMessage)
		}
		if class := pullClass(text); class != "" {
			return Result{Evidence: []incident.Fact{
				{Kind: incident.FactPull, Text: class}}}
		}
	}
	return Result{}
}

func pullClass(text string) string {
	text = strings.ToLower(text)
	for _, c := range pullClasses {
		if c.pattern.MatchString(text) {
			return c.class
		}
	}
	return ""
}

// sortedMembers returns the members most severe first, then by entity,
// so investigations read them in a stable order.
func sortedMembers(p incident.Incident) []detection.Finding {
	out := make([]detection.Finding, 0, len(p.Members))
	for _, m := range p.Members {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Severity != out[j].Severity {
			return out[i].Severity > out[j].Severity
		}
		if out[i].Entity != out[j].Entity {
			return out[i].Entity.String() < out[j].Entity.String()
		}
		return out[i].Reason < out[j].Reason
	})
	return out
}

// evidenceValue is the first non-empty evidence value under one of
// labels.
func evidenceValue(f detection.Finding, labels ...string) string {
	for _, label := range labels {
		for _, e := range f.Evidence {
			if e.Label == label && strings.TrimSpace(e.Value) != "" {
				return strings.TrimSpace(e.Value)
			}
		}
	}
	return ""
}

func attributeText(
	model inventory.Reader, id inventory.EntityID, name string,
) string {
	e, ok := model.Entity(id)
	if !ok {
		return ""
	}
	if a, ok := e.Attribute(name); ok {
		return strings.TrimSpace(a.Value.AsText())
	}
	return ""
}
