package slack

import (
	"fmt"
	"strings"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/message"
	"github.com/abahmed/kwatch/internal/model"

	slackClient "github.com/slack-go/slack"
)

// Slack Block Kit hard limits. Exceeding any one of them makes the API reject
// the entire message with invalid_blocks, so the alert is lost rather than
// degraded. Every limit below is enforced, not assumed.
const (
	maxFieldsPerSection = 10
	maxFieldChars       = 2000
	maxBlocksPerMessage = 50
)

// truncateField shortens s to Slack's per-field limit. It slices on rune
// boundaries so a multi-byte character is never split into invalid UTF-8.
func truncateField(s string) string {
	const maxChars = maxFieldChars
	r := []rune(s)
	if len(r) <= maxChars {
		return s
	}
	const ellipsis = "..."
	if maxChars <= len(ellipsis) {
		return string(r[:maxChars])
	}
	return string(r[:maxChars-len(ellipsis)]) + ellipsis
}

// capBlocks keeps a message within Slack's block limit, reserving the last
// slot for a marker so a trimmed alert says so instead of quietly dropping
// evidence.
func capBlocks(blocks []slackClient.Block) []slackClient.Block {
	if len(blocks) <= maxBlocksPerMessage {
		return blocks
	}
	kept := blocks[:maxBlocksPerMessage-1]
	omitted := len(blocks) - len(kept)
	return append(kept, markdownSection(
		fmt.Sprintf(
			"_%d more block(s) omitted to stay within Slack's limit._",
			omitted,
		),
	))
}

// evidenceTitle names the pod that logs and events were collected from, when
// the incident covers more than that one pod. An incident is keyed by owner,
// so it can name several replicas under Resources while the evidence below
// comes from exactly one of them — saying which removes the contradiction.
func evidenceTitle(title string, inc *model.Incident) string {
	pod := inc.EvidencePod
	if pod == "" {
		return title
	}
	if len(inc.Resources) <= 1 && inc.Name == pod {
		return title
	}
	return fmt.Sprintf("%s — from `%s`", title, pod)
}

// chunkedSections renders a titled section whose body is split into
// fixed-size code blocks.
func chunkedSections(title, text string) []slackClient.Block {
	blocks := []slackClient.Block{markdownSection(title)}
	for _, chunk := range util.Chunks(text, chunkSize) {
		blocks = append(blocks, markdownSection("```"+chunk+"```"))
	}
	return blocks
}

func buildIncidentBlocks(
	inc *model.Incident,
	appCfg *config.App,
) *slackClient.Blocks {
	return buildIncidentBlocksWithInsight(inc, appCfg, nil)
}

func diagnosisFromParts(
	cause, pattern, impact string,
	changes []string,
) (slackClient.Block, bool) {
	var lines []string
	if cause != "" {
		line := "• *Why:* " + cause
		if pattern != "" {
			line += fmt.Sprintf(" _(%s)_", pattern)
		}
		lines = append(lines, line)
	}
	if impact != "" {
		lines = append(lines, "• *Impact:* "+impact)
	}
	if len(changes) > 0 {
		lines = append(
			lines,
			"• *Changed recently:* "+strings.Join(changes, "; "),
		)
	}
	if len(lines) == 0 {
		return nil, false
	}
	return markdownSection("🧠 *Diagnosis*\n" + strings.Join(lines, "\n")), true
}

// contextLine renders a compact "meta" strip: small grey text under the
// message, where Slack puts things that support the alert without competing
// with it.
func contextLine(parts []string) (slackClient.Block, bool) {
	var kept []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			kept = append(kept, p)
		}
	}
	if len(kept) == 0 {
		return nil, false
	}
	text := truncateField(strings.Join(kept, "  ·  "))
	return slackClient.NewContextBlock(
		"",
		slackClient.NewTextBlockObject(
			slackClient.MarkdownType,
			text,
			false,
			false,
		),
	), true
}

func reportFor(
	inc *model.Incident,
	action model.IncidentAction,
	ins *insight.Insight,
	appCfg *config.App,
) *message.Report {
	cluster := ""
	if appCfg != nil {
		cluster = appCfg.ClusterName
	}
	return message.NewReportBuilder(cluster).Build(inc, action, ins)
}

