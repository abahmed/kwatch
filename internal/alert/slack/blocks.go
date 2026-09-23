package slack

import (
	"strings"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/message"
	"github.com/abahmed/kwatch/internal/model"

	slackClient "github.com/slack-go/slack"
)

func buildIncidentBlocks(
	inc *model.Incident,
	clusterName string,
	timeSource clock.Clock,
) *slackClient.Blocks {
	return buildIncidentBlocksWithInsight(
		inc, clusterName, nil, timeSource,
	)
}

func buildIncidentBlocksWithInsight(
	inc *model.Incident,
	clusterName string,
	ins *insight.Insight,
	timeSource clock.Clock,
) *slackClient.Blocks {
	r := reportFor(inc, model.ActionCreate, ins, clusterName, timeSource)

	blocks := []slackClient.Block{markdownSection(headline(r))}

	// Narrative is shared with every other text provider. Slack only adds
	// Block Kit containers around it; it does not rebuild the diagnosis.
	if narrative := message.Narrative(r); narrative != "" {
		blocks = append(blocks, markdownSection(truncateField(narrative)))
	}
	if r.Timeline != "" {
		blocks = append(blocks, markdownSection("🕒 "+truncateField(r.Timeline)))
	}

	if changes := message.ChangeSummary(r); changes != "" {
		blocks = append(blocks, markdownSection(truncateField(changes)))
	}
	if r.Diagnosis != nil && r.Diagnosis.Hint != "" {
		blocks = append(blocks, markdownSection("💡 "+truncateField(r.Diagnosis.Hint)))
	}

	if c, ok := contextLine(metaParts(r, inc)); ok {
		blocks = append(blocks, c)
	}

	if inc.IncludeEvents {
		events := strings.TrimSpace(message.RedactEvidence(inc.Events))
		if events != "" {
			blocks = append(
				blocks,
				chunkedSections(
					evidenceTitle(":mag: *Events*", inc),
					events,
				)...)
		}
	}
	if inc.IncludeLogs {
		logs := strings.TrimSpace(message.RedactEvidence(inc.Logs))
		if logs != "" {
			blocks = append(
				blocks,
				chunkedSections(evidenceTitle(":memo: *Logs*", inc), logs)...)
		}
	}
	if r.Runbook != "" {
		blocks = append(blocks, markdownSection(
			"📖 "+truncateField(r.Runbook),
		))
	}

	return &slackClient.Blocks{
		BlockSet: capBlocks(append(blocks, markdownSection(constant.Footer))),
	}
}

func buildIncidentUpdateBlocks(
	inc *model.Incident,
	timeSource clock.Clock,
) *slackClient.Blocks {
	return buildIncidentUpdateBlocksWithInsight(inc, nil, timeSource)
}

func buildIncidentUpdateBlocksWithInsight(
	inc *model.Incident,
	ins *insight.Insight,
	timeSource clock.Clock,
) *slackClient.Blocks {
	r := reportFor(inc, model.ActionUpdate, ins, "", timeSource)

	// Updates land in the thread under the original alert, so they carry only
	// what moved: the headline, the current state, and the meta strip — as a
	// single block unless there is fresh evidence.
	text := headline(r)
	if narrative := message.Narrative(r); narrative != "" {
		text += "\n" + truncateField(narrative)
	}
	if r.Timeline != "" {
		text += "\n🕒 " + truncateField(r.Timeline)
	}
	if parts := metaParts(r, inc); len(parts) > 0 {
		text += "\n_" + truncateField(strings.Join(parts, "  ·  ")) + "_"
	}
	blocks := []slackClient.Block{markdownSection(text)}

	if inc.IncludeEvents {
		events := strings.TrimSpace(message.RedactEvidence(inc.Events))
		if events != "" {
			blocks = append(
				blocks,
				chunkedSections(
					evidenceTitle(":mag: *Events*", inc),
					events,
				)...)
		}
	}
	if inc.IncludeLogs {
		if logs := strings.TrimSpace(message.RedactEvidence(inc.Logs)); logs != "" {
			blocks = append(
				blocks,
				chunkedSections(evidenceTitle(":memo: *Logs*", inc), logs)...)
		}
	}
	return &slackClient.Blocks{BlockSet: capBlocks(blocks)}
}

// buildIncidentResolvedBlocks wraps the shared resolved rendering in a Block
// Kit section.
//
// It used to build its own headline. That second copy drifted: it dropped the
// reason when a friendly label existed, hard-coded the ✅ instead of using the
// resolved emoji the report carries, and pluralised the peak count as "pods"
// for node and volume incidents too. There is one resolved message now, and
// Slack only decides which container it goes in.
func buildIncidentResolvedBlocks(
	inc *model.Incident,
	timeSource clock.Clock,
) *slackClient.Blocks {
	r := reportFor(inc, model.ActionResolved, nil, "", timeSource)
	text := message.NewSlackRenderer().RenderResolved(r)
	return &slackClient.Blocks{
		BlockSet: []slackClient.Block{
			markdownSection(truncateField(text)),
		},
	}
}
