//go:build browser

package dashboard

import "testing"

// Issues waiting for their namespace's labels get one notice on the
// health page, counted in the side bar's dot; none at 0.
func TestBrowserHealthShowsScopeWaiting(t *testing.T) {
	js := `(() => [
  [...document.querySelectorAll("[data-blind]")].
    map((n) => n.textContent).join(" / "),
  document.querySelector("[data-kw-dot]").hidden,
  document.querySelector("[data-kw-dot]").dataset.count].join("|"))()`
	st := healthStatus(healthOpts{memory: 0.4})
	st["kwatch"].(map[string]any)["scopeWaiting"] = 3
	srv := stubAPIServer(t, map[string]any{"/api/status": st})
	got := healthText(t, srv, 1280, js)
	want := "3 issues are waiting: kwatch hasn't read their namespace's " +
		"labels yetThey are announced once kwatch has read them, or " +
		"left out if their namespace is out of scope.|false|1"
	if got != want {
		t.Errorf("health page says\n%q, want\n%q", got, want)
	}

	st = healthStatus(healthOpts{memory: 0.4})
	srv = stubAPIServer(t, map[string]any{"/api/status": st})
	if got := healthText(t, srv, 1280, js); got != "|true|0" {
		t.Errorf("without waiting issues the page says %q", got)
	}
}

// An issue kwatch has held for a while over a kind it cannot read says
// so beside its level, on one line.
func TestBrowserLevelNoteSaysKwatchCannotVerify(t *testing.T) {
	line := "kwatch can't verify this right now: it can no longer read " +
		"pod (access was lost)"
	inc := realAnswer(t, "/api/incidents/inc-004")
	inc["why"] = map[string]any{"alertLevelReason": "Urgent: users " +
		"can't reach it.", "unverifiedReason": line}
	srv := listsServer(t, map[string]any{"/api/incidents/inc-004": inc})
	tab := pagingTab(t, srv, 1280, 800, "/issues/inc-004",
		"[data-level-note]")
	got := evalString(t, tab, `(() => {
  const n = document.querySelectorAll("[data-level-note]");
  return n.length + "|" + n[0].textContent;
})()`)
	if got != "1|"+line {
		t.Errorf("level note is %q", got)
	}
}
