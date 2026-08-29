package correlation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/model"
)

func TestSnapshotPersistedIsSortedAndDetached(t *testing.T) {
	e := newTestEngine()
	e.state[model.IncidentKey("z:owner:reason:")] = &model.Incident{
		Subject: model.Subject{Key: "z:owner:reason:"},
		Status:  model.Status{State: model.StateActive, LastSeen: time.Unix(2, 0)},
	}
	e.state[model.IncidentKey("a:owner:reason:")] = &model.Incident{
		Subject: model.Subject{Key: "a:owner:reason:"},
		Status:  model.Status{State: model.StateActive, LastSeen: time.Unix(1, 0)},
	}

	snapshot := e.SnapshotPersisted()
	require.Len(t, snapshot, 2)
	require.Equal(t, model.IncidentKey("a:owner:reason:"), snapshot[0].Key)
	require.Equal(t, model.IncidentKey("z:owner:reason:"), snapshot[1].Key)

	// Mutating the returned representation must not alter engine state.
	snapshot[0].Reason = "changed"
	require.NotEqual(t, "changed", e.state[model.IncidentKey("a:owner:reason:")].Reason)
}
