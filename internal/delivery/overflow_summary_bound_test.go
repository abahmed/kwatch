package delivery

import (
	"fmt"
	"strings"
	"testing"
)

// A summary stores at most maxOverflowReasonKeys reasons however many
// distinct titles overflow, and still counts every notification.
func TestOverflowSummaryBoundsDistinctReasons(t *testing.T) {
	state := &overflowSummary{byReason: map[string]int{}}
	const sent = maxOverflowReasonKeys * 3
	for i := range sent {
		state.count(fmt.Sprintf("title %d", i), 1)
	}
	if len(state.byReason) != maxOverflowReasonKeys {
		t.Fatalf("stored %d reasons, want %d",
			len(state.byReason), maxOverflowReasonKeys)
	}
	if state.total != sent {
		t.Fatalf("counted %d notifications, want %d", state.total, sent)
	}
	if want := sent - maxOverflowReasonKeys; state.unnamed != want {
		t.Fatalf("unnamed %d, want %d", state.unnamed, want)
	}
	if text := renderOverflowSummary(state); !strings.Contains(
		text, "more of other kinds") {
		t.Fatalf("summary hides the unnamed notifications: %s", text)
	}
}
