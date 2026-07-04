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

	sections = append(sections, fmt.Sprintf("Incident: %s | Severity: %s",
		inc.Name, severityLabel(inc.Severity)))

	sections = append(sections, fmt.Sprintf("Namespace: %s | Resource: %s | Reason: %s",
		inc.Namespace, inc.Resource, inc.Reason))

	sections = append(sections, fmt.Sprintf("Count: %d | Duration: %s",
		inc.Count, durationStr(inc.FirstSeen, inc.LastSeen)))

	if inc.Hint != "" {
		sections = append(sections, "Hint: "+inc.Hint)
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

	if inc.Runbook != "" {
		sections = append(sections, "Runbook: "+inc.Runbook)
	}

	if inc.Resource == "pod" && len(inc.Resources) > 0 {
		pod := firstKey(inc.Resources)
		cmd := fmt.Sprintf("kubectl -n %s logs %s --previous", inc.Namespace, pod)
		if inc.ContainerName != "" {
			cmd = fmt.Sprintf("kubectl -n %s logs %s -c %s --previous", inc.Namespace, pod, inc.ContainerName)
		}
		sections = append(sections, "Investigate: "+cmd)
	}

	return strings.Join(sections, "\n")
}

func (b *Builder) buildUpdate(inc *model.Incident, ins *insight.Insight) string {
	var sections []string

	sections = append(sections, fmt.Sprintf("Update: %s | Severity: %s",
		inc.Name, severityLabel(inc.Severity)))

	sections = append(sections, fmt.Sprintf("Namespace: %s | Resource: %s | Reason: %s",
		inc.Namespace, inc.Resource, inc.Reason))

	sections = append(sections, fmt.Sprintf("Count: %d | Duration: %s",
		inc.Count, durationStr(inc.FirstSeen, inc.LastSeen)))

	if ins != nil && ins.Cause != "" {
		sections = append(sections, "Likely cause: "+ins.Cause)
	}

	return strings.Join(sections, "\n")
}

func (b *Builder) buildResolved(inc *model.Incident) string {
	return fmt.Sprintf("Resolved: %s | Namespace: %s | Resource: %s | Reason: %s | Duration: %s | Events: %d",
		inc.Name, inc.Namespace, inc.Resource, inc.Reason,
		durationStr(inc.FirstSeen, inc.LastSeen), inc.Count)
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
