package slack

import (
	"fmt"
	"strings"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
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
