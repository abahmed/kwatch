package metrics

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"

	"k8s.io/klog/v2"
)

type Registry struct {
	IncidentsCreate         atomic.Int64
	IncidentsUpdate         atomic.Int64
	IncidentsResolved       atomic.Int64
	IncidentsGrouped        atomic.Int64
	NotificationsTotal      atomic.Int64
	NotificationsDropped    atomic.Int64
	BaselineSize            atomic.Int64
	ActiveIncidents         atomic.Int64
	GraphNodes              atomic.Int64
	GraphEdges              atomic.Int64
	APIServerProbeErrors    atomic.Int64
	APIServerLatencyMs      atomic.Int64
	ControlPlaneProbeErrors atomic.Int64
	InformerWatchErrors     atomic.Int64
	InformerEvents          atomic.Int64
}

var Default = &Registry{}

func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		var lines []string
		lines = append(
			lines,
			"# HELP kwatch_incidents_total Total incidents by action",
		)
		lines = append(lines, "# TYPE kwatch_incidents_total counter")
		for _, sample := range []struct {
			action string
			count  int64
		}{
			{action: "create", count: r.IncidentsCreate.Load()},
			{action: "update", count: r.IncidentsUpdate.Load()},
			{action: "resolved", count: r.IncidentsResolved.Load()},
			{action: "grouped", count: r.IncidentsGrouped.Load()},
		} {
			lines = append(
				lines,
				fmt.Sprintf(
					`kwatch_incidents_total{action="%s"} %d`,
					sample.action,
					sample.count,
				),
			)
		}
		lines = append(lines, "")
		lines = append(lines, "# HELP kwatch_apiserver_probe_errors_total API server health probe failures")
		lines = append(lines, "# TYPE kwatch_apiserver_probe_errors_total counter")
		lines = append(lines, fmt.Sprintf("kwatch_apiserver_probe_errors_total %d", r.APIServerProbeErrors.Load()))
		lines = append(lines, "# HELP kwatch_apiserver_latency_milliseconds Latest API server readyz latency")
		lines = append(lines, "# TYPE kwatch_apiserver_latency_milliseconds gauge")
		lines = append(lines, fmt.Sprintf("kwatch_apiserver_latency_milliseconds %d", r.APIServerLatencyMs.Load()))
		lines = append(lines, "# HELP kwatch_controlplane_probe_errors_total Control-plane component probe failures")
		lines = append(lines, "# TYPE kwatch_controlplane_probe_errors_total counter")
		lines = append(lines, fmt.Sprintf("kwatch_controlplane_probe_errors_total %d", r.ControlPlaneProbeErrors.Load()))
		lines = append(lines, "# HELP kwatch_informer_watch_errors_total Informer watch interruptions")
		lines = append(lines, "# TYPE kwatch_informer_watch_errors_total counter")
		lines = append(lines, fmt.Sprintf("kwatch_informer_watch_errors_total %d", r.InformerWatchErrors.Load()))
		lines = append(lines, "# HELP kwatch_informer_events_total Informer events received by kwatch")
		lines = append(lines, "# TYPE kwatch_informer_events_total counter")
		lines = append(lines, fmt.Sprintf("kwatch_informer_events_total %d", r.InformerEvents.Load()))
		lines = append(
			lines,
			"# HELP kwatch_notifications_total Total notification attempts",
		)
		lines = append(lines, "# TYPE kwatch_notifications_total counter")
		lines = append(
			lines,
			fmt.Sprintf(
				"kwatch_notifications_total %d",
				r.NotificationsTotal.Load(),
			),
		)
		lines = append(lines, "")
		lines = append(
			lines,
			"# HELP kwatch_notifications_dropped_total Notifications dropped "+
				"(channel full)",
		)
		lines = append(
			lines,
			"# TYPE kwatch_notifications_dropped_total counter",
		)
		lines = append(
			lines,
			fmt.Sprintf(
				"kwatch_notifications_dropped_total %d",
				r.NotificationsDropped.Load(),
			),
		)
		lines = append(lines, "")
		lines = append(
			lines,
			"# HELP kwatch_incidents_active Currently active incidents",
		)
		lines = append(lines, "# TYPE kwatch_incidents_active gauge")
		lines = append(
			lines,
			fmt.Sprintf("kwatch_incidents_active %d", r.ActiveIncidents.Load()),
		)
		lines = append(lines, "")
		lines = append(
			lines,
			"# HELP kwatch_baseline_size Baseline entries (seen pods)",
		)
		lines = append(lines, "# TYPE kwatch_baseline_size gauge")
		lines = append(
			lines,
			fmt.Sprintf("kwatch_baseline_size %d", r.BaselineSize.Load()),
		)
		lines = append(lines, "")
		lines = append(
			lines,
			"# HELP kwatch_graph_nodes Resources in the dependency graph",
		)
		lines = append(lines, "# TYPE kwatch_graph_nodes gauge")
		lines = append(
			lines,
			fmt.Sprintf("kwatch_graph_nodes %d", r.GraphNodes.Load()),
		)
		lines = append(lines, "")
		lines = append(
			lines,
			"# HELP kwatch_graph_edges Relationships in the dependency graph",
		)
		lines = append(lines, "# TYPE kwatch_graph_edges gauge")
		lines = append(
			lines,
			fmt.Sprintf("kwatch_graph_edges %d", r.GraphEdges.Load()),
		)
		if _, err := fmt.Fprint(w, strings.Join(lines, "\n")+"\n"); err != nil {
			klog.ErrorS(err, "metrics: write prometheus output")
		}
	})
}
