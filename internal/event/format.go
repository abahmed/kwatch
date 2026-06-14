package event

import (
	"fmt"
	"strings"

	"github.com/abahmed/kwatch/internal/constant"
)

func (e *Event) FormatMarkdown(clusterName, text, delimiter string) string {
	// use custom text if it's provided, otherwise use default
	if len(text) == 0 {
		text = constant.DefaultText
	}

	if len(delimiter) == 0 {
		delimiter = "\n"
	}

	eventsBlock := ""
	if e.IncludeEvents {
		eventsText := constant.DefaultEvents
		events := strings.TrimSpace(e.Events)
		if len(events) > 0 {
			eventsText = e.Events
		}
		eventsBlock = "**Events:**\n```\n" + eventsText + "\n```"
	}

	logsBlock := ""
	if e.IncludeLogs {
		logsText := constant.DefaultLogs
		logs := strings.TrimSpace(e.Logs)
		if len(logs) > 0 {
			logsText = e.Logs
		}
		logsBlock = "**Logs:**\n```\n" + logsText + "\n```"
	}

	msg := fmt.Sprintf(
		"%s"+delimiter+
			"**Cluster:** %s"+delimiter+
			"**Pod:** %s"+delimiter+
			"**Container:** %s"+delimiter+
			"**Namespace:** %s"+delimiter+
			"**Node:** %s"+delimiter+
			"**Reason:** %s"+delimiter+
			"%s"+delimiter+
			"%s",
		text,
		clusterName, e.PodName,
		e.ContainerName,
		e.Namespace,
		e.NodeName,
		e.Reason,
		eventsBlock,
		logsBlock,
	)

	return msg
}

func (e *Event) FormatHtml(clusterName, text string) string {
	// use custom text if it's provided, otherwise use default
	if len(text) == 0 {
		text = constant.DefaultText
	}

	eventsBlock := ""
	if e.IncludeEvents {
		eventsText := constant.DefaultEvents
		events := strings.TrimSpace(e.Events)
		if len(events) > 0 {
			eventsText = e.Events
		}
		eventsBlock = "<b>Events:</b><br/><blockquote>" + strings.ReplaceAll(eventsText, "\n", "<br/>") + "</blockquote>"
	}

	logsBlock := ""
	if e.IncludeLogs {
		logsText := constant.DefaultLogs
		logs := strings.TrimSpace(e.Logs)
		if len(logs) > 0 {
			logsText = e.Logs
		}
		logsBlock = "<b>Logs:</b> <br/><blockquote>" + strings.ReplaceAll(logsText, "\n", "<br/>") + "</blockquote>"
	}

	msg := fmt.Sprintf(
		"%s<br/>"+
			"<b>Cluster:</b> %s <br/>"+
			"<b>Pod:</b> %s <br/>"+
			"<b>Container:</b> %s<br/>"+
			"<b>Namespace:</b> %s<br/>"+
			"<b>Node:</b> %s<br/>"+
			"<b>Reason:</b> %s<br/>"+
			"%s"+
			"%s",
		text,
		clusterName,
		e.PodName,
		e.ContainerName,
		e.Namespace,
		e.NodeName,
		e.Reason,
		eventsBlock,
		logsBlock,
	)

	return msg
}

func (e *Event) FormatText(clusterName, text string) string {
	// use custom text if it's provided, otherwise use default
	if len(text) == 0 {
		text = constant.DefaultText
	}

	eventsBlock := ""
	if e.IncludeEvents {
		eventsText := constant.DefaultEvents
		events := strings.TrimSpace(e.Events)
		if len(events) > 0 {
			eventsText = e.Events
		}
		eventsBlock = "Events:\n" + eventsText + "\n\n"
	}

	logsBlock := ""
	if e.IncludeLogs {
		logsText := constant.DefaultLogs
		logs := strings.TrimSpace(e.Logs)
		if len(logs) > 0 {
			logsText = e.Logs
		}
		logsBlock = "Logs:\n" + logsText + "\n\n"
	}

	msg := fmt.Sprintf(
		"There is an issue with container in a pod!\n\n"+
			"cluster: %s\n"+
			"Pod Name: %s\n"+
			"Container: %s\n"+
			"Namespace: %s\n"+
			"Node: %s\n"+
			"Reason: %s\n\n"+
			"%s"+
			"%s",
		clusterName,
		e.PodName,
		e.ContainerName,
		e.Namespace,
		e.NodeName,
		e.Reason,
		eventsBlock,
		logsBlock,
	)

	return msg
}
