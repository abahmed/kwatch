package message

import (
	"fmt"
	"strings"
)

// markup is the only thing that differs between text providers: how to bold,
// how to fence code, how to write a link. Everything else — what to say and in
// what order — is shared, so the 55 text providers cannot drift apart.
type markup struct {
	bold func(string) string
	code func(string) string
	link func(label, url string) string
	hint string // prefix for the hint line
}

var (
	plainMarkup = markup{
		bold: func(s string) string { return s },
		code: func(s string) string { return s },
		link: func(label, url string) string { return label + ": " + url },
		hint: "Hint:",
	}
	slackMarkup = markup{
		bold: func(s string) string { return "*" + s + "*" },
		code: func(s string) string { return "```" + s + "```" },
		link: func(label, url string) string {
			return "<" + url + "|" + label + ">"
		},
		hint: "💡",
	}
	discordMarkup = markup{
		bold: func(s string) string { return "**" + s + "**" },
		code: func(s string) string { return "```\n" + s + "\n```" },
		link: func(label, url string) string {
			return "[" + label + "](" + url + ")"
		},
		hint: "💡",
	}
)

// textRenderer renders a Report in the same top-down order a person reads:
// what happened, to what; the current state; why and what it affects; the
// hint; the identifying details; the evidence.
type textRenderer struct{ m markup }

func (t textRenderer) RenderCreate(r *Report) string {
	lines := []string{t.headline(r)}
	lines = append(lines, t.story(r))
	lines = append(lines, t.changes(r))
	lines = append(lines, t.hint(r))
	lines = append(lines, t.meta(r))
	lines = append(lines, t.typeSpecific(r)...)
	lines = append(lines, t.suppressed(r)...)
	lines = append(lines, t.evidence(r)...)
	if r.Runbook != "" {
		lines = append(lines, "📖 "+t.m.link("Runbook", r.Runbook))
	}
	return joinNonEmpty(lines)
}

// RenderUpdate is a follow-up on something already announced: only what may
// have moved — state, cause, identity, fresh evidence.
func (t textRenderer) RenderUpdate(r *Report) string {
	lines := []string{t.headline(r)}
	lines = append(lines, t.story(r))
	lines = append(lines, t.meta(r))
	lines = append(lines, t.typeSpecific(r)...)
	lines = append(lines, t.evidence(r)...)
	return joinNonEmpty(lines)
}

// story turns the structured diagnosis into a short, natural explanation.
// It deliberately avoids exposing the internal field layout as a form.
func (t textRenderer) story(r *Report) string {
	if r == nil {
		return ""
	}
	var sentences []string
	if r.State != nil && r.State.Message != "" {
		sentences = append(sentences, r.State.Message)
	}
	if r.Diagnosis != nil {
		if r.Diagnosis.Cause != "" {
			cause := "The strongest signal points to " + strings.TrimSuffix(r.Diagnosis.Cause, ".")
			if r.Diagnosis.Confidence > 0 {
				cause += fmt.Sprintf(" (%.0f%% confidence)", r.Diagnosis.Confidence*100)
			}
			sentences = append(sentences, cause+".")
		}
		if r.Diagnosis.Impact != "" {
			sentences = append(sentences, capitalizeSentence(r.Diagnosis.Impact)+".")
		}
		if len(r.Diagnosis.Evidence) > 0 {
			sentences = append(sentences, "This is supported by "+strings.Join(r.Diagnosis.Evidence, "; ")+".")
		}
		if len(r.Diagnosis.NextSteps) > 0 {
			sentences = append(sentences, "Start by "+strings.ToLower(strings.TrimSuffix(r.Diagnosis.NextSteps[0], "."))+".")
		}
	}
	return strings.Join(sentences, " ")
}

