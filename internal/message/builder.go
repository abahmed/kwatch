package message

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/context"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
)

type Builder struct {
	now func() time.Time
}

func NewBuilder() *Builder {
	return &Builder{now: time.Now}
}

func (b *Builder) Build(inc *model.Incident, action model.IncidentAction, ins *insight.Insight) string {
	switch action {
	case model.ActionCreate:
		return b.buildCreate(inc, ins)
	case model.ActionUpdate:
		return b.buildUpdate(inc, ins)
	case model.ActionResolved:
		return b.buildResolved(inc)
	default:
		return ""
	}
}

func (b *Builder) buildCreate(inc *model.Incident, ins *insight.Insight) string {
	var sections []string

	header := fmt.Sprintf("%s", inc.Reason)
	if inc.Resource != "" && inc.Name != "" {
		header += fmt.Sprintf(" in %s/%s", inc.Resource, inc.Name)
	} else if inc.Name != "" {
		header += " — " + inc.Name
	}
	if inc.Namespace != "" {
		header += fmt.Sprintf(" (%s)", inc.Namespace)
	}
	if s := severityLabel(inc.Severity); s != "normal" {
		header += " · " + s
	}
	sections = append(sections, header)

	var infoParts []string
	if inc.ContainerName != "" && inc.ContainerName != "." {
		infoParts = append(infoParts, fmt.Sprintf("Container: %s", inc.ContainerName))
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
	if inc.RestartCount > 0 {
		infoParts = append(infoParts, fmt.Sprintf("Restarts: %d", inc.RestartCount))
	}
	infoParts = append(infoParts, fmt.Sprintf("Count: %d", inc.Count))
	infoParts = append(infoParts, fmt.Sprintf("Duration: %s", durationStr(inc.FirstSeen, inc.LastSeen)))
	if inc.PeakResources > 0 {
		infoParts = append(infoParts, fmt.Sprintf("Peak: %d", inc.PeakResources))
	}
	sections = append(sections, strings.Join(infoParts, " · "))

	if inc.Hint != "" {
		sections = append(sections, "💡 "+inc.Hint)
	}

	if ins != nil {
		if ins.Pattern != "" {
			sections = append(sections, fmt.Sprintf("Pattern: %s", ins.Pattern))
		}
		if ins.Cause != "" {
			sections = append(sections, "Likely cause: "+ins.Cause)
		}
		if ins.Impact != "" {
			sections = append(sections, "Impact: "+ins.Impact)
		}
		if ins.AffectedCount > 0 {
			sections = append(sections, fmt.Sprintf("Affected resources: %d", ins.AffectedCount))
		}
		if len(ins.RecentChanges) > 0 {
			sections = append(sections, formatChanges(ins.RecentChanges))
		}
	}

	if inc.IncludeLogs && inc.Logs != "" {
		sections = append(sections, "\nLogs:\n"+inc.Logs)
	}

	if inc.IncludeEvents && inc.Events != "" {
		sections = append(sections, "\nEvents:\n"+inc.Events)
	}

	if inc.Runbook != "" {
		sections = append(sections, "📖 Runbook: "+inc.Runbook)
	}

	return strings.Join(sections, "\n")
}

func (b *Builder) buildUpdate(inc *model.Incident, ins *insight.Insight) string {
	var sections []string

	header := fmt.Sprintf("Update — %s", inc.Reason)
	if inc.Name != "" {
		header += " — " + inc.Name
	}
	if inc.Namespace != "" {
		header += fmt.Sprintf(" (%s)", inc.Namespace)
	}
	sections = append(sections, header)

	var infoParts []string
	if inc.Resource == "pod" && inc.ContainerName != "" && inc.ContainerName != "." {
		infoParts = append(infoParts, fmt.Sprintf("Container: %s", inc.ContainerName))
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
	if inc.RestartCount > 0 {
		infoParts = append(infoParts, fmt.Sprintf("Restarts: %d", inc.RestartCount))
	}
	infoParts = append(infoParts, fmt.Sprintf("Count: %d", inc.Count))
	infoParts = append(infoParts, fmt.Sprintf("Duration: %s", durationStr(inc.FirstSeen, inc.LastSeen)))
	if inc.PeakResources > 0 {
		infoParts = append(infoParts, fmt.Sprintf("Peak: %d", inc.PeakResources))
	}
	sections = append(sections, strings.Join(infoParts, " · "))

	if inc.Hint != "" {
		sections = append(sections, "💡 "+inc.Hint)
	}

	if ins != nil && ins.Cause != "" {
		sections = append(sections, "Likely cause: "+ins.Cause)
	}

	if inc.IncludeLogs && inc.Logs != "" {
		sections = append(sections, "\nLogs:\n"+inc.Logs)
	}

	if inc.IncludeEvents && inc.Events != "" {
		sections = append(sections, "\nEvents:\n"+inc.Events)
	}

	return strings.Join(sections, "\n")
}

func (b *Builder) buildResolved(inc *model.Incident) string {
	header := fmt.Sprintf("Resolved — %s", inc.Reason)
	if inc.Resource != "" && inc.Name != "" {
		header += fmt.Sprintf(" in %s/%s", inc.Resource, inc.Name)
	} else if inc.Name != "" {
		header += " — " + inc.Name
	}
	if inc.Namespace != "" {
		header += fmt.Sprintf(" (%s)", inc.Namespace)
	}

	var infoParts []string
	infoParts = append(infoParts, fmt.Sprintf("Duration: %s", durationStr(inc.FirstSeen, inc.LastSeen)))
	if inc.NodeName != "" {
		infoParts = append(infoParts, fmt.Sprintf("Node: %s", inc.NodeName))
	}
	if inc.LastContainerState != nil && inc.LastContainerState.ExitCode > 0 {
		infoParts = append(infoParts, fmt.Sprintf("Exit Code: %d", inc.LastContainerState.ExitCode))
	}
	if inc.OwnerKind != "" {
		infoParts = append(infoParts, fmt.Sprintf("Kind: %s", inc.OwnerKind))
	}
	infoParts = append(infoParts, fmt.Sprintf("Total events: %d", inc.Count))
	if inc.PeakResources > 0 {
		infoParts = append(infoParts, fmt.Sprintf("Peak: %d", inc.PeakResources))
	}
	return fmt.Sprintf("%s\n%s", header, strings.Join(infoParts, " · "))
}

func severityLabel(s string) string {
	if s == "" {
		return "normal"
	}
	return s
}

func durationStr(first, last time.Time) string {
	d := last.Sub(first).Round(time.Minute)
	if d < time.Minute {
		return "< 1m"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

func firstKey(m map[string]bool) string {
	for k := range m {
		return k
	}
	return ""
}

func formatChanges(changes []context.Change) string {
	if len(changes) == 0 {
		return ""
	}
	var parts []string
	seen := make(map[string]bool)
	for _, c := range changes {
		key := c.Resource + "/" + c.Namespace + "/" + c.Name + c.Type.String()
		if seen[key] {
			continue
		}
		seen[key] = true
		ref := c.Name
		if c.Namespace != "" {
			ref = c.Namespace + "/" + c.Name
		}
		parts = append(parts, fmt.Sprintf("  %s %s %s", c.Resource, ref, c.Type))
	}
	if len(parts) == 0 {
		return ""
	}
	sort.Strings(parts)
	return "Recent changes:\n" + strings.Join(parts, "\n")
}
