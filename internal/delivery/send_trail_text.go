package delivery

import (
	"maps"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/notification"
)

// maxSentTextBytes bounds the text the trail keeps of one message.
const maxSentTextBytes = 2048

// SentText is what kwatch sent one provider for an incident: the text of
// the latest message it accepted, so the dashboard can show "what we
// sent". It is the provider-neutral plain text after the user template,
// the fallback notice and the provider's size cut, before the provider's
// own markup and escaping; the message text was redacted when it was
// composed. Only the latest message per provider is kept.
type SentText struct {
	At   time.Time `json:"at"`
	Kind string    `json:"kind,omitempty"`
	Text string    `json:"text"`
	// Cut reports that Text was shortened to maxSentTextBytes.
	Cut bool `json:"cut,omitempty"`
}

// sentTextOf is the plain text of a prepared incident message: the note
// and the application output after it.
func sentTextOf(m notification.Message) string {
	text := m.NoteText()
	if len(m.Output) > 0 {
		text += "\n\n" + strings.Join(m.Output, "\n")
	}
	return text
}

func newSentText(at time.Time, kind, text string) SentText {
	kept := notification.Truncate(text, maxSentTextBytes)
	return SentText{At: at, Kind: kind, Text: kept, Cut: kept != text}
}

// noteText remembers the latest text a provider accepted for key. The
// attempt added right after it marks the key for the next write.
func (t *sendTrail) noteText(key, provider string, s SentText) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.sent == nil {
		t.sent = map[string]map[string]SentText{}
	}
	if t.sent[key] == nil {
		t.sent[key] = map[string]SentText{}
	}
	t.sent[key][provider] = s
}

// restoreTextLocked puts saved texts behind what this session already
// noted: a text of this session is newer. The caller holds t.mu.
func (t *sendTrail) restoreTextLocked(key string, saved map[string]SentText) {
	if len(saved) == 0 {
		return
	}
	if t.sent == nil {
		t.sent = map[string]map[string]SentText{}
	}
	merged := make(map[string]SentText, len(saved))
	for provider, s := range saved {
		merged[provider] = s
	}
	maps.Copy(merged, t.sent[key])
	t.sent[key] = merged
}

// textsLocked is a copy of key's texts, nil when there are none. The
// caller holds t.mu.
func (t *sendTrail) textsLocked(key string) map[string]SentText {
	if len(t.sent[key]) == 0 {
		return nil
	}
	return maps.Clone(t.sent[key])
}

// SentTexts returns the text of the latest message each provider
// accepted for the incident with id key, by provider name; nil when none
// is remembered.
func (m *Manager) SentTexts(key string) map[string]SentText {
	m.trail.mu.Lock()
	defer m.trail.mu.Unlock()
	return m.trail.textsLocked(key)
}
