package slack

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/notification"
)

// A persisted thread is a single string: the Slack thread timestamp, and
// when the announcement is known, a newline and its stored form as JSON.
// Slack timestamps never contain a newline, and a bare timestamp (state
// saved before roots were stored) still decodes.
const threadSeparator = "\n"

// maxStoredOutputChars bounds the persisted output excerpt. The note is
// already cut to Slack's section limit when it is rendered, so it is
// stored at that size; with the output this keeps one record near 5 KB,
// and the conversation map is bounded by maxThreadMapSize.
const maxStoredOutputChars = 1500

// storedRoot is what editRoot needs to rebuild the announcement under a
// new marker: the lead, the note and the output excerpt, nothing more.
type storedRoot struct {
	Marker string `json:"marker,omitempty"`
	Status uint8  `json:"status,omitempty"`
	Short  string `json:"short"`
	Note   string `json:"note"`
	Output string `json:"output,omitempty"`
	// Doc is the structured narrative, kept only when it is small, so
	// an edited root keeps its bold names and lists.
	Doc []notification.Block `json:"doc,omitempty"`
	// Rollup marks a member of a roll-up: the roll-up's conversation key.
	// Such a record holds no announcement of its own.
	Rollup string `json:"rollup,omitempty"`
	// ReopenUntil is the end of the reopen window, in Unix seconds, of a
	// resolved conversation that is kept for a possible reopen.
	ReopenUntil int64 `json:"reopen_until,omitempty"`
}

// encodeThread is the persisted form of one conversation.
func encodeThread(state conversationState) string {
	if state.Rollup != "" {
		return encodeStored(state.ThreadTS, storedRoot{Rollup: state.Rollup})
	}
	if state.Root == nil && state.ReopenUntil.IsZero() {
		return state.ThreadTS
	}
	var stored storedRoot
	if state.Root != nil {
		stored = newStoredRoot(*state.Root)
	}
	if !state.ReopenUntil.IsZero() {
		stored.ReopenUntil = state.ReopenUntil.Unix()
	}
	return encodeStored(state.ThreadTS, stored)
}

func encodeStored(threadTS string, stored storedRoot) string {
	data, err := json.Marshal(stored)
	if err != nil {
		return threadTS
	}
	return threadTS + threadSeparator + string(data)
}

// decodeThread reads a persisted thread. A value without a readable root
// yields a state with only the timestamp, which editRoot handles with a
// short status line.
func decodeThread(value string) conversationState {
	ts, rest, found := strings.Cut(value, threadSeparator)
	state := conversationState{ThreadTS: ts}
	if !found {
		return state
	}
	var stored storedRoot
	if err := json.Unmarshal([]byte(rest), &stored); err != nil {
		return state
	}
	if stored.Rollup != "" {
		state.Rollup = stored.Rollup
		return state
	}
	if stored.ReopenUntil != 0 {
		state.ReopenUntil = time.Unix(stored.ReopenUntil, 0)
	}
	if stored.Short != "" || stored.Note != "" {
		root := stored.message()
		state.Root = &root
	}
	return state
}

func newStoredRoot(m notification.Message) storedRoot {
	return storedRoot{
		Marker: m.Marker,
		Status: uint8(m.Status),
		Short:  truncateMrkdwn(m.ShortText(), maxSectionTextChars),
		Note:   truncateMrkdwn(m.NoteText(), maxSectionTextChars),
		Output: truncateMrkdwn(
			strings.Join(m.Output, "\n"), maxStoredOutputChars),
		Doc: storedDoc(m),
	}
}

func (r storedRoot) message() notification.Message {
	m := notification.Message{
		Marker: r.Marker,
		Status: notification.Status(r.Status),
		Short:  r.Short,
		Note:   r.Note,
		Doc:    r.Doc,
	}
	if r.Output != "" {
		m.Output = []string{r.Output}
	}
	return m
}

// storedDoc is the message's blocks when they fit the stored size.
func storedDoc(m notification.Message) []notification.Block {
	size := 0
	for _, b := range m.Doc {
		size += len(b.Text())
	}
	if size > maxSectionTextChars {
		return nil
	}
	return m.Doc
}
