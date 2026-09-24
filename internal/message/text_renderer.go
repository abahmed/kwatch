package message

import (
	"fmt"
	"strings"
)

// markup is the only thing that differs between text providers: how to bold,
// how to fence code, and how to write a link. Everything else — what to say
// and in
// what order — is shared, so the 55 text providers cannot drift apart.
type markup struct {
	bold func(string) string
	code func(string) string
	// mono is inline code, for a single identifier inside a sentence. code
	// is a fenced block and cannot be used mid-line.
	mono func(string) string
	// italic marks a line as supporting detail rather than the message.
	italic func(string) string
	link   func(label, url string) string
	hint   string // prefix for the hint line
}

var (
	plainMarkup = markup{
		bold:   func(s string) string { return s },
		code:   func(s string) string { return s },
		mono:   func(s string) string { return s },
		italic: func(s string) string { return s },
		link:   func(label, url string) string { return label + ": " + url },
		hint:   "Hint:",
	}
	slackMarkup = markup{
		bold:   func(s string) string { return "*" + s + "*" },
		code:   func(s string) string { return "```" + s + "```" },
		mono:   func(s string) string { return "`" + s + "`" },
		italic: func(s string) string { return "_" + s + "_" },
		link: func(label, url string) string {
			return "<" + url + "|" + label + ">"
		},
		hint: "💡",
	}
	discordMarkup = markup{
		bold:   func(s string) string { return "**" + s + "**" },
		code:   func(s string) string { return "```\n" + s + "\n```" },
		mono:   func(s string) string { return "`" + s + "`" },
		italic: func(s string) string { return "*" + s + "*" },
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
	lines = append(lines, t.context(r))
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
	lines = append(lines, t.context(r))
	lines = append(lines, t.typeSpecific(r)...)
	lines = append(lines, t.evidence(r)...)
	return joinNonEmpty(lines)
}

// story turns the structured diagnosis into a short, natural explanation.
// It deliberately avoids exposing the internal field layout as a form.
func (t textRenderer) story(r *Report) string {
	return Narrative(r)
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
	var info []string
	if r.Summary.Duration != "" {
		info = append(info, "lasted "+r.Summary.Duration)
	}
	if r.Summary.Peak > 1 {
		info = append(info, fmt.Sprintf(
			"peak %d %s", r.Summary.Peak, resourcePlural(r),
		))
	}
	if r.Identity != nil && r.Identity.Node != "" {
		info = append(info, "node "+t.m.mono(r.Identity.Node))
	}
	if r.Resolution != nil {
		if r.Resolution.Summary != "" {
			info = append(info, r.Resolution.Summary)
		}
		if r.Resolution.Evidence != "" {
			info = append(info, r.Resolution.Evidence)
		}
	}
	if len(info) == 0 {
		return head
	}
	return head + "\n" + t.m.italic(strings.Join(info, " · "))
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

// resourcePlural names what a peak count counts. A node incident that said
// "peak 4 pods" was simply wrong.
func resourcePlural(r *Report) string {
	if r.Resource == "" {
		return "resources"
	}
	return r.Resource + "s"
}

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
	return h
}

// changes: what moved just before the incident, with how long before.
func (t textRenderer) changes(r *Report) string {
	return ChangeSummary(r)
}

// context adds only operationally useful details in sentence form. It avoids
// the fixed label row used by the previous template-style renderer.
func (t textRenderer) context(r *Report) string {
	var facts []string
	if r.Identity != nil {
		if r.Identity.Container != "" {
			facts = append(facts, "container "+t.m.mono(r.Identity.Container))
		}
		if r.Identity.Node != "" {
			facts = append(facts, "node "+t.m.mono(r.Identity.Node))
		}
	}
	if r.State != nil {
		if r.State.ExitCode > 0 {
			facts = append(facts, fmt.Sprintf("exit code %d", r.State.ExitCode))
		}
		if r.State.Restarts > 0 {
			facts = append(facts, fmt.Sprintf("%d restarts", r.State.Restarts))
		}
	}
	if r.Summary.Peak > 1 {
		facts = append(facts, fmt.Sprintf("%d affected %s",
			r.Summary.Peak, resourcePlural(r)))
	}
	if r.Summary.Duration != "" {
		facts = append(facts, "active for "+r.Summary.Duration)
	}
	if len(facts) == 0 {
		return ""
	}
	return capitalizeSentence(strings.Join(facts, ", ")) + "."
}

func (t textRenderer) typeSpecific(r *Report) []string {
	var out []string
	if r.OOM != nil {
		if r.OOM.MemoryLimit != "" {
			out = append(out,
				"🧠 The container memory limit is "+r.OOM.MemoryLimit+".")
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
			out = append(out,
				"Memory usage before the crash was "+r.OOM.Timeline+".")
		}
	}
	if r.Probe != nil {
		out = append(out, fmt.Sprintf(
			"🩺 The %s probe to %s is failing.",
			r.Probe.ProbeType, r.Probe.Endpoint,
		))
	}
	if r.Image != nil && r.Image.RegistryHint != "" {
		out = append(out, r.Image.RegistryHint)
	}
	if r.Pending != nil {
		if r.Pending.Delay != "" {
			out = append(out,
				"⏳ The pod has been waiting to schedule for "+
					r.Pending.Delay+".")
		}
		out = append(out, r.Pending.ResourceRequests...)
	}
	return out
}

// suppressedScope says what the suppressed pods have in common. A node
// incident speaks for the pods on that node; a workload incident speaks for
// its own pods, and telling the reader they were "on this node" was simply
// untrue.
func suppressedScope(r *Report) string {
	if r.Resource == "node" {
		return "on this node"
	}
	return "of this " + r.Resource
}

func (t textRenderer) suppressed(r *Report) []string {
	if r.SuppressedPods == 0 {
		return nil
	}
	out := []string{
		fmt.Sprintf(
			"🔗 %d other %s %s also failed; Kwatch grouped them here.",
			r.SuppressedPods,
			pluralWord(r.SuppressedPods, "pod", "pods"),
			suppressedScope(r),
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

func pluralWord(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}

func (t textRenderer) evidence(r *Report) []string {
	if r.Evidence == nil {
		return nil
	}
	var out []string
	if r.Evidence.Logs != "" {
		label := "Recent container logs"
		if r.Identity != nil && r.Identity.Container != "" {
			label += " from " + t.m.mono(r.Identity.Container)
		}
		out = append(out, label+":", t.m.code(r.Evidence.Logs))
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
