package pushover

import (
	"context"
	"net/url"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func resolveForm(t *testing.T, cfg map[string]interface{}) url.Values {
	t.Helper()
	rec := providertest.NewRecorder(t)
	cfg["token"], cfg["user"] = "tok", "usr"
	c := NewPushover(cfg, "dev", rec.Dependencies())
	if c == nil {
		t.Fatal("NewPushover() = nil")
	}
	c.url = rec.URL()
	if err := c.SendIncident(
		context.Background(), providertest.Resolve(),
	); err != nil {
		t.Fatalf("SendIncident() error = %v", err)
	}
	form, err := url.ParseQuery(string(rec.Last(t).Body))
	if err != nil {
		t.Fatal(err)
	}
	return form
}

func TestResolvePriorityRules(t *testing.T) {
	cases := []struct {
		name string
		cfg  map[string]interface{}
		want string
	}{
		{"unset is normal", map[string]interface{}{}, ""},
		{"high is capped", map[string]interface{}{"priority": 1}, ""},
		{"low stays low", map[string]interface{}{"priority": -1}, "-1"},
		{"lowest stays", map[string]interface{}{"priority": -2}, "-2"},
		{"override wins", map[string]interface{}{
			"priority": -2, "resolvedPriority": 1}, "1"},
		{"emergency is capped", map[string]interface{}{
			"priority": 2, "retry": 60, "expire": 3600}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			form := resolveForm(t, tc.cfg)
			if form.Get("priority") != tc.want || form.Has("retry") {
				t.Fatalf("form = %v, want priority %q", form, tc.want)
			}
		})
	}
}

func TestResolvedPriorityRejectsEmergencyAndRange(t *testing.T) {
	for _, v := range []int{2, 3, -3} {
		c := NewPushover(map[string]interface{}{
			"token": "tok", "user": "usr", "resolvedPriority": v,
		}, "dev", providertest.NewRecorder(t).Dependencies())
		if c != nil {
			t.Fatalf("resolvedPriority %d must be rejected", v)
		}
	}
}
