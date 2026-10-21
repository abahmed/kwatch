package app

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/audit"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/metrics"
)

func TestDecisionLogSurvivesAPanickingAuditWriter(t *testing.T) {
	var calls, written atomic.Int64
	reg := &metrics.Registry{}
	l := newDecisionLog(func(audit.Entry) {
		if calls.Add(1) == 1 {
			panic("audit writer blew up")
		}
		written.Add(1)
	}, reg, func() []incident.Incident { return nil })
	go l.run()

	l.record(audit.Entry{Incident: "lost"}, false)
	l.record(audit.Entry{Incident: "kept"}, false)

	require.True(t, l.closeWithin(time.Second, time.After),
		"the worker drains and stops after a panic")
	require.EqualValues(t, 1, written.Load(), "the next entry is written")
	require.EqualValues(t, 1, l.written.Load())
	require.EqualValues(t, 1, reg.AuditDropped.Load(),
		"the entry lost to the panic is counted")
}

func TestDecisionLogSurvivesAPanickingIncidentGauge(t *testing.T) {
	var written atomic.Int64
	reg := &metrics.Registry{}
	l := newDecisionLog(func(audit.Entry) { written.Add(1) }, reg,
		func() []incident.Incident { panic("incidents blew up") })
	go l.run()

	l.record(audit.Entry{Incident: "a"}, true) // asks for a gauge refresh
	l.record(audit.Entry{Incident: "b"}, true)

	require.True(t, l.closeWithin(time.Second, time.After))
	require.EqualValues(t, 2, written.Load())
}
