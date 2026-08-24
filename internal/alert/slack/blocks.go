package slack

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/alert/util"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/message"
	"github.com/abahmed/kwatch/internal/model"

	slackClient "github.com/slack-go/slack"
)

// resourcesField renders the grouped-resources field value along with whether
// the field should appear at all.
func resourcesField(inc *model.Incident) (string, bool) {
	resources := make([]string, 0, len(inc.Resources))
	for r := range inc.Resources {
		resources = append(resources, r)
	}
	resourcesStr := strings.Join(resources, ", ")
	if len(resourcesStr) > 200 {
		resourcesStr = resourcesStr[:200] + "..."
	}
	if len(resources) == 0 && inc.PeakResources == 0 {
		return "", false
	}
	peak := ""
	if inc.PeakResources > 0 {
		peak = fmt.Sprintf(" (Peak: %d)", inc.PeakResources)
	}
	return fmt.Sprintf("*Resources%s*\n%s", peak, resourcesStr), true
}

func incidentFields(inc *model.Incident, appCfg *config.App, duration time.Duration) []*slackClient.TextBlockObject {
	fields := []*slackClient.TextBlockObject{
		markdownF("*Cluster*\n%s", appCfg.ClusterName),
		markdownF("*Name*\n%s", inc.Name),
	}
	if inc.OwnerKind != "" {
		fields = append(fields, markdownF("*Kind*\n%s", inc.OwnerKind))
	}
	if inc.Namespace != "" {
		fields = append(fields, markdownF("*Namespace*\n%s", inc.Namespace))
	}
	containerName := containerSummary(inc)
	if containerName != "" {
		fields = append(fields, markdownF("*Container*\n%s", containerName))
	}
	if inc.Image != "" {
		fields = append(fields, markdownF("*Image*\n%s", inc.Image))
	}
	fields = append(fields, markdownF("*Reason*\n%s", inc.Reason))
	if inc.NodeName != "" {
		fields = append(fields, markdownF("*Node*\n%s", inc.NodeName))
	}
	if inc.LastContainerState != nil && inc.LastContainerState.ExitCode > 0 {
		fields = append(fields, markdownF("*Exit Code*\n%d", inc.LastContainerState.ExitCode))
	}
	if inc.LastContainerState != nil && inc.LastContainerState.Msg != "" {
		fields = append(fields, markdownF("*Message*\n%s", inc.LastContainerState.Msg))
	}
	if inc.RestartCount > 0 {
		fields = append(fields, markdownF("*Restarts*\n%d", inc.RestartCount))
	}
	fields = append(fields, markdownF("*Count*\n%d", inc.Count))

	if res, ok := resourcesField(inc); ok {
		fields = append(fields, markdownF("%s", res))
	}

	return append(fields, markdownF("*Duration*\n%s", duration))
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

func buildIncidentBlocks(inc *model.Incident, appCfg *config.App) *slackClient.Blocks {
	duration := inc.LastSeen.Sub(inc.FirstSeen).Round(time.Minute)

	blocks := []slackClient.Block{
		markdownSection(fmt.Sprintf("🚨 *%s*", inc.Reason)),
		slackClient.SectionBlock{
			Type:   "section",
			Fields: incidentFields(inc, appCfg, duration),
		},
	}

	if inc.Hint != "" {
		blocks = append(blocks, markdownSection("💡 "+inc.Hint))
	}

	if inc.IncludeEvents {
		events := strings.TrimSpace(inc.Events)
		if len(events) > 0 {
			blocks = append(blocks, chunkedSections(":mag: *Events*", events)...)
		}
	}

	if inc.IncludeLogs {
		logs := strings.TrimSpace(inc.Logs)
		if len(logs) > 0 {
			blocks = append(blocks, chunkedSections(":memo: *Logs*", logs)...)
		}
	}

	return &slackClient.Blocks{
		BlockSet: append(blocks, markdownSection(constant.Footer)),
	}
}

func buildIncidentUpdateBlocks(inc *model.Incident) *slackClient.Blocks {
	duration := inc.LastSeen.Sub(inc.FirstSeen).Round(time.Minute)

	header := fmt.Sprintf("🔄 *%s*", inc.Reason)
	if inc.Name != "" {
		header += " — " + inc.Name
	}
	if inc.Namespace != "" {
		header += fmt.Sprintf(" (%s)", inc.Namespace)
	}

	var infoParts []string
	containerName := containerSummary(inc)
	if containerName != "" {
		infoParts = append(infoParts, fmt.Sprintf("Container: %s", containerName))
	}
	if inc.Image != "" {
		infoParts = append(infoParts, fmt.Sprintf("Image: %s", inc.Image))
	}
	if inc.NodeName != "" {
		infoParts = append(infoParts, fmt.Sprintf("Node: %s", inc.NodeName))
	}
	if inc.LastContainerState != nil && inc.LastContainerState.Msg != "" {
		infoParts = append(infoParts, fmt.Sprintf("Message: %s", inc.LastContainerState.Msg))
	}
	if inc.LastContainerState != nil && inc.LastContainerState.ExitCode > 0 {
		infoParts = append(infoParts, fmt.Sprintf("Exit Code: %d", inc.LastContainerState.ExitCode))
	}
	if inc.OwnerKind != "" {
		infoParts = append(infoParts, fmt.Sprintf("Kind: %s", inc.OwnerKind))
	}
	infoParts = append(infoParts, fmt.Sprintf("Count: %d", inc.Count))
	infoParts = append(infoParts, fmt.Sprintf("Duration: %s", duration))
	if inc.RestartCount > 0 {
		infoParts = append(infoParts, fmt.Sprintf("Restarts: %d", inc.RestartCount))
	}
	if inc.PeakResources > 0 {
		infoParts = append(infoParts, fmt.Sprintf("Peak: %d %s", inc.PeakResources, resourcePlural(inc)))
	}

	blocks := []slackClient.Block{
		markdownSection(header + "\n" + strings.Join(infoParts, " · ")),
	}

	if inc.Hint != "" {
		blocks = append(blocks, markdownSection("💡 "+inc.Hint))
	}

	if inc.IncludeEvents {
		events := strings.TrimSpace(inc.Events)
		if len(events) > 0 {
			blocks = append(blocks, markdownSection(":mag: *Events*"))
			for _, chunk := range util.Chunks(events, chunkSize) {
				blocks = append(blocks, markdownSection("```"+chunk+"```"))
			}
		}
	}

	if inc.IncludeLogs {
		logs := strings.TrimSpace(inc.Logs)
		if len(logs) > 0 {
			blocks = append(blocks, markdownSection(":memo: *Logs*"))
			for _, chunk := range util.Chunks(logs, chunkSize) {
				blocks = append(blocks, markdownSection("```"+chunk+"```"))
			}
		}
	}

	return &slackClient.Blocks{
		BlockSet: blocks,
	}
}

func buildIncidentResolvedBlocks(inc *model.Incident) *slackClient.Blocks {
	duration := inc.LastSeen.Sub(inc.FirstSeen).Round(time.Minute)

	header := fmt.Sprintf("✅ *Resolved* — *%s*", inc.Reason)
	if inc.Resource != "" && inc.Name != "" {
		header += fmt.Sprintf(" in %s/%s", inc.Resource, inc.Name)
	} else if inc.Name != "" {
		header += " — " + inc.Name
	}
	if inc.Namespace != "" {
		header += fmt.Sprintf(" (%s)", inc.Namespace)
	}

	var infoParts []string
	infoParts = append(infoParts, fmt.Sprintf("Duration: %s", duration))
	if inc.NodeName != "" {
		infoParts = append(infoParts, fmt.Sprintf("Node: %s", inc.NodeName))
	}
	infoParts = append(infoParts, fmt.Sprintf("Total events: %d", inc.Count))
	if inc.PeakResources > 0 {
		infoParts = append(infoParts, fmt.Sprintf("Peak: %d %s", inc.PeakResources, resourcePlural(inc)))
	}

	text := header + "\n" + strings.Join(infoParts, " · ")

	return &slackClient.Blocks{
		BlockSet: []slackClient.Block{
			markdownSection(text),
		},
	}
}

func formatIncidentText(inc *model.Incident, action model.IncidentAction) string {
	renderer := message.NewSlackRenderer()
	report := message.NewReportBuilder("").Build(inc, action, nil)
	return message.RenderAction(renderer, report)
}

func containerSummary(inc *model.Incident) string {
	if len(inc.Containers) > 1 {
		names := make([]string, 0, len(inc.Containers))
		for c := range inc.Containers {
			names = append(names, c)
		}
		sort.Strings(names)
		return strings.Join(names, ", ")
	}
	if inc.ContainerName != "" && inc.ContainerName != "." {
		return inc.ContainerName
	}
	return ""
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
		fmt.Sprintf(format, a...),
		false,
		true)
}
