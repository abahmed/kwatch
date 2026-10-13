package storage

import "testing"

// The per-bucket caps are the most history the compactor keeps. Their sum
// must leave most of the total cap to what is never evicted (incidents,
// baselines, fingerprints, state, threads, the outbox) and to the audit
// log, so the always-on flight recording cannot crowd them out.
func TestHistoryCapsLeaveRoomUnderTheSizeCap(t *testing.T) {
	p := DefaultPolicy()
	capped := p.EvidenceCap + p.TimelineCap + p.ChangesCap + p.FlightCap
	if capped >= p.SizeCap {
		t.Fatalf("capped history %d MiB reaches the %d MiB size cap",
			capped>>20, p.SizeCap>>20)
	}
	if free := p.SizeCap - capped; free < p.SizeCap/2 {
		t.Errorf("only %d of %d MiB is left for data that is never evicted",
			free>>20, p.SizeCap>>20)
	}
	t.Logf("capped history %d MiB of %d MiB", capped>>20, p.SizeCap>>20)
}
