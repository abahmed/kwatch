package event

import (
	"fmt"
	"strings"
)

func (e *Event) FormatMarkdown(clusterName, text, delimiter string) string {
	if text == "" {
		text = defaultEventText(e)
	}
	if delimiter == "" {
		delimiter = "\n"
	}

	var parts []string
	parts = append(parts, text)

	if clusterName != "" {
		parts = append(parts, fmt.Sprintf("**Cluster:** %s", clusterName))
	}
	if e.PodName != "" {
		parts = append(parts, fmt.Sprintf("**Pod:** %s", e.PodName))
	}
	if e.ContainerName != "" {
		parts = append(parts, fmt.Sprintf("**Container:** %s", e.ContainerName))
	}
	if e.Namespace != "" {
		parts = append(parts, fmt.Sprintf("**Namespace:** %s", e.Namespace))
	}
	if e.NodeName != "" {
		parts = append(parts, fmt.Sprintf("**Node:** %s", e.NodeName))
	}
	if e.Reason != "" {
		parts = append(parts, fmt.Sprintf("**Reason:** %s", e.Reason))
	}

	if e.IncludeEvents {
		events := strings.TrimSpace(e.Events)
		if len(events) > 0 {
			parts = append(parts, "**Events:**\n```\n"+events+"\n```")
		}
	}

	if e.IncludeLogs {
		logs := strings.TrimSpace(e.Logs)
		if len(logs) > 0 {
			parts = append(parts, "**Logs:**\n```\n"+logs+"\n```")
		}
	}

	return strings.Join(parts, delimiter)
}

func (e *Event) FormatHtml(clusterName, text string) string {
	if text == "" {
		text = defaultEventText(e)
	}

	var parts []string
	parts = append(parts, text)

	if clusterName != "" {
		parts = append(parts, fmt.Sprintf("<b>Cluster:</b> %s", clusterName))
	}
	if e.PodName != "" {
		parts = append(parts, fmt.Sprintf("<b>Pod:</b> %s", e.PodName))
	}
	if e.ContainerName != "" {
		parts = append(parts, fmt.Sprintf("<b>Container:</b> %s", e.ContainerName))
	}
	if e.Namespace != "" {
		parts = append(parts, fmt.Sprintf("<b>Namespace:</b> %s", e.Namespace))
	}
	if e.NodeName != "" {
		parts = append(parts, fmt.Sprintf("<b>Node:</b> %s", e.NodeName))
	}
	if e.Reason != "" {
		parts = append(parts, fmt.Sprintf("<b>Reason:</b> %s", e.Reason))
	}

	if e.IncludeEvents {
		events := strings.TrimSpace(e.Events)
		if len(events) > 0 {
			parts = append(parts, "<b>Events:</b><br/><blockquote>"+strings.ReplaceAll(events, "\n", "<br/>")+"</blockquote>")
		}
	}

	if e.IncludeLogs {
		logs := strings.TrimSpace(e.Logs)
		if len(logs) > 0 {
			parts = append(parts, "<b>Logs:</b><br/><blockquote>"+strings.ReplaceAll(logs, "\n", "<br/>")+"</blockquote>")
		}
	}

	return strings.Join(parts, "<br/>")
}

func (e *Event) FormatText(clusterName, text string) string {
	if text == "" {
		text = defaultEventText(e)
	}

	var parts []string
	parts = append(parts, text)
	parts = append(parts, "")

	if clusterName != "" {
		parts = append(parts, fmt.Sprintf("cluster: %s", clusterName))
	}
	if e.PodName != "" {
		parts = append(parts, fmt.Sprintf("Pod Name: %s", e.PodName))
	}
	if e.ContainerName != "" {
		parts = append(parts, fmt.Sprintf("Container: %s", e.ContainerName))
	}
	if e.Namespace != "" {
		parts = append(parts, fmt.Sprintf("Namespace: %s", e.Namespace))
	}
	if e.NodeName != "" {
		parts = append(parts, fmt.Sprintf("Node: %s", e.NodeName))
	}
	if e.Reason != "" {
		parts = append(parts, fmt.Sprintf("Reason: %s", e.Reason))
	}

	parts = append(parts, "")

	if e.IncludeEvents {
		events := strings.TrimSpace(e.Events)
		if len(events) > 0 {
			parts = append(parts, "Events:\n"+events)
		}
	}

	if e.IncludeLogs {
		logs := strings.TrimSpace(e.Logs)
		if len(logs) > 0 {
			parts = append(parts, "Logs:\n"+logs)
		}
	}

	return strings.Join(parts, "\n")
}

func defaultEventText(e *Event) string {
	if e.Reason != "" && e.PodName != "" {
		return fmt.Sprintf("Alert: %s in %s", e.Reason, e.PodName)
	}
	if e.Reason != "" {
		return fmt.Sprintf("Alert: %s", e.Reason)
	}
	return "Alert: container issue detected"
}
