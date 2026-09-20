package persistence

import (
	"context"
	"time"

	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/model"
)

// BaselineStore is the persistence surface needed by baseline seeding and
// saving. Consumers should not receive the full ConfigMap manager when this
// is all they need.
type BaselineStore interface {
	GetBaseline(context.Context) map[string]map[string]int64
	SaveBaseline(context.Context, map[string]map[string]int64) error
}

// ChangeHistoryStore persists the bounded resource-change history used by
// insight analysis.
type ChangeHistoryStore interface {
	LoadChangeHistory(context.Context) ([]kwcontext.Change, error)
	SaveChangeHistory(context.Context, []kwcontext.Change) error
}

// FeedbackStore persists the bounded RCA outcome history.
type FeedbackStore interface {
	LoadRCAFeedback(context.Context) ([]model.RCARecord, error)
	SaveRCAFeedback(context.Context, []model.RCARecord) error
}

// IncidentStore persists incident state and its related delivery state. The
// flat DTOs are intentionally part of this contract so persistence shape is
// visible at the call site.
type IncidentStore interface {
	SaveIncidentState(
		context.Context,
		[]model.PersistedIncident,
		[]model.PersistedGroup,
		map[string]map[string]string,
		model.PersistedEngineState,
	) error
	LoadPersistedIncidents(context.Context) ([]model.PersistedIncident, error)
	LoadPersistedGroups(context.Context) ([]model.PersistedGroup, error)
	LoadProviderThreads(
		context.Context,
	) (map[string]map[string]string, error)
	LoadEngineState(context.Context) (model.PersistedEngineState, error)
}

// TelemetryStore persists the last telemetry heartbeat.
type TelemetryStore interface {
	GetTelemetryLastSent(context.Context) (time.Time, error)
	SetTelemetryLastSent(context.Context, time.Time) error
}
