package kube

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCounterRatesComputesRateAndResets(t *testing.T) {
	c := newCounterRates()
	at := time.Unix(100, 0)
	_, ok := c.rate("k", at, 10)
	assert.False(t, ok)
	r, ok := c.rate("k", at.Add(10*time.Second), 60)
	assert.True(t, ok)
	assert.InDelta(t, 5, r, 0.001)
	_, ok = c.rate("k", at.Add(20*time.Second), 1)
	assert.False(t, ok)
	_, ok = c.rate("k", at.Add(20*time.Second), 5)
	assert.False(t, ok)
}

func TestCounterRatesThrottleRatioUsesLiveCgroup(t *testing.T) {
	c := newCounterRates()
	at := time.Unix(100, 0)
	first := `# HELP x
container_cpu_cfs_periods_total{id="/a",namespace="n",pod="p",` +
		`container="c"} 100
container_cpu_cfs_throttled_periods_total{id="/a",namespace="n",` +
		`pod="p",container="c"} 10
container_cpu_cfs_periods_total{id="/old",namespace="n",pod="p",` +
		`container="c"} 5
`
	assert.Empty(t, c.throttleRatios([]byte(first), at))
	second := `container_cpu_cfs_periods_total{id="/a",namespace="n",` +
		`pod="p",container="c"} 200
container_cpu_cfs_throttled_periods_total{id="/a",namespace="n",` +
		`pod="p",container="c"} 60
`
	got := c.throttleRatios([]byte(second), at.Add(time.Minute))
	assert.InDelta(t, 50, got["n/p/c"], 0.001)
}

func TestCountersSumMetricAddsSamples(t *testing.T) {
	body := []byte("# c\nfoo 1\nfoo{a=\"x\"} 2.5\nbar 9\nfoo bad\n")
	total, found := sumMetric(body, "foo")
	assert.True(t, found)
	assert.InDelta(t, 3.5, total, 0.001)
	_, found = sumMetric(body, "missing")
	assert.False(t, found)
}

func TestCountersMetricLineParsing(t *testing.T) {
	tt := []struct {
		name   string
		line   string
		ok     bool
		metric string
		labels map[string]string
		value  float64
	}{
		{"blank", "", false, "", nil, 0},
		{"comment", "# HELP", false, "", nil, 0},
		{"plain", "m 4", true, "m", map[string]string{}, 4},
		{"labels", `m{a="1,2",b="x"} 7 123`, true, "m",
			map[string]string{"a": "1,2", "b": "x"}, 7},
		{"broken_brace", "m}x{", false, "", nil, 0},
		{"no_value", "m", false, "", nil, 0},
		{"bad_value", "m abc", false, "m", map[string]string{}, 0},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			name, labels, value, ok := metricLine(tc.line)
			assert.Equal(t, tc.ok, ok)
			if tc.ok {
				assert.Equal(t, tc.metric, name)
				assert.Equal(t, tc.labels, labels)
				assert.Equal(t, tc.value, value)
			}
		})
	}
	assert.Equal(t, []string{`a="x,y"`, "b=1"},
		splitLabels(`a="x,y",b=1`))
}