func capitalizeSentence(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func (t textRenderer) RenderResolved(r *Report) string {
	head := fmt.Sprintf(
		"%s %s",
		r.Summary.Emoji,
		t.m.bold("Resolved — "+labelOf(r)),
	)
	if subj := subjectOf(r); subj != "" {
		head += " — " + subj
	}
	if labelOf(r) != r.Reason {
		head += " · " + r.Reason
	}
	var info []string
	if r.Summary.Duration != "" {
		info = append(info, "lasted "+r.Summary.Duration)
	}
	if r.Summary.Count > 1 {
		info = append(info, fmt.Sprintf("%d occurrences", r.Summary.Count))
	}
	if r.Summary.Peak > 1 {
		info = append(info, fmt.Sprintf("peak %d pods", r.Summary.Peak))
	}
	if r.Identity != nil && r.Identity.Node != "" {
		info = append(info, "node "+r.Identity.Node)
	}
	return joinNonEmpty([]string{head, strings.Join(info, " · ")})
}

// ── pieces ──────────────────────────────────────────────────────────

func labelOf(r *Report) string {
	if r.Summary.Label != "" {
		return r.Summary.Label
	}
	return r.Reason
}

// subjectOf is what the incident happened to. A group's Name is already a
// sentence ("6 workloads in dev: …") and is used as is; a single
// resource is qualified with its namespace.
func subjectOf(r *Report) string {
	name := r.Name
	if name == "" {
		return ""
	}
	if strings.Contains(name, " ") || r.Namespace == "" ||
		strings.HasPrefix(name, r.Namespace+"/") {
		return name
	}
	return r.Namespace + "/" + name
}

func isGroupSubject(r *Report) bool { return strings.Contains(r.Name, " ") }

// headline: "🔴 Pod not ready — dev/api · Deployment ·
// ContainersNotReady · high"
func (t textRenderer) headline(r *Report) string {
	h := fmt.Sprintf("%s %s", r.Summary.Emoji, t.m.bold(labelOf(r)))
	if subj := subjectOf(r); subj != "" {
		h += " — " + subj
	}
	if r.Identity != nil && r.Identity.OwnerKind != "" && !isGroupSubject(r) &&
		!strings.Contains(r.Name, r.Identity.OwnerKind) {
		h += " · " + r.Identity.OwnerKind
	}
	if labelOf(r) != r.Reason {
		h += " · " + r.Reason
	}
	if r.Severity != "" && r.Severity != "normal" {
		h += " · " + t.m.bold(r.Severity)
	}
	return h
}

// changes: what moved just before the incident, with how long before.
func (t textRenderer) changes(r *Report) string {
	if r.Changes == nil || len(r.Changes.Items) == 0 {
		return ""
	}
	const show = 3
	var parts []string
	for i, c := range r.Changes.Items {
		if i == show {
			parts = append(
				parts,
				fmt.Sprintf("+%d more", len(r.Changes.Items)-show),
			)
			break
		}
		p := fmt.Sprintf("%s %s %s", c.Resource, c.Reference, strings.ToLower(c.Type))
		if c.Age != "" {
			p += " " + c.Age + " ago"
		}
		if len(c.Fields) > 0 {
			field := c.Fields[0]
			p += ": " + field.Path
			if field.Before != "" && field.After != "" {
				p += " changed from " + field.Before + " to " + field.After
			}
			if c.Additional > 0 {
				p += fmt.Sprintf(" (+%d more fields)", c.Additional)
			}
		}
		parts = append(parts, p)
	}
	return "A recent change may be related: " + strings.Join(parts, "; ")
}

func (t textRenderer) hint(r *Report) string {
	if r.Diagnosis == nil || r.Diagnosis.Hint == "" {
		return ""
	}
	return t.m.hint + " " + r.Diagnosis.Hint
}

// meta is the identifying detail, one line, only what is set.
func (t textRenderer) meta(r *Report) string {
	var parts []string
	if r.Identity != nil {
		if r.Identity.Container != "" {
			parts = append(parts, "Container: "+r.Identity.Container)
		}
		if r.Identity.Image != "" {
			parts = append(parts, "Image: "+r.Identity.Image)
		}
		if r.Identity.Node != "" {
			parts = append(parts, "Node: "+r.Identity.Node)
		}
	}
	if r.State != nil {
		if r.State.ExitCode > 0 {
			parts = append(
				parts,
				fmt.Sprintf("Exit code: %d", r.State.ExitCode),
			)
		}
		if r.State.Restarts > 0 {
			parts = append(parts, fmt.Sprintf("Restarts: %d", r.State.Restarts))
		}
	}
	if r.Summary.Count > 1 {
		parts = append(parts, fmt.Sprintf("Seen: ×%d", r.Summary.Count))
	}
	if r.Summary.Peak > 1 {
		parts = append(parts, fmt.Sprintf("Peak: %d pods", r.Summary.Peak))
	}
	if r.Summary.Duration != "" {
		parts = append(parts, "Duration: "+r.Summary.Duration)
	}
	if r.Cluster != "" {
		parts = append(parts, "Cluster: "+r.Cluster)
	}
	return strings.Join(parts, " · ")
}

func (t textRenderer) typeSpecific(r *Report) []string {
	var out []string
	if r.OOM != nil {
		if r.OOM.MemoryLimit != "" {
			out = append(out, "Memory limit: "+r.OOM.MemoryLimit)
		}
		if r.OOM.IsLeak {
			out = append(
				out,
				fmt.Sprintf(
					"⚠️ Potential memory leak (%d OOMs in %dm)",
					r.OOM.LeakCount,
					r.OOM.WindowMin,
				),
			)
		}
		if r.OOM.Timeline != "" {
			out = append(out, "Memory before crash: "+r.OOM.Timeline)
		}
	}
	if r.Probe != nil {
		out = append(
			out,
			fmt.Sprintf("Probe: %s %s", r.Probe.ProbeType, r.Probe.Endpoint),
		)
	}
	if r.Image != nil && r.Image.RegistryHint != "" {
		out = append(out, r.Image.RegistryHint)
	}
	if r.Pending != nil {
		if r.Pending.Delay != "" {
			out = append(out, "Scheduling delay: "+r.Pending.Delay)
		}
		out = append(out, r.Pending.ResourceRequests...)
	}
	return out
}

func (t textRenderer) suppressed(r *Report) []string {
	if r.SuppressedPods == 0 {
		return nil
	}
	out := []string{
		fmt.Sprintf(
			"⚠️ %d other pod(s) on this node also crashed (grouped to reduce "+
				"noise)",
			r.SuppressedPods,
		),
	}
	if n := len(r.SuppressedPodSummaries); n > 0 && n <= 5 {
		for _, ps := range r.SuppressedPodSummaries {
			out = append(
				out,
				fmt.Sprintf(
					"  %s/%s (%s)",
					ps.Namespace,
					ps.PodName,
					ps.Reason,
				),
			)
		}
	}
	return out
}

func (t textRenderer) evidence(r *Report) []string {
	if r.Evidence == nil {
		return nil
	}
	var out []string
	if r.Evidence.Events != "" {
		out = append(out, "Events:", t.m.code(r.Evidence.Events))
	}
	if r.Evidence.Logs != "" {
		out = append(out, "Logs:", t.m.code(r.Evidence.Logs))
	}
	return out
}

func joinNonEmpty(lines []string) string {
	kept := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			kept = append(kept, l)
		}
	}
	return strings.Join(kept, "\n")
}
