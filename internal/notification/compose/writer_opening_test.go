package compose

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
)

// An announcement opens its conversation. Updates and resolves carry
// the announcement as it would read now, so delivery can send a full
// first message to a provider that lost the original.
func TestWriterMarksOpeningAndCarriesIt(t *testing.T) {
	announcement := Writer{}.Write(announce(badRollout()), at(5, 0))
	if !announcement.Opens || announcement.Opening != nil {
		t.Fatalf("announcement: Opens=%v Opening=%v", announcement.Opens,
			announcement.Opening)
	}

	update := writeSpreading()
	resolve := writeResolvedByRollback()
	for name, msg := range map[string]notification.Message{
		"update": update, "resolve": resolve,
	} {
		t.Run(name, func(t *testing.T) {
			if msg.Opens {
				t.Fatal("only the announcement opens the conversation")
			}
			o := msg.Opening
			if o == nil || !o.Opens || o.Opening != nil || o.Key != msg.Key {
				t.Fatalf("opening = %+v", o)
			}
			if o.Status == notification.StatusResolved ||
				!strings.Contains(o.Note, "payments is down in shop") {
				t.Fatalf("opening must read as the announcement: %q",
					o.Note)
			}
		})
	}
}

func TestStartupSummaryOpensItsConversation(t *testing.T) {
	msg := Writer{}.StartupSummary([]incident.Decision{
		podDecision("a", incident.Notify)}, at(0, 0))

	if !msg.Opens {
		t.Fatal("the startup summary is the first message of its key")
	}
}
