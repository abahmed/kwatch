package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	editionRebuildsDesc = prometheus.NewDesc(
		"kwatch_edition_rebuilds_total",
		"In-process rebuilds after a license edition change, by result",
		[]string{"result"}, nil)

	// editionRebuildResults: success (the new generation is built and
	// serving); retry (one transient step failure that was retried);
	// failed (the rebuild gave up and the process exits).
	editionRebuildResults = [...]string{"success", "retry", "failed"}
)

// IncEditionRebuild records one rebuild event using a bounded result
// label; an unknown result counts as failed.
func (r *Registry) IncEditionRebuild(result string) {
	for i, allowed := range editionRebuildResults {
		if result == allowed {
			r.EditionRebuilds[i].Add(1)
			return
		}
	}
	r.EditionRebuilds[len(editionRebuildResults)-1].Add(1)
}

func (r *Registry) describeRebuilds(ch chan<- *prometheus.Desc) {
	ch <- editionRebuildsDesc
}

func (r *Registry) collectRebuilds(ch chan<- prometheus.Metric) {
	for i, result := range editionRebuildResults {
		ch <- prometheus.MustNewConstMetric(
			editionRebuildsDesc, prometheus.CounterValue,
			float64(r.EditionRebuilds[i].Load()), result,
		)
	}
}
