package slack

import (
	"context"
	"sort"
	"strings"
	"time"

	slackClient "github.com/slack-go/slack"

	"github.com/abahmed/kwatch/internal/notification"
)

// conversationState is the Slack thread that carries one incident.
// Root is the message the root post was built from, kept so the root can
// be edited under a new status. It is persisted with ThreadTS (see
// root_state.go) and is nil only for state saved before that existed.
//
// Rollup is set when the incident was announced inside a roll-up: ThreadTS
// is then the roll-up message's thread, the roll-up owns the root (so Root
// is nil and is never edited for this incident), and the key names the
// roll-up conversation.
//
// ReopenUntil is set after a resolve whose incident may reopen (see
// notification.Message.ReopenWithin): the thread is kept until then so a
// "failing again" update replies in it. Zero while the incident is open.
//
// Posted names the message already posted into the thread whose root edit
// was rate limited, so the delivery retry does not post it again. It lives
// in memory only: after a restart the retry may post once more.
type conversationState struct {
	ThreadTS    string
	Root        *notification.Message
	Rollup      string
	ReopenUntil time.Time
	Posted      postedMark
}

// postedMark identifies one message of a conversation.
type postedMark struct {
	Revision int
	Status   notification.Status
}

func markOf(m notification.Message) postedMark {
	return postedMark{Revision: m.Revision, Status: m.Status}
}

// posted reports whether m is the message already posted into the thread
// while its root edit is still owed.
func (c conversationState) posted(m notification.Message) bool {
	return c.Posted != postedMark{} && c.Posted == markOf(m)
}

// expired reports a resolved conversation whose reopen window has passed.
func (c conversationState) expired(now time.Time) bool {
	return !c.ReopenUntil.IsZero() && now.After(c.ReopenUntil)
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
	text, threadTS string,
) (string, error) {
	opts := []slackClient.MsgOption{
		slackClient.MsgOptionBlocks(blocks.BlockSet...),
		slackClient.MsgOptionText(escapeMrkdwn(text), false),
		slackClient.MsgOptionAsUser(true),
	}
	if threadTS != "" {
		opts = append(opts, slackClient.MsgOptionTS(threadTS))
	}
	channelID, ts, err := s.apiClient.PostMessageContext(
		ctx, s.channel, opts...)
	if err == nil {
		s.rememberChannelID(channelID)
	}
	return ts, wrapSlackRateLimit(err)
}

// rememberChannelID keeps the channel ID Slack reported for a post.
// chat.update accepts only an ID, while the configured channel may be a
// name such as "#alerts"; one provider posts to one channel.
func (s *Slack) rememberChannelID(channelID string) {
	if channelID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.channelID = channelID
}

// updateChannel is the channel for chat.update: the ID from the last post
// response, or the configured channel before any post has been answered.
func (s *Slack) updateChannel() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.channelID != "" {
		return s.channelID
	}
	return s.channel
}

// SnapshotThreads implements delivery.ThreadStateProvider: incident key to
// the thread timestamp, followed by the stored root when there is one.
func (s *Slack) SnapshotThreads() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.conversations) == 0 {
		return nil
	}
	out := make(map[string]string, len(s.conversations))
	now := s.clockSource.Now()
	for key, state := range s.conversations {
		if !state.expired(now) {
			out[key] = encodeThread(state)
		}
	}
	return out
}

// RestoreThreads implements delivery.ThreadStateProvider. Keys are adopted
// in a stable order so the size bound applies deterministically, and a
// thread already posted by this run always wins over a saved one.
func (s *Slack) RestoreThreads(saved map[string]string) {
	keys := make([]string, 0, len(saved))
	for key := range saved {
		if decodeThread(saved[key]).ThreadTS != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		s.mu.Lock()
		_, live := s.conversations[key]
		s.mu.Unlock()
		if !live {
			s.saveConversation(key, decodeThread(saved[key]))
		}
	}
}
