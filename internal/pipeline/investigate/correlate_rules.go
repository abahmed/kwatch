package investigate

import (
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// maxChangePaths bounds the paths a change fact names.
const maxChangePaths = 3

// firstFailure is when the earliest member began failing, else when the
// incident opened.
func firstFailure(p incident.Incident) time.Time {
	first := p.Opened
	for _, m := range p.Members {
		if !m.Since.IsZero() && m.Since.Before(first) {
			first = m.Since
		}
	}
	return first
}

// changeCandidates are the objects whose change may have started the
// failure: the root, the cause's root, the workload, the pods' config
// objects and their node.
func changeCandidates(
	model inventory.Reader, p incident.Incident,
) []inventory.EntityID {
	ids := []inventory.EntityID{p.Root}
	if p.Cause != nil {
		ids = append(ids, p.Cause.Root)
	}
	if w, ok := workloadOf(model, p); ok {
		ids = append(ids, w)
	}
	ids = append(ids, configObjects(model, p)...)
	for _, pod := range incidentPods(model, p) {
		ids = append(ids, relatedOf(model, pod, inventory.RunsOn,
			inventory.Outgoing, kube.KindNode)...)
	}
	return firstUnique(ids, len(ids))
}

// nearestChange names the latest change before the first failure:
// "deployment api changed spec.template... at 13:41 UTC, 1m before the
// first failure". Values are never shown.
func nearestChange(model inventory.Reader, p incident.Incident) string {
	first := firstFailure(p)
	var best *inventory.Change
	for _, id := range changeCandidates(model, p) {
		for _, c := range model.Changes(id, first.Add(-configLookback)) {
			if c.At.After(first) || (best != nil && !c.At.After(best.At)) {
				continue
			}
			change := c
			best = &change
		}
	}
	if best == nil {
		return ""
	}
	var paths []string
	for _, f := range best.Fields[:min(len(best.Fields), maxChangePaths)] {
		paths = append(paths, f.Path)
	}
	text := string(best.Entity.Kind) + " " + best.Entity.Name + " changed"
	if len(paths) > 0 {
		text += " (" + strings.Join(paths, ", ") + ")"
	}
	return text + " at " + best.At.UTC().Format("15:04 UTC") + ", " +
		format.Duration(first.Sub(best.At)) + " before the first failure"
}

// spreadOf says how far the signature reaches: the pods whose output
// has it, of the pods read, and the nodes they run on.
func spreadOf(
	model inventory.Reader, p incident.Incident, report []incident.Section,
	sig signature,
) string {
	if sig.class == "" {
		return ""
	}
	pods, read := podsWithSignature(report, logSignature(sig.line))
	if len(pods) <= 1 {
		return "Only this pod"
	}
	nodes := map[string]bool{}
	for _, id := range incidentPods(model, p) {
		if !pods[id.Name] {
			continue
		}
		for _, n := range relatedOf(model, id, inventory.RunsOn,
			inventory.Outgoing, kube.KindNode) {
			nodes[n.Name] = true
		}
	}
	return "Same output in " + strconv.Itoa(len(pods)) + " of " +
		strconv.Itoa(read) + " pods, on " +
		strconv.Itoa(len(nodes)) + " nodes"
}

// podsWithSignature are the pods whose log sections carry want, and
// how many pods were read.
func podsWithSignature(
	report []incident.Section, want string,
) (map[string]bool, int) {
	pods, read := map[string]bool{}, map[string]bool{}
	for _, s := range report {
		if s.Check != incident.CheckLogs {
			continue
		}
		pod, _ := splitName(s.Subject)
		read[pod] = true
		for _, line := range matchedLines(s) {
			if logSignature(clip(line)) == want {
				pods[pod] = true
			}
		}
	}
	return pods, len(read)
}

// rolloutDiff joins the image and resources changes the rollout
// section shows.
func rolloutDiff(report []incident.Section) string {
	var diffs []string
	for _, s := range report {
		if s.Check != incident.CheckRollout {
			continue
		}
		for _, f := range s.Facts {
			if strings.HasPrefix(f.Label, "Changed at") &&
				strings.Contains(f.Value, "→") {
				diffs = append(diffs, f.Value)
			}
		}
	}
	return strings.Join(diffs, "; ")
}

// confirmRules prove a cause from gathered evidence, first match wins.
// Each returns what it read that proves it, or "".
var confirmRules = []func(
	model inventory.Reader, p incident.Incident, report []incident.Section,
	sig signature,
) string{
	confirmUnreachable, confirmOOM, confirmMissingConfig,
	confirmVolume, confirmPullSecret, confirmWaiting, confirmNode,
	confirmNoPods, confirmCrashExit,
}

// confirm is why gathered evidence proves the cause, or "".
func confirm(
	model inventory.Reader, p incident.Incident, report []incident.Section,
	sig signature,
) string {
	for _, rule := range confirmRules {
		if why := rule(model, p, report, sig); why != "" {
			return why
		}
	}
	return ""
}

// blamed reports id as the incident's root or its cause's root.
func blamed(p incident.Incident, id inventory.EntityID) bool {
	return id == p.Root || (p.Cause != nil && id == p.Cause.Root)
}

func confirmUnreachable(
	model inventory.Reader, p incident.Incident, _ []incident.Section,
	sig signature,
) string {
	if !sig.found || sig.target.Kind != kube.KindService ||
		!blamed(p, sig.target) {
		return ""
	}
	if ready, ok := modelEndpoints(model, sig.target); !ok || ready > 0 {
		return ""
	}
	return "the logs show " + sig.class + " to " + sig.target.Name +
		", and the Service has 0 ready endpoints"
}

func confirmOOM(
	model inventory.Reader, p incident.Incident, _ []incident.Section,
	_ signature,
) string {
	if !hasMode(p, []string{"OOMKilled"}) {
		return ""
	}
	for _, pod := range incidentPods(model, p) {
		for _, c := range containersOf(model, pod) {
			e, _ := model.Entity(c)
			if textOf(e, kube.AttrLastReason) == "OOMKilled" {
				return "the last run of " + c.Name + " ended OOMKilled" +
					exitCode(e)
			}
		}
	}
	return ""
}

func exitCode(e inventory.Entity) string {
	if code := textOf(e, kube.AttrLastExitCode); code != "" {
		return ", exit code " + code
	}
	return ""
}

func confirmMissingConfig(
	model inventory.Reader, p incident.Incident, _ []incident.Section,
	_ signature,
) string {
	if p.Cause == nil || !isConfigKind(p.Cause.Root.Kind) ||
		model.Exists(p.Cause.Root) {
		return ""
	}
	return string(p.Cause.Root.Kind) + " " + p.Cause.Root.Name +
		" does not exist in " + p.Cause.Root.Namespace
}

func confirmVolume(
	model inventory.Reader, p incident.Incident, _ []incident.Section,
	_ signature,
) string {
	for _, claim := range claimsOf(model, p) {
		for _, class := range relatedOf(model, claim, inventory.References,
			inventory.Outgoing, kube.KindStorageClass) {
			if !model.Exists(class) && (blamed(p, claim) || blamed(p, class)) {
				return "the claim names the StorageClass " + class.Name +
					", which does not exist"
			}
		}
	}
	return ""
}

func confirmPullSecret(
	_ inventory.Reader, p incident.Incident, report []incident.Section,
	_ signature,
) string {
	if !isRegistryRoot(p) {
		return ""
	}
	for _, s := range report {
		for _, f := range s.Facts {
			if f.Label == "Pull secret" &&
				strings.HasSuffix(f.Value, ", which does not exist") {
				return "the pull secret " + strings.TrimSuffix(f.Value,
					", which does not exist") + " does not exist"
			}
		}
	}
	return ""
}

func confirmNode(
	model inventory.Reader, p incident.Incident, _ []incident.Section,
	_ signature,
) string {
	if !isNodeRoot(p) {
		return ""
	}
	node, ok := model.Entity(p.Root)
	if !ok {
		return ""
	}
	if failing := nodeConditions(node); failing != "" {
		return "node " + p.Root.Name + " reports " + failing
	}
	return ""
}

func confirmNoPods(
	model inventory.Reader, p incident.Incident, _ []incident.Section,
	_ signature,
) string {
	if p.Root.Kind != kube.KindService || !model.Exists(p.Root) {
		return ""
	}
	selected := relatedOf(model, p.Root, inventory.Selects,
		inventory.Outgoing, kube.KindPod)
	if len(selected) > 0 {
		return ""
	}
	return "the selector of " + p.Root.Name + " matches no pod"
}
