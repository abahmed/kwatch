package slack

import (
	"context"
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/notification"
)

// isRollup reports whether m is a roll-up or its closing resolve.
func isRollup(m notification.Message) bool {
	return strings.HasPrefix(m.Key, notification.RollupKeyPrefix)
}

// sendRollup posts a roll-up as one top-level message and remembers its
// thread for every incident it announced, so their updates and resolves
// reply under it. The closing resolve edits the roll-up message.
func (s *Slack) sendRollup(
	ctx context.Context, m notification.Message,
) error {
	// A roll-up writes its members' conversations too, so it holds their
	// locks as well as its own.
	unlock := s.lockConversations(append([]string{m.Key}, m.Members...))
	defer unlock()
	post := s.poster(fallbackText(m))
	if m.Resolved() {
		return s.closeRollup(ctx, post, m)
	}
	ts, err := post(ctx, noteBlocks(m), "")
	if err != nil {
		return err
	}
	s.saveRollup(m, ts)
	return nil
}

// lockConversations locks the stripes guarding keys and returns the
// function that releases them. Stripes are taken in ascending order, each
// once (two keys may share a stripe), so two roll-ups with overlapping
// members cannot deadlock; a single incident send holds only one stripe.
func (s *Slack) lockConversations(keys []string) func() {
	seen := make(map[uint32]bool, len(keys))
	stripes := make([]uint32, 0, len(keys))
	for _, key := range keys {
		if index := lockIndex(key); !seen[index] {
			seen[index] = true
			stripes = append(stripes, index)
		}
	}
	sort.Slice(stripes, func(i, j int) bool {
		return stripes[i] < stripes[j]
	})
	for _, index := range stripes {
		s.conversationLocks[index].Lock()
	}
	return func() {
		for i := len(stripes) - 1; i >= 0; i-- {
			s.conversationLocks[stripes[i]].Unlock()
		}
	}
}

// saveRollup records the member threads first and the roll-up last, so
// the size bound evicts the oldest incident rather than the roll-up that
// the rest still depend on. An incident that already has a conversation
// keeps it.
func (s *Slack) saveRollup(m notification.Message, ts string) {
	if len(m.Members) == 0 {
		return
	}
	for _, key := range m.Members {
		s.mu.Lock()
		_, live := s.conversations[key]
		s.mu.Unlock()
		if !live {
			s.saveConversation(key,
				conversationState{ThreadTS: ts, Rollup: m.Key})
		}
	}
	s.saveConversation(m.Key, conversationState{ThreadTS: ts, Root: &m})
}

// closeRollup marks the roll-up message resolved through the root edit
// path once every member has resolved. Without the roll-up's saved root
// (webhook-less tests, an evicted or older conversation) the resolve is a
// short standalone line.
func (s *Slack) closeRollup(
	ctx context.Context, post postFunc, m notification.Message,
) error {
	s.mu.Lock()
	state, known := s.conversations[m.Key]
	s.mu.Unlock()
	if !known || state.Root == nil || s.apiClient == nil {
		_, err := post(ctx, rootBlocks(m), "")
		return err
	}
	if _, err := s.editRoot(ctx, state, m); err != nil {
		return err
	}
	s.deleteConversation(m.Key)
	return nil
}
