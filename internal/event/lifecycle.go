package event

import "strings"

// noticeKey groups non-incident notices (startup, digests, test alerts) so
// paging systems deduplicate them into one alert instead of paging per notice.
const noticeKey = "kwatch-notice"

// IsResolve reports whether the event announces an incident recovery.
func (e *Event) IsResolve() bool {
	return e != nil && e.Action == "resolved"
}

// IsNotice reports whether the event carries a plain message rather than an
// incident, such as the startup banner or a queue digest.
func (e *Event) IsNotice() bool {
	return e != nil && e.DedupKey == "" && e.Narrative == "" &&
		e.Reason == "notify"
}

// AlertKey is the stable identity paging systems use to update and resolve
// one alert per incident.
func (e *Event) AlertKey() string {
	if e == nil || e.DedupKey == "" {
		return noticeKey
	}
	return "kwatch-" + e.DedupKey
}

// AlertTitle is a one-line summary for alert titles.
func (e *Event) AlertTitle(limit int) string {
	title := ""
	switch {
	case e == nil:
	case e.IsNotice():
		title = firstLine(e.PodName)
	case strings.TrimSpace(e.Narrative) != "":
		title = firstLine(e.Narrative)
	default:
		title = strings.TrimSpace(e.Reason + " " + e.Namespace + "/" +
			e.PodName)
	}
	if title == "" {
		title = "kwatch alert"
	}
	if limit > 0 && len(title) > limit {
		cut := limit - len("…")
		for cut > 0 && (title[cut]&0xC0) == 0x80 {
			cut--
		}
		title = title[:cut] + "…"
	}
	return title
}

// AlertBody is the full text for an alert description.
func (e *Event) AlertBody(clusterName string) string {
	if e == nil {
		return ""
	}
	if e.IsNotice() {
		return e.PodName
	}
	if narrative := strings.TrimSpace(e.Narrative); narrative != "" {
		return narrative
	}
	return e.FormatText(clusterName, "")
}

func firstLine(value string) string {
	return strings.TrimSpace(strings.SplitN(
		strings.TrimSpace(value), "\n", 2,
	)[0])
}
