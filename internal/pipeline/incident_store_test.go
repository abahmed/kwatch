package pipeline

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

func TestIncidentStoreRoundTripsIncidents(t *testing.T) {
	ps := NewIncidentStore(openTempStore(t))
	root := inventory.CoreID("node", "", "n1")
	records := []incident.Record{
		{ID: "a", Root: root, Opened: time.Unix(10, 0).UTC()},
		{ID: "b", Root: root},
	}

	if err := ps.SaveIncidents(records); err != nil {
		t.Fatal(err)
	}
	got, err := ps.LoadIncidents()

	if err != nil || len(got) != 2 {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestIncidentStoreSaveRemovesDroppedIncidents(t *testing.T) {
	ps := NewIncidentStore(openTempStore(t))
	_ = ps.SaveIncidents([]incident.Record{{ID: "a"}, {ID: "b"}})

	if err := ps.SaveIncidents([]incident.Record{{ID: "b"}}); err != nil {
		t.Fatal(err)
	}
	got, _ := ps.LoadIncidents()

	if len(got) != 1 || got[0].ID != "b" {
		t.Fatalf("got %v, want only b", got)
	}
}

func TestIncidentStoreRoundTripsFingerprints(t *testing.T) {
	ps := NewIncidentStore(openTempStore(t))

	err := ps.SaveFingerprints(map[string]any{"k": "v", "j": "w"})
	got, loadErr := ps.LoadFingerprints()

	if err != nil || loadErr != nil {
		t.Fatal(err, loadErr)
	}
	if len(got) != 2 || got["k"] != "v" {
		t.Fatalf("got %v", got)
	}
}

func TestIncidentStoreLoadFailsAfterClose(t *testing.T) {
	s := openTempStore(t)
	ps := NewIncidentStore(s)
	_ = s.Close()

	if _, err := ps.LoadIncidents(); err == nil {
		t.Error("LoadIncidents on a closed store must fail")
	}
	if _, err := ps.LoadFingerprints(); err == nil {
		t.Error("LoadFingerprints on a closed store must fail")
	}
	if err := ps.SaveIncidents(nil); err == nil {
		t.Error("SaveIncidents on a closed store must fail")
	}
}

func TestIncidentStoreResolvedIncidentExpiresAfterHistory(t *testing.T) {
	ps := NewIncidentStore(openTempStore(t))
	now := time.Unix(1_800_000_000, 0)
	records := []incident.Record{
		{ID: "old", State: incident.Resolved,
			Resolved: now.Add(-resolvedHistory - time.Minute)},
		{ID: "recent", State: incident.Resolved,
			Resolved: now.Add(-time.Hour)},
		{ID: "open", State: incident.Open, Opened: now.Add(-30 * 24 *
			time.Hour)},
	}
	if err := ps.SaveIncidents(records); err != nil {
		t.Fatal(err)
	}

	got, err := ps.LoadIncidents()

	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, r := range got {
		ids = append(ids, r.ID)
	}
	if len(ids) != 2 || ids[0] != "open" || ids[1] != "recent" {
		t.Fatalf("got %v, want open and recent", ids)
	}
}

func TestIncidentStoreRoundTripsStartupMarker(t *testing.T) {
	ps := NewIncidentStore(openTempStore(t))
	if _, found, err := ps.LoadStartup(); err != nil || found {
		t.Fatalf("empty store: found=%v err=%v", found, err)
	}
	if err := ps.SaveStartup(StartupState{}); err != nil {
		t.Fatal(err)
	}
	if _, found, err := ps.LoadStartup(); err != nil || !found {
		t.Fatalf("saved marker: found=%v err=%v", found, err)
	}
}
