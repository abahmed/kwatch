package kube

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUnresolvedInReadsServiceShapedNames(t *testing.T) {
	cases := map[string]string{
		"Name or service not known (geo-api:80)":                "geo-api.shop",
		"lookup geo-api.data on 10.96.0.10:53: no such host":    "geo-api.data",
		"java.net.UnknownHostException: geo-api.data.svc":       "geo-api.data",
		"getaddrinfo ENOTFOUND geo-api.data.svc.cluster.local":  "geo-api.data",
		"curl: (6) Could not resolve host: geo-api":             "geo-api.shop",
		"geo-api: Name or service not known":                    "geo-api.shop",
		"lookup api.example.com on 10.96.0.10:53: no such host": "",
		"lookup geo-api on 10.96.0.10:53: i/o timeout":          "",
		"Temporary failure in name resolution (geo-api:80)":     "",
		"socket.gaierror: [Errno -2] Name or service not known": "",
		"Name or service not known (localhost:80)":              "",
		"Name or service not known (10.0.0.5:80)":               "",
	}
	for line, want := range cases {
		got := ""
		for name := range unresolvedIn([]string{line}, "shop") {
			got = name
		}
		assert.Equal(t, want, got, line)
	}
}

func unresolvedWindows(at time.Time, seen ...bool) []windowCount {
	var out []windowCount
	for i, s := range seen {
		w := windowCount{at: at.Add(time.Duration(i) * 5 * time.Minute)}
		if s {
			w.unresolved = map[string]string{"geo.shop": "line"}
		}
		out = append(out, w)
	}
	return out
}

func TestPersistentUnresolvedNeedsSamplesSpanAndRecency(t *testing.T) {
	start := fixedNow()
	// Five windows over twenty minutes: it holds.
	windows := unresolvedWindows(start, true, true, true, true, true)
	s, ok := persistentUnresolved(windows, start.Add(20*time.Minute))
	assert.True(t, ok)
	assert.Equal(t, "geo.shop", s.name)
	assert.Equal(t, 5, s.samples)
	// One miss at start-up never counts.
	_, ok = persistentUnresolved(unresolvedWindows(start, true, false,
		false, false, false), start.Add(20*time.Minute))
	assert.False(t, ok)
	// Three samples within ten minutes: not spread over a while.
	_, ok = persistentUnresolved(unresolvedWindows(start, true, true,
		true), start.Add(10*time.Minute))
	assert.False(t, ok)
	// The errors stopped half an hour ago: cleared.
	_, ok = persistentUnresolved(windows, start.Add(time.Hour))
	assert.False(t, ok)
}
