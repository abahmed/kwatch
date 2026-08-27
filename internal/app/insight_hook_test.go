package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/audit"
	kwcontext "github.com/abahmed/kwatch/internal/context"
	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
)

// Every grouped, renotified, escalated or restored incident is announced
// through the lifecycle hook. It used to pass a nil insight, so the diagnosis
// the graph had already worked out never reached the notification.
func TestLifecycleHookAttachesDiagnosis(t *testing.T) {
	graph := kwcontext.NewResourceGraph()
	graph.AddEdge(
		"pod",
		"dev",
		"api-abc",
		"node",
		"",
		"ip-10-0-81-7",
		"scheduled_on",
	)

	type call struct {
		action model.IncidentAction
		ins    *insight.Insight
	}
	var calls []call
	opts := &engineOptions{
		auditLogger:   audit.NewLogger(audit.Config{Enabled: false}),
		insightEngine: insight.NewEngine(graph, nil),
		notify: func(
			_ *model.Incident, a model.IncidentAction, ins *insight.Insight,
		) {
			calls = append(calls, call{a, ins})
		},
	}
	holder := &engineHolder{
		engine: correlation.NewEngine(correlation.Config{Window: time.Minute}),
	}
	hook := lifecycleHook(opts, holder)

	inc := &model.Incident{
		Subject: model.Subject{
			Key:       "dev:dev/api:ContainersNotReady:",
			Resource:  "pod",
			Namespace: "dev",
			Name:      "api",
			NodeName:  "ip-10-0-81-7",
		},
		Status: model.Status{
			Resources: map[string]bool{"api-abc": true},
		},
	}

	hook(inc, model.ActionCreate)
	hook(inc, model.ActionUpdate)
	hook(inc, model.ActionResolved)
	hook(inc, model.ActionSkip)

	require.Len(
		t,
		calls,
		3,
		"skip is not announced; create, update and resolve are",
	)

	require.NotNil(t, calls[0].ins, "a create must carry a diagnosis")
	assert.Contains(
		t,
		calls[0].ins.Cause,
		"ip-10-0-81-7",
		"the graph knows the node is the cause",
	)
	assert.Equal(t, "node_failure", calls[0].ins.Pattern)

	require.NotNil(t, calls[1].ins, "an update must carry a diagnosis")
	assert.Nil(t, calls[2].ins, "a resolve has nothing left to explain")
}
