package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	apiPressureDesc = prometheus.NewDesc("kwatch_api_pressure",
		"1 while the API server is under pressure and kwatch pauses "+
			"optional work, else 0", nil, nil)
	apiPressureSignalsDesc = prometheus.NewDesc(
		"kwatch_api_pressure_signals_total",
		"Signs of API server pressure kwatch saw, by signal",
		[]string{"signal"}, nil)

	// apiPressureSignals: throttled (HTTP 429), slow (a request over
	// the slow bound) and limiter (kwatch's own client-side limiter
	// held a request back), in kubeclient.PressureSignal order.
	apiPressureSignals = [...]string{"throttled", "slow", "limiter"}
)

// IncAPIPressureSignal counts one sign of API pressure by its index in
// apiPressureSignals; an unknown index is ignored, so the label set
// cannot grow.
func (r *Registry) IncAPIPressureSignal(signal int) {
	if signal >= 0 && signal < len(apiPressureSignals) {
		r.APIPressureSignals[signal].Add(1)
	}
}

func (r *Registry) describeAPIPressure(ch chan<- *prometheus.Desc) {
	ch <- apiPressureDesc
	ch <- apiPressureSignalsDesc
}

func (r *Registry) collectAPIPressure(ch chan<- prometheus.Metric) {
	ch <- prometheus.MustNewConstMetric(apiPressureDesc,
		prometheus.GaugeValue, float64(r.APIPressure.Load()))
	for i, signal := range apiPressureSignals {
		ch <- prometheus.MustNewConstMetric(
			apiPressureSignalsDesc, prometheus.CounterValue,
			float64(r.APIPressureSignals[i].Load()), signal,
		)
	}
}
