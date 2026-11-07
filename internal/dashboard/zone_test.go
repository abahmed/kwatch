package dashboard

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
	_ "time/tzdata"

	"github.com/abahmed/kwatch/internal/incident"
)

// A week across a daylight saving change counts each day in the zone's
// own offset that day, not in today's offset.
func TestTrendsCountDaysAcrossADaylightSavingChange(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	// 22:30 UTC on 22 Oct is 00:30 on 23 Oct in Berlin (summer time,
	// UTC+2), but 23:30 on 22 Oct at today's UTC+1.
	at := time.Date(2026, 10, 22, 22, 30, 0, 0, time.UTC)
	items := []incident.Incident{opened("a", "shop", incident.Page, at)}
	zoned := buildTrends(items, trendNow, berlin, 0)
	fixed := buildTrends(items, trendNow, offsetZone(60), 0)
	if !strings.Contains(dayTotals(zoned), "10-23=1 ") {
		t.Errorf("Berlin days: %s", dayTotals(zoned))
	}
	if !strings.Contains(dayTotals(fixed), "10-22=1 ") {
		t.Errorf("UTC+1 days: %s", dayTotals(fixed))
	}
}

func TestParseZonePrefersTheZoneNameAndFallsBackToTheOffset(t *testing.T) {
	cases := []struct {
		query string
		want  string
	}{
		{"", "UTC"},
		{"tz=60", "+0100"},
		{"tzName=Africa/Cairo&tz=60", "Africa/Cairo"},
		{"tzName=Mars/Olympus&tz=60", "+0100"},
		{"tzName=Local&tz=0", "UTC"},
	}
	for _, c := range cases {
		q, _ := url.ParseQuery(c.query)
		zone, err := parseZone(q)
		if err != nil {
			t.Fatalf("%q: %v", c.query, err)
		}
		got := zone.String()
		if got == "" {
			got = time.Date(2026, 1, 1, 0, 0, 0, 0, zone).Format("-0700")
		}
		if got != c.want {
			t.Errorf("%q: zone %q, want %q", c.query, got, c.want)
		}
	}
}

func TestTrendsRefuseANameThatCannotBeAZone(t *testing.T) {
	s := newTestServer()
	for _, name := range []string{"../../etc/passwd", "/UTC",
		strings.Repeat("A", maxZoneName+1), "Europe/Berlin%00"} {
		w := get(t, s, "/api/overview?tzName="+url.QueryEscape(name))
		got := decodeBadQuery(t, w)
		if w.Code != http.StatusBadRequest || got.Param != "tzName" {
			t.Errorf("%q: %d %s", name, w.Code, w.Body.String())
		}
	}
	w := get(t, s, "/api/overview?tzName=Europe/Berlin")
	if w.Code != http.StatusOK {
		t.Errorf("Europe/Berlin: %d %s", w.Code, w.Body.String())
	}
}
