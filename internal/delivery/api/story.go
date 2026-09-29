package api

import (
	"context"

	"github.com/abahmed/kwatch/internal/notice"
)

// StoryProvider renders kwatch's problem messages natively: threads,
// edits in place, rich formatting. Providers without it receive the
// plain-text rendering through SendMessage, or an alert event when they
// deliver through SendEvent.
type StoryProvider interface {
	SendStory(context.Context, notice.Message) error
}
