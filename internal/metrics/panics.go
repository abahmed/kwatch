package metrics

import "github.com/prometheus/client_golang/prometheus"

// panicStages are the stages whose panics are contained: the decision
// path, and "background" for everything else that runs under safego
// (writers, probes, watchers). The set is fixed so the label stays
// bounded.
var panicStages = [...]string{
	"detector", "incident", "compose", "announce", "background",
}

var panicsDesc = prometheus.NewDesc("kwatch_panics_recovered_total",
	"Panics contained by stage", []string{"stage"},
	nil)

// IncPanic records one contained panic using a bounded stage label.
// An unknown stage counts as the last one, "background".
func (r *Registry) IncPanic(stage string) {
	for i, allowed := range panicStages {
		if stage == allowed {
			r.Panics[i].Add(1)
			return
		}
	}
	r.Panics[len(panicStages)-1].Add(1)
}

// PanicCount returns the panics contained at a stage.
func (r *Registry) PanicCount(stage string) int64 {
	for i, allowed := range panicStages {
		if stage == allowed {
			return r.Panics[i].Load()
		}
	}
	return 0
}

func (r *Registry) collectPanics(ch chan<- prometheus.Metric) {
	for i, stage := range panicStages {
		ch <- prometheus.MustNewConstMetric(panicsDesc,
			prometheus.CounterValue, float64(r.Panics[i].Load()), stage)
	}
}