// headline is the one line a reader sees first: what happened, to what.
//
//	🚨 *Pod not ready* — `dev/api` · Deployment · high
func headline(r *message.Report) string {
	label := r.Summary.Label
	if label == "" {
		label = r.Reason
	}
	h := fmt.Sprintf("%s *%s*", r.Summary.Emoji, label)
	subject := r.Name
	if r.Namespace != "" && !strings.HasPrefix(subject, r.Namespace+"/") &&
		!strings.Contains(subject, " ") {
		subject = r.Namespace + "/" + subject
	}
	if subject != "" {
		h += " — " + subject
	}
	// A single workload gets its kind; a group summary is already a sentence
	// about several, and the first member's kind would mislabel it.
	if r.Identity != nil && r.Identity.OwnerKind != "" &&
		!strings.Contains(
			subject,
			r.Identity.OwnerKind,
		) && !strings.Contains(subject, " ") {
		h += " · " + r.Identity.OwnerKind
	}
	if label != r.Reason {
		h += fmt.Sprintf(" · `%s`", r.Reason)
	}
	if r.Severity != "" && r.Severity != "normal" {
		h += " · *" + r.Severity + "*"
	}
	return h
}

// metaParts is everything that used to be a 14-field grid, as one line.
func metaParts(r *message.Report, inc *model.Incident) []string {
	var parts []string
	if inc.EvidencePod != "" && len(inc.Resources) <= 1 {
		parts = append(parts, "pod `"+inc.EvidencePod+"`")
	} else if n := len(inc.Resources); n > 1 {
		parts = append(parts, fmt.Sprintf("%d pods", n))
	}
	if r.Identity != nil {
		if r.Identity.Container != "" && r.Identity.Container != inc.Name {
			parts = append(parts, "container `"+r.Identity.Container+"`")
		}
		if r.Identity.Image != "" {
			parts = append(parts, "image `"+r.Identity.Image+"`")
		}
		if r.Identity.Node != "" {
			parts = append(parts, "node `"+r.Identity.Node+"`")
		}
	}
	if r.State != nil {
		if r.State.Restarts > 0 {
			parts = append(parts, fmt.Sprintf("restarts %d", r.State.Restarts))
		}
		if r.State.ExitCode > 0 {
			parts = append(parts, fmt.Sprintf("exit %d", r.State.ExitCode))
		}
	}
	if r.Summary.Count > 1 {
		parts = append(parts, fmt.Sprintf("seen ×%d", r.Summary.Count))
	}
	if r.Summary.Duration != "" {
		parts = append(parts, r.Summary.Duration)
	}
	if r.Cluster != "" {
		parts = append(parts, r.Cluster)
	}
	return parts
}

func buildIncidentBlocksWithInsight(
	inc *model.Incident,
	appCfg *config.App,
	ins *insight.Insight,
) *slackClient.Blocks {
	r := reportFor(inc, model.ActionCreate, ins, appCfg)

	blocks := []slackClient.Block{markdownSection(headline(r))}

	// What, in one sentence — the state message carries the specifics
	// ("pod stopped being ready 3h ago", "exit code 137").
	if r.State != nil && r.State.Message != "" {
		blocks = append(blocks, markdownSection(truncateField(r.State.Message)))
	}

	// Why / impact / what changed — from the graph.
	var changes []string
	if r.Changes != nil {
		for i, c := range r.Changes.Items {
			if i == 3 {
				changes = append(
					changes,
					fmt.Sprintf("+%d more", len(r.Changes.Items)-3),
				)
				break
			}
			line := fmt.Sprintf("%s %s %s", c.Resource, c.Reference, c.Type)
			if c.Age != "" {
				line += " " + c.Age + " ago"
			}
			changes = append(changes, line)
		}
	}
	if r.Diagnosis != nil {
		if d, ok := diagnosisFromParts(
			r.Diagnosis.Cause,
			r.Diagnosis.Pattern,
			r.Diagnosis.Impact,
			changes,
		); ok {
			blocks = append(blocks, d)
		}
		if r.Diagnosis.Hint != "" {
			blocks = append(
				blocks,
				markdownSection("💡 "+truncateField(r.Diagnosis.Hint)),
			)
		}
	}

	if c, ok := contextLine(metaParts(r, inc)); ok {
		blocks = append(blocks, c)
	}

	if inc.IncludeEvents {
		if events := strings.TrimSpace(inc.Events); events != "" {
			blocks = append(
				blocks,
				chunkedSections(
					evidenceTitle(":mag: *Events*", inc),
					events,
				)...)
		}
	}
	if inc.IncludeLogs {
		if logs := strings.TrimSpace(inc.Logs); logs != "" {
			blocks = append(
				blocks,
				chunkedSections(evidenceTitle(":memo: *Logs*", inc), logs)...)
		}
	}
	if r.Runbook != "" {
		blocks = append(blocks, markdownSection("📖 "+r.Runbook))
	}

	return &slackClient.Blocks{
		BlockSet: capBlocks(append(blocks, markdownSection(constant.Footer))),
	}
}

