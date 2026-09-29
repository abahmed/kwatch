package slack

import (
	"context"
	"sort"
	"strings"

	slackClient "github.com/slack-go/slack"
)

// conversationState is the Slack thread that carries one story.
type conversationState struct {
	ThreadTS string
}

// staleThreadErrors are Slack API errors meaning the thread root is gone,
// for example deleted or from a channel the bot left.
var staleThreadErrors = []string{
	"invalid_thread_ts", "thread_not_found", "message_not_found",
}

func isStaleThreadError(err error) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	for _, code := range staleThreadErrors {
		if strings.Contains(text, code) {
			return true
		}
	}
	return false
}

// postWithThreadFallback posts into threadTS and, when Slack reports the
// thread no longer exists, posts once at top level so the update is not
// lost. It returns the thread to use from now on.
func postWithThreadFallback(
	ctx context.Context,
	post func(context.Context, *slackClient.Blocks, string) (string, error),
	blocks *slackClient.Blocks,
	threadTS string,
) (string, error) {
	_, err := post(ctx, blocks, threadTS)
	if threadTS == "" || !isStaleThreadError(err) {
		return threadTS, err
	}
	ts, err := post(ctx, blocks, "")
	if err != nil {
		return threadTS, err
	}
	return ts, nil
}

func (s *Slack) postBlocks(
	ctx context.Context,
	blocks *slackClient.Blocks,
	threadTS string,
) (string, error) {
	opts := []slackClient.MsgOption{
		slackClient.MsgOptionBlocks(blocks.BlockSet...),
		slackClient.MsgOptionAsUser(true),
	}
	if threadTS != "" {
		opts = append(opts, slackClient.MsgOptionTS(threadTS))
	}
	_, ts, err := s.apiClient.PostMessageContext(ctx, s.channel, opts...)
	return ts, wrapSlackRateLimit(err)
}

// SnapshotThreads implements delivery.ThreadStateProvider: story key to
// thread timestamp.
func (s *Slack) SnapshotThreads() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.conversations) == 0 {
		return nil
	}
	out := make(map[string]string, len(s.conversations))
	for key, state := range s.conversations {
		out[key] = state.ThreadTS
	}
	return out
}

// RestoreThreads implements delivery.ThreadStateProvider. Keys are adopted
// in a stable order so the size bound applies deterministically, and a
// thread already posted by this run always wins over a saved one.
func (s *Slack) RestoreThreads(saved map[string]string) {
	keys := make([]string, 0, len(saved))
	for key, ts := range saved {
		if ts != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		s.mu.Lock()
		_, live := s.conversations[key]
		s.mu.Unlock()
		if !live {
			s.saveConversation(key, conversationState{ThreadTS: saved[key]})
		}
	}
}
