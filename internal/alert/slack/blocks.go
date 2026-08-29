package slack

import (
	"fmt"
	"strings"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"

	slackClient "github.com/slack-go/slack"
)

func buildIncidentBlocks(
	inc *model.Incident,
	appCfg *config.App,
) *slackClient.Blocks {
	return buildIncidentBlocksWithInsight(inc, appCfg, nil)
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
