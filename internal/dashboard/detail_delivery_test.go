package dashboard

import (
	"fmt"
	"testing"
	"time"
)

func TestDetailDeliveriesNameChannelsAndRedact(t *testing.T) {
	s := newTestServer(testIncident("a", "shop", 0))
	var asked string
	s.SetDeliveryTrail(func(id string) []DeliveryAttempt {
		asked = id
		return []DeliveryAttempt{
			{At: testEpoch, Channel: "pagerduty", Kind: "incident",
				Placement: "root", Result: "retrying",
				Error: "401 token=abcdef0123456789"},
			{At: testEpoch.Add(time.Minute), Kind: "incident",
				Result: "carried", Carrier: "digest"},
		}
	})
	d := detailOf(t, s, "a")
	if asked != "a" || len(d.Deliveries) != 2 {
		t.Fatalf("asked %q deliveries %+v", asked, d.Deliveries)
	}
	first := d.Deliveries[0]
	if first.Name != "PagerDuty" || first.Error == "" ||
		first.Error == "401 token=abcdef0123456789" {
		t.Errorf("first = %+v", first)
	}
	if d.Deliveries[1].Name != "" || d.Deliveries[1].Carrier != "digest" {
		t.Errorf("carried = %+v", d.Deliveries[1])
	}
}

func TestDetailDeliveriesNameTheChannelAFallbackStoodInFor(
	t *testing.T,
) {
	got, _ := toDeliveries([]DeliveryAttempt{{Channel: "opsgenie",
		Result: "ok", Fallback: "pagerduty"}})
	if got[0].Name != "Opsgenie" || got[0].Fallback != "PagerDuty" {
		t.Errorf("fallback row = %+v", got[0])
	}
}

func TestDetailDeliveriesKeepTheNewest(t *testing.T) {
	var trail []DeliveryAttempt
	for i := range maxDeliveries + 5 {
		trail = append(trail, DeliveryAttempt{Channel: "slack",
			Result: fmt.Sprint(i)})
	}
	got, cut := toDeliveries(trail)
	if len(got) != maxDeliveries || cut != 5 || got[0].Result != "5" {
		t.Errorf("kept %d cut %d first %+v", len(got), cut, got[0])
	}
	if got, cut := toDeliveries(nil); got != nil || cut != 0 {
		t.Errorf("empty trail = %+v %d", got, cut)
	}
}
