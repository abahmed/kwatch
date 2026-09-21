package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/model"
)

type incidentSaverStub struct {
	saves int
	err   error
}

func (s *incidentSaverStub) SaveIncidentState(
	context.Context,
	[]model.PersistedIncident,
	[]model.PersistedGroup,
	map[string]map[string]string,
	model.PersistedEngineState,
) error {
	s.saves++
	return s.err
}

func TestIncidentSnapshotHelpersCoalesceAndFingerprintState(t *testing.T) {
	ch := make(chan stateSnapshot, 1)
	first := stateSnapshot{incidents: []model.PersistedIncident{{Key: "a"}}}
	second := stateSnapshot{incidents: []model.PersistedIncident{{Key: "b"}}}
	trySendIncidentSnapshot(ch, first)
	trySendIncidentSnapshot(ch, second)
	got := <-ch
	if len(got.incidents) != 1 || got.incidents[0].Key != "b" {
		t.Fatalf("coalesced snapshot = %+v", got)
	}
	left, leftOK := snapshotFingerprint(first)
	right, rightOK := snapshotFingerprint(second)
	if !leftOK || !rightOK || left == right {
		t.Fatalf("fingerprints = %d/%v and %d/%v", left, leftOK, right, rightOK)
	}
	copyOfFirst, copyOK := snapshotFingerprint(first)
	if !copyOK || copyOfFirst != left {
		t.Fatal("snapshot fingerprint was not deterministic")
	}
}

func TestSaveIncidentSnapshotSuppressesDuplicatesAndReportsErrors(
	t *testing.T,
) {
	saver := &incidentSaverStub{}
	snap := stateSnapshot{incidents: []model.PersistedIncident{{Key: "a"}}}
	reported := 0
	report := func(err error) {
		reported++
	}
	first, err := saveIncidentSnapshot(
		context.Background(), saver, snap, time.Second, 0, report,
		func() bool { return true },
	)
	if err != nil || first == 0 || saver.saves != 1 || reported != 1 {
		t.Fatalf(
			"first save = %d, %v, saves=%d, reports=%d",
			first, err, saver.saves, reported,
		)
	}
	second, err := saveIncidentSnapshot(
		context.Background(), saver, snap, time.Second, first, report,
		func() bool { return true },
	)
	if err != nil || second != first || saver.saves != 1 {
		t.Fatal("duplicate snapshot was written")
	}
	if next, err := saveIncidentSnapshot(
		context.Background(), saver, snap, time.Second, first, report,
		func() bool { return false },
	); err != nil || next != first {
		t.Fatalf("disabled write = %d, %v", next, err)
	}
	saver.err = errors.New("persist failed")
	_, err = saveIncidentSnapshot(
		context.Background(), saver, stateSnapshot{
			groups: []model.PersistedGroup{{GroupKey: "group"}},
		}, time.Second, 0, report, func() bool { return true },
	)
	if err == nil || reported != 2 {
		t.Fatalf("failed save = %v, reports=%d", err, reported)
	}
}
