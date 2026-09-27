package incident

import (
	"testing"

	"github.com/abahmed/kwatch/internal/model"
)

func TestEngineEmitDropsStaleRevision(t *testing.T) {
	var calls []uint64
	e := newTestEngine(Config{
		LifecycleHook: func(
			inc *model.Incident, _ model.IncidentAction,
		) {
			calls = append(calls, inc.Revision)
		},
	})

	key := model.IncidentKey("ns:owner:Reason:")
	e.emit(transition{
		inc: &model.Incident{
			Subject:  Subject(key),
			Delivery: model.Delivery{Revision: 3},
		},
		action: model.ActionCreate,
	})
	e.emit(transition{
		inc: &model.Incident{
			Subject:  Subject(key),
			Delivery: model.Delivery{Revision: 2},
		},
		action: model.ActionUpdate,
	})

	if len(calls) != 1 || calls[0] != 3 {
		t.Fatalf("expected only revision 3 delivered, got %v", calls)
	}
}

func TestEngineEmitDeliversEqualRevisionRenotify(t *testing.T) {
	var calls int
	e := newTestEngine(Config{
		LifecycleHook: func(
			*model.Incident, model.IncidentAction,
		) {
			calls++
		},
	})

	key := model.IncidentKey("ns:owner:Reason:")
	e.emit(transition{
		inc: &model.Incident{
			Subject:  Subject(key),
			Delivery: model.Delivery{Revision: 5},
		},
		action: model.ActionCreate,
	})
	e.emit(transition{
		inc: &model.Incident{
			Subject:  Subject(key),
			Delivery: model.Delivery{Revision: 5},
		},
		action: model.ActionUpdate,
	})

	if calls != 2 {
		t.Fatalf("expected equal-revision renotify delivered, got %d calls",
			calls)
	}
}

func TestEngineEmitNeverDropsRevisionZero(t *testing.T) {
	var calls int
	e := newTestEngine(Config{
		LifecycleHook: func(
			*model.Incident, model.IncidentAction,
		) {
			calls++
		},
	})

	key := model.IncidentKey("ns:owner:Reason:")
	for i := 0; i < 3; i++ {
		e.emit(transition{
			inc: &model.Incident{
				Subject: Subject(key),
			},
			action: model.ActionCreate,
		})
	}

	if calls != 3 {
		t.Fatalf("expected every revision-0 emission delivered, got %d",
			calls)
	}
}

// Subject builds a minimal model.Subject carrying only the incident key,
// enough to exercise staleEmission's per-key revision tracking.
func Subject(key model.IncidentKey) model.Subject {
	return model.Subject{Key: key}
}
