package app

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/notification"
)

// SelfAlertReason is the route reason of every self alert, so a
// provider route can send them to one channel or leave them out.
const SelfAlertReason = "kwatch_degraded"

// selfAlertKeyPrefix starts the conversation key of each part's alert.
const selfAlertKeyPrefix = "kwatch-self/"

// selfWords is one degraded reason in plain words: what kwatch cannot
// do (%s is the part), what it means and what to do.
type selfWords struct {
	cannot, means, todo string
}

// selfReasonWords word each reason a self alert is sent for.
var selfReasonWords = map[string]selfWords{
	"kubelet_unreachable": {"it cannot read usage from any node's kubelet",
		"Memory, CPU and disk use are not checked, so limits and full " +
			"disks are not seen coming.",
		"Check that kwatch can reach the kubelets (port 10250) and may " +
			"read nodes/stats and nodes/proxy; its log names the error."},
	"kubelet_partially_unreachable": {
		"it cannot read usage from some nodes' kubelets",
		"Memory, CPU and disk use on those nodes are not checked.",
		"Check that kwatch can reach every kubelet (port 10250); its " +
			"log names the nodes."},
	"permission_denied": {"the Kubernetes API refuses it %s",
		"Problems with those objects are not seen.",
		"Add the missing permission to kwatch's ClusterRole; /health " +
			"and the dashboard's kwatch page name it."},
	"optional_permission_denied": {"the Kubernetes API refuses it %s",
		"Alerts carry less detail, for example no crash output.",
		"Add the missing permission to kwatch's ClusterRole " +
			"(for pod logs: get on pods/log)."},
	"api_unavailable": {"the API server does not serve %s",
		"Problems with those objects are not seen.",
		"Check that the API is installed and served."},
	"cache_sync_failed": {"it cannot keep its watch of %s up to date",
		"Changes to those objects may be missed.", watchTodo},
	"cache_sync_timeout": {"it could not finish reading %s",
		"Changes to those objects may be missed.", watchTodo},
	"watcher_failed": {"a watch keeps failing (%s)",
		"Changes may be missed.", watchTodo},
	"handler_panics": {"it keeps failing to read %s",
		"Those objects are not checked.", watchTodo},
	"storage_reset": {"its saved state was reset",
		"Issues and history from before were lost.", volumeTodo},
	"storage_over_cap": {"its state file stays over its size cap",
		"Older history is dropped early.",
		"Give the state volume more room or keep less history."},
	"persistence_write_failed": {"it cannot save %s",
		"A restart may lose what was not saved.", volumeTodo},
	"persistence_restore_failed": {"it could not read back %s",
		"What was saved before was lost.", volumeTodo},
	"trail_restore_failed": {"it could not read back who was told",
		"The dashboard lacks earlier sends.", volumeTodo},
	"provider_unavailable": {"it cannot reach the %s alert channel",
		"Alerts to it wait and are tried again.",
		"Check the channel's address, the network and the token."},
	"provider_rejected": {"the %s alert channel refuses its messages",
		"Alerts to it are not delivered.",
		"Check the channel's token and settings."},
	"provider_rate_limited": {"the %s alert channel slows it down",
		"Alerts to it arrive late.",
		"Send fewer alerts there (routes) or raise the channel's limit."},
	"heartbeat_failed": {"its heartbeat pings fail",
		"Your monitoring may report kwatch as down while it runs.",
		"Check heartbeatMonitor.url and that kwatch can reach it."},
	"rate_limited": {"the Kubernetes API server is throttling it",
		"Optional work (log samples, new kinds) waits, so it sees less.",
		"Check the API server's load and its limits for kwatch."},
	"component_failed": {"its %s part keeps failing",
		partTodo, logTodo},
	"component_stalled": {"its %s part stopped making progress",
		partTodo, logTodo},
	"component_stopped": {"its %s part keeps stopping",
		partTodo, logTodo},
	selfReasonLag: {"it is falling behind: changes take over 10 s to " +
		"be judged", "Alerts arrive late.",
		"Give kwatch more CPU, or narrow what it watches."},
	selfReasonBacklog: {"its messages have waited to be sent for a while",
		"Alerts arrive late or not at all.",
		"Check the alert channels on /health or the dashboard."},
}

// Shared "what to do" sentences.
const (
	watchTodo = "Check kwatch's log for the watch errors and the API " +
		"server's health."
	volumeTodo = "Check the state volume: free space, and that it is writable."
	logTodo    = "Check kwatch's log for the errors of that part."
	partTodo   = "What that part does is not done."
)

// partWords names a health component in a sentence.
func partWords(part string) string {
	switch {
	case part == "pod-logs":
		return "the pod logs"
	case strings.HasPrefix(part, "source-"):
		return "the " + strings.TrimPrefix(part, "source-")
	case strings.HasPrefix(part, providerComponentPrefix):
		return strings.TrimPrefix(part, providerComponentPrefix)
	case part == "delivery-outbox":
		return "its unsent alerts"
	case part == "usage-history":
		return "its usage history"
	}
	return part
}

// wordsOf is reason in plain words for part.
func wordsOf(part, reason string) selfWords {
	w, ok := selfReasonWords[reason]
	if !ok {
		w = selfReasonWords["component_failed"]
	}
	w.cannot = strings.Replace(w.cannot, "%s", partWords(part), 1)
	return w
}

// selfLead starts each message: "kwatch" with the cluster's name.
func selfLead(cluster string) string {
	if cluster == "" {
		return "kwatch"
	}
	return "kwatch in " + cluster
}

// selfMessage is the open (or updated) self alert of part.
func selfMessage(cluster, part string, open selfOpen) notification.Message {
	w := wordsOf(part, open.Reason)
	lead := selfLead(cluster) + " is degraded: " + w.cannot + " since " +
		clockUTC(open.Since) + "."
	lines := []string{"What this means: " + w.means,
		"What to do: " + w.todo}
	msg := selfBase(part, open.Revision, notification.StatusLow,
		notification.MarkerLow, lead, lines)
	msg.Opens = open.Revision == 1
	return msg
}

// selfResolved closes the self alert of part at now.
func selfResolved(
	cluster, part string, open selfOpen, now time.Time,
) notification.Message {
	w := wordsOf(part, open.Reason)
	lead := selfLead(cluster) + " is back to normal after " +
		format.Duration(now.Sub(open.Since)) + "."
	lines := []string{"It had reported that " + w.cannot + " (since " +
		clockUTC(open.Since) + ")."}
	return selfBase(part, open.Revision, notification.StatusResolved,
		notification.MarkerResolved, lead, lines)
}

// selfBase is the message both share. It never pages: paging tools
// and issue trackers skip it (SkipPaging), and its route says it is
// information about kwatch itself.
func selfBase(
	part string, revision int, status notification.Status,
	marker, lead string, lines []string,
) notification.Message {
	return notification.Message{
		Key: selfAlertKeyPrefix + part, Revision: revision,
		Status: status, Marker: marker, Title: lead,
		Short: marker + " " + lead, Lines: lines,
		Note:       marker + " " + lead + " " + strings.Join(lines, " "),
		SkipPaging: true,
		Route: notification.Route{Reasons: []string{SelfAlertReason},
			Severity: "info"},
	}
}

// clockUTC is t as "10:12 UTC".
func clockUTC(t time.Time) string {
	return t.UTC().Format("15:04") + " UTC"
}
