package slack

import (
	"strings"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/message"
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

// buildIncidentResolvedBlocks wraps the shared resolved rendering in a Block
// Kit section.
//
// It used to build its own headline. That second copy drifted: it dropped the
// reason when a friendly label existed, hard-coded the ✅ instead of using the
// resolved emoji the report carries, and pluralised the peak count as "pods"
// for node and volume incidents too. There is one resolved message now, and
// Slack only decides which container it goes in.
func buildIncidentResolvedBlocks(inc *model.Incident) *slackClient.Blocks {
	r := reportFor(inc, model.ActionResolved, nil, nil)
	text := message.NewSlackRenderer().RenderResolved(r)
	return &slackClient.Blocks{
		BlockSet: []slackClient.Block{
			markdownSection(truncateField(text)),
		},
	}
}
