package event

import (
	"fmt"
	"strings"

	"github.com/abahmed/kwatch/constant"
)

func (e *Event) FormatMarkdown(clusterName, text, delimiter string) string {
	// add events part if it exists
	eventsText := constant.DefaultEvents
	events := strings.TrimSpace(e.Events)
	if len(events) > 0 {
		eventsText = e.Events
	}

	// add logs part if it exist
	logsText := constant.DefaultLogs
	logs := strings.TrimSpace(e.Logs)
	if len(logs) > 0 {
		logsText = e.Logs
	}

	// use custom text if it's provided, otherwise use default
	if len(text) == 0 {
		text = constant.DefaultText
	}

	if len(delimiter) == 0 {
		delimiter = "\n"
	}

	msg := fmt.Sprintf(
		"%s"+delimiter+
			"**Cluster:** %s"+delimiter+
			"**Pod:** %s"+delimiter+
			"**Container:** %s"+delimiter+
			"**Namespace:** %s"+delimiter+
			"**Reason:** %s"+delimiter+
			"**Events:**\n```\n%s\n```"+delimiter+
			"**Logs:**\n```\n%s\n```",
		text,
		clusterName, e.PodName,
		e.ContainerName,
		e.Namespace,
		e.Reason,
		eventsText,
		logsText,
	)

	return msg
}

func (e *Event) FormatHtml(clusterName, text string) string {
	eventsText := constant.DefaultEvents
	logsText := constant.DefaultLogs

	// add events part if it exists
	events := strings.TrimSpace(e.Events)
	if len(events) > 0 {
		eventsText = e.Events
	}

	// add logs part if it exists
	logs := strings.TrimSpace(e.Logs)
	if len(logs) > 0 {
		logsText = e.Logs
	}

	// use custom text if it's provided, otherwise use default
	if len(text) == 0 {
		text = constant.DefaultText
	}

	msg := fmt.Sprintf(
		"%s<br/>"+
			"<b>Cluster:</b> %s <br/>"+
			"<b>Pod:</b> %s <br/>"+
			"<b>Container:</b> %s<br/>"+
			"<b>Namespace:</b> %s<br/>"+
			"<b>Reason:</b> %s<br/>"+
			"<b>Events:</b><br/><blockquote>%s</blockquote>"+
			"<b>Logs:</b> <br/><blockquote>%s</blockquote>",
		text,
		clusterName,
		e.PodName,
		e.ContainerName,
		e.Namespace,
		e.Reason,
		strings.ReplaceAll(eventsText, "\n", "<br/>"),
		strings.ReplaceAll(logsText, "\n", "<br/>"),
	)

	return msg
}

func (e *Event) FormatText(clusterName, text string) string {
	eventsText := constant.DefaultEvents
	logsText := constant.DefaultLogs

	// add events part if it exists
	events := strings.TrimSpace(e.Events)
	if len(events) > 0 {
		eventsText = e.Events
	}

	// add logs part if it exists
	logs := strings.TrimSpace(e.Logs)
	if len(logs) > 0 {
		logsText = e.Logs
	}

	// use custom text if it's provided, otherwise use default
	if len(text) == 0 {
		text = constant.DefaultText
	}

	msg := fmt.Sprintf(
		"There is an issue with container in a pod!\n\n"+
			"cluster: %s\n"+
			"Pod Name: %s\n"+
			"Container: %s\n"+
			"Namespace: %s\n"+
			"Reason: %s\n\n"+
			"Events:\n%s\n\n"+
			"Logs:\n%s\n\n",
		clusterName,
		e.PodName,
		e.ContainerName,
		e.Namespace,
		e.Reason,
		eventsText,
		logsText,
	)

	return msg
}
