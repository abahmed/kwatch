package incident

import (
	"testing"
	"time"
)

func TestConfigDefaultsFillZeroValues(t *testing.T) {
	c := Config{}.withDefaults()
	if c.Settle != DefaultSettle || c.PageSettle != DefaultPageSettle ||
		c.Hold != DefaultHold || c.MaxHold != DefaultMaxHold ||
		c.FlapWindow != DefaultFlapWindow ||
		c.FlapCycles != DefaultFlapCycles ||
		c.Remember != DefaultRemember {
		t.Fatalf("defaults not applied: %+v", c)
	}
}

func TestConfigDefaultsKeepExplicitValues(t *testing.T) {
	c := Config{Settle: time.Second, FlapCycles: 5}.withDefaults()
	if c.Settle != time.Second || c.FlapCycles != 5 {
		t.Fatalf("explicit values overwritten: %+v", c)
	}
}

func TestConfigHoldDoublesPerRecentRecoveryUpToMax(t *testing.T) {
	c := Config{}.withDefaults()
	cases := map[int]time.Duration{
		0: 3 * time.Minute, 1: 6 * time.Minute, 2: 12 * time.Minute,
		3: 24 * time.Minute, 4: 30 * time.Minute, 9: 30 * time.Minute,
	}
	for cycles, want := range cases {
		if got := c.hold(cycles); got != want {
			t.Errorf("hold(%d) = %v, want %v", cycles, got, want)
		}
	}
}

func TestOrdinalSuffixes(t *testing.T) {
	cases := map[int]string{
		1: "1st", 2: "2nd", 3: "3rd", 4: "4th", 11: "11th", 12: "12th",
		13: "13th", 21: "21st", 22: "22nd", 101: "101st", 111: "111th",
	}
	for n, want := range cases {
		if got := ordinal(n); got != want {
			t.Errorf("ordinal(%d) = %s, want %s", n, got, want)
		}
	}
}

func TestIncidentTimelineIsBounded(t *testing.T) {
	p := &Incident{}
	for i := 0; i < 60; i++ {
		p.note(at(time.Duration(i)*time.Second), "e")
	}
	if len(p.Timeline) != 50 || !p.Timeline[0].At.Equal(at(10*time.Second)) {
		t.Fatalf("timeline = %d entries from %v",
			len(p.Timeline), p.Timeline[0].At)
	}
}

func TestOccurrencesAreBounded(t *testing.T) {
	var times []time.Time
	for i := 0; i < 25; i++ {
		times = appendOccurrence(times, at(time.Duration(i)*time.Hour))
	}
	if len(times) != maxOccurrences ||
		!times[0].Equal(at(5*time.Hour)) {
		t.Fatalf("occurrences = %d from %v", len(times), times[0])
	}
}

func TestTimeOfDayDistanceWrapsAroundMidnight(t *testing.T) {
	a := time.Date(2026, 1, 1, 23, 50, 0, 0, time.UTC)
	b := time.Date(2026, 1, 9, 0, 10, 0, 0, time.UTC)
	if got := timeOfDayDistance(a, b); got != 20*time.Minute {
		t.Fatalf("distance = %v, want 20m", got)
	}
}
