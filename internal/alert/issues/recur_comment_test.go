package issues

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// A resolve whose state change was never completed (delivery gave up)
// must not stop the next resolve, after the incident recurred, from
// posting its comment.
func TestRecurrenceClearsThePostedResolveComment(t *testing.T) {
	m := NewMap(time.Now)
	tr := &splitCloseTracker{stateErrs: 1}
	openIssue(t, m, tr)
	ctx := t.Context()
	if err := m.Deliver(ctx, tr, providertest.Resolve(),
		"t", "b"); err == nil {
		t.Fatal("want the state failure back")
	}
	deliver(t, m, tr, providertest.Update())
	deliver(t, m, tr, providertest.Resolve())

	want := "create:t,comment:42,state:42,comment:42,comment:42,state:42"
	if got := strings.Join(tr.calls, ","); got != want {
		t.Fatalf("calls = %s, want %s", got, want)
	}
}