func buildIncidentUpdateBlocks(inc *model.Incident) *slackClient.Blocks {
	return buildIncidentUpdateBlocksWithInsight(inc, nil)
}

func buildIncidentUpdateBlocksWithInsight(
	inc *model.Incident,
	ins *insight.Insight,
) *slackClient.Blocks {
	r := reportFor(inc, model.ActionUpdate, ins, nil)

	// Updates land in the thread under the original alert, so they carry only
	// what moved: the headline, the current state, and the meta strip — as a
	// single block unless there is fresh evidence.
	text := headline(r)
	if r.State != nil && r.State.Message != "" {
		text += "\n" + truncateField(r.State.Message)
	}
	// The cause can change mid-incident (a node blamed at first, a rollout
	// found later). A follow-up carries the current one so the thread does
	// not go stale; impact and history stay on the original alert.
	if r.Diagnosis != nil && r.Diagnosis.Cause != "" {
		why := "🧠 *Why:* " + r.Diagnosis.Cause
		if r.Diagnosis.Pattern != "" {
			why += fmt.Sprintf(" _(%s)_", r.Diagnosis.Pattern)
		}
		text += "\n" + truncateField(why)
	}
	if parts := metaParts(r, inc); len(parts) > 0 {
		text += "\n_" + truncateField(strings.Join(parts, "  ·  ")) + "_"
	}
	blocks := []slackClient.Block{markdownSection(text)}

	if inc.IncludeEvents {
		if events := strings.TrimSpace(inc.Events); events != "" {
			blocks = append(
				blocks,
				chunkedSections(
					evidenceTitle(":mag: *Events*", inc),
					events,
				)...)
		}
	}
	if inc.IncludeLogs {
		if logs := strings.TrimSpace(inc.Logs); logs != "" {
			blocks = append(
				blocks,
				chunkedSections(evidenceTitle(":memo: *Logs*", inc), logs)...)
		}
	}
	return &slackClient.Blocks{BlockSet: capBlocks(blocks)}
}

func buildIncidentResolvedBlocks(inc *model.Incident) *slackClient.Blocks {
	r := reportFor(inc, model.ActionResolved, nil, nil)
	label := r.Summary.Label
	if label == "" {
		label = r.Reason
	}
	subject := r.Name
	if r.Namespace != "" && !strings.HasPrefix(subject, r.Namespace+"/") &&
		!strings.Contains(subject, " ") {
		subject = r.Namespace + "/" + subject
	}
	header := fmt.Sprintf("✅ *Resolved — %s*", label)
	if subject != "" {
		header += " — " + subject
	}

	var info []string
	if r.Summary.Duration != "" {
		info = append(info, "lasted "+r.Summary.Duration)
	}
	if r.Summary.Count > 1 {
		info = append(info, fmt.Sprintf("%d occurrences", r.Summary.Count))
	}
	if r.Summary.Peak > 1 {
		info = append(
			info,
			fmt.Sprintf("peak %d %s", r.Summary.Peak, resourcePlural(inc)),
		)
	}
	if r.Identity != nil && r.Identity.Node != "" {
		info = append(info, "node `"+r.Identity.Node+"`")
	}
	text := header
	if len(info) > 0 {
		text += "\n_" + strings.Join(info, "  ·  ") + "_"
	}
	return &slackClient.Blocks{
		BlockSet: []slackClient.Block{markdownSection(text)},
	}
}

func formatIncidentText(
	inc *model.Incident,
	action model.IncidentAction,
) string {
	renderer := message.NewSlackRenderer()
	report := message.NewReportBuilder("").Build(inc, action, nil)
	return message.RenderAction(renderer, report)
}

func resourcePlural(inc *model.Incident) string {
	if inc.Resource != "" {
		return inc.Resource + "s"
	}
	return "resources"
}

func plainSection(txt string) slackClient.SectionBlock {
	return slackClient.SectionBlock{
		Type: "section",
		Text: slackClient.NewTextBlockObject(
			slackClient.PlainTextType,
			txt,
			true,
			false),
	}
}

func markdownSection(txt string) slackClient.SectionBlock {
	return slackClient.SectionBlock{
		Type: "section",
		Text: slackClient.NewTextBlockObject(
			slackClient.MarkdownType,
			txt,
			false,
			true),
	}
}

func markdownF(format string, a ...interface{}) *slackClient.TextBlockObject {
	return slackClient.NewTextBlockObject(
		slackClient.MarkdownType,
		truncateField(fmt.Sprintf(format, a...)),
		false,
		true)
}
