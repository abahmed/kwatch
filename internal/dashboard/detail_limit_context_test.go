package dashboard

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// contextHeavy is a detail whose context parts alone are far over the
// response limit.
func contextHeavy() IncidentDetail {
	pad := strings.Repeat("z", 400)
	d := IncidentDetail{SeenBefore: &SeenBefore{}}
	for i := range 200 {
		d.Changes = append(d.Changes, ChangeItem{Name: fmt.Sprint(i),
			Summary: pad})
		d.Containers = append(d.Containers, ContainerRow{
			Pod: fmt.Sprint(i), Image: pad})
		d.Deliveries = append(d.Deliveries, DeliveryAttempt{
			Result: fmt.Sprint(i), Error: pad})
		d.Affected = append(d.Affected, AffectedObject{
			Name: fmt.Sprint(i), Commands: []string{"kubectl " + pad}})
		d.SeenBefore.Recent = append(d.SeenBefore.Recent,
			PastOccurrence{Fix: fmt.Sprint(i), Mode: pad})
	}
	return d
}

func TestFitDetailCutsContextPartsToFit(t *testing.T) {
	d := contextHeavy()
	fitDetail(&d, detailBudget)
	if size := detailSize(&d); size > detailBudget {
		t.Fatalf("detail is %d bytes", size)
	}
	tr := d.Truncated
	if tr == nil || tr.Texts == 0 || tr.Occurrences == 0 ||
		tr.Deliveries == 0 || tr.Affected == 0 {
		t.Fatalf("truncated = %+v", tr)
	}
	if len(d.Changes)+tr.Changes != 200 ||
		len(d.Containers)+tr.Containers != 200 ||
		len(d.Deliveries)+tr.Deliveries != 200 ||
		len(d.Affected)+tr.Affected != 200 {
		t.Errorf("kept and cut do not add up: %+v", tr)
	}
	last := d.Deliveries[len(d.Deliveries)-1]
	if last.Result != "199" {
		t.Errorf("newest delivery not kept: %+v", last)
	}
	if d.Changes[0].Name != "0" || d.Affected[0].Name != "0" {
		t.Error("the most relevant entries were not kept")
	}
	if !strings.HasPrefix(d.Affected[0].Commands[0], "kubectl zzz") ||
		len(d.Affected[0].Commands[0]) != len("kubectl ")+400 {
		t.Error("a command was shortened")
	}
}

func TestFitDetailContextIsDeterministic(t *testing.T) {
	const budget = 16 << 10
	a, b := contextHeavy(), contextHeavy()
	fitDetail(&a, budget)
	fitDetail(&b, budget)
	if detailSize(&a) > budget {
		t.Fatalf("detail is %d bytes", detailSize(&a))
	}
	if detailSize(&a) != detailSize(&b) || *a.Truncated != *b.Truncated {
		t.Error("two fits of the same detail differ")
	}
}

func TestDetailOmitsContextWithoutData(t *testing.T) {
	p := testIncident("a", "shop", -time.Hour)
	body := get(t, newTestServer(p), "/api/incidents/a").Body.String()
	for _, field := range []string{`"changes"`, `"fixedBy"`,
		`"seenBefore"`, `"containers"`, `"deliveries"`, `"fix"`} {
		if strings.Contains(body, field) {
			t.Errorf("detail without data has %s: %s", field, body)
		}
	}
	for _, field := range []string{`"affected"`, `"phases"`} {
		if !strings.Contains(body, field) {
			t.Errorf("detail lacks %s: %s", field, body)
		}
	}
}
