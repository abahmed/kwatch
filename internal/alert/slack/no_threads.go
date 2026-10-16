package slack

import (
	"context"

	"github.com/abahmed/kwatch/internal/notification"
)

// DisableThreads implements api.ThreadSwitch. Without threads an update
// or resolve is a new top-level message, as it is in webhook mode.
func (s *Slack) DisableThreads() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.noThreads = true
}

func (s *Slack) threadsOff() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.noThreads
}

// sendUnthreaded posts m as its own top-level message and keeps no
// conversation state.
func (s *Slack) sendUnthreaded(
	ctx context.Context, m notification.Message,
) error {
	blocks := noteBlocks(m)
	if m.Resolved() {
		blocks = rootBlocks(m)
	}
	_, err := s.poster(fallbackText(m))(ctx, blocks, "")
	return err
}
