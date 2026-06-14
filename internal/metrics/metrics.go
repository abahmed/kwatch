package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"
)

type Registry struct {
	IncidentsTotal      atomic.Int64
	IncidentsCreate     atomic.Int64
	IncidentsUpdate     atomic.Int64
	IncidentsResolved   atomic.Int64
	IncidentsDigest     atomic.Int64
	NotificationsTotal  atomic.Int64
	NotificationsDropped atomic.Int64
	BaselineSize        atomic.Int64
	ActiveIncidents     atomic.Int64
	WorkQueueDepth      atomic.Int64
}

var Default = &Registry{}

func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		var lines []string
		lines = append(lines, "# HELP kwatch_incidents_total Total incidents by action")
		lines = append(lines, "# TYPE kwatch_incidents_total counter")
		for action, count := range map[string]int64{
			"create":   r.IncidentsCreate.Load(),
			"update":   r.IncidentsUpdate.Load(),
			"resolved": r.IncidentsResolved.Load(),
			"digest":   r.IncidentsDigest.Load(),
		} {
			lines = append(lines, fmt.Sprintf(`kwatch_incidents_total{action="%s"} %d`, action, count))
		}
		lines = append(lines, "")
		lines = append(lines, "# HELP kwatch_notifications_total Total notification attempts")
		lines = append(lines, "# TYPE kwatch_notifications_total counter")
		lines = append(lines, fmt.Sprintf("kwatch_notifications_total %d", r.NotificationsTotal.Load()))
		lines = append(lines, "")
		lines = append(lines, "# HELP kwatch_notifications_dropped_total Notifications dropped (channel full)")
		lines = append(lines, "# TYPE kwatch_notifications_dropped_total counter")
		lines = append(lines, fmt.Sprintf("kwatch_notifications_dropped_total %d", r.NotificationsDropped.Load()))
		lines = append(lines, "")
		lines = append(lines, "# HELP kwatch_incidents_active Currently active incidents")
		lines = append(lines, "# TYPE kwatch_incidents_active gauge")
		lines = append(lines, fmt.Sprintf("kwatch_incidents_active %d", r.ActiveIncidents.Load()))
		lines = append(lines, "")
		lines = append(lines, "# HELP kwatch_baseline_size Baseline entries (seen pods)")
		lines = append(lines, "# TYPE kwatch_baseline_size gauge")
		lines = append(lines, fmt.Sprintf("kwatch_baseline_size %d", r.BaselineSize.Load()))
		sort.Strings(lines)
		fmt.Fprint(w, strings.Join(lines, "\n")+"\n")
	})
}
