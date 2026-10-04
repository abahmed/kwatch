//go:build e2e

package harness

import (
	"encoding/json"
	"testing"
)

func TestDeliveryPayloadMatchersReadNotificationFields(t *testing.T) {
	var payload map[string]any
	body := `{"title":"persistent is failing in apps (e2e) after release",` +
		`"route":{"Reasons":["CrashLoopBackOff","FailedMount"]}}`
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		got  bool
		want bool
	}{
		"name as a word":        {titleNames(payload, "persistent"), true},
		"name inside a word":    {titleNames(payload, "persist"), false},
		"namespace in title":    {titleNames(payload, "apps"), true},
		"unrelated name":        {titleNames(payload, "other"), false},
		"listed reason":         {routeHasReason(payload, "FailedMount"), true},
		"reason not on route":   {routeHasReason(payload, "OOMKilled"), false},
		"payload without route": {routeHasReason(map[string]any{}, "x"), false},
	} {
		if tc.got != tc.want {
			t.Errorf("%s: got %v, want %v", name, tc.got, tc.want)
		}
	}
}

func TestTitleNamesSearchesRollupLinesOnly(t *testing.T) {
	roll := map[string]any{"key": "rollup/x",
		"title": "kwatch found 2 new problems at the same time.",
		"lines": []any{"web is failing in apps", "api is failing in apps"}}
	if !titleNames(roll, "api") {
		t.Error("roll-up line should name api")
	}
	roll["key"] = "incident/x"
	if titleNames(roll, "api") {
		t.Error("lines of a normal message must not match")
	}
}
