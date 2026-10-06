package slack

import (
	"context"
	"strings"
	"time"

	slackClient "github.com/slack-go/slack"

	"github.com/abahmed/kwatch/internal/notification"
)

// postFunc posts blocks, into threadTS when it is set, and returns the
// timestamp of the posted message.
type postFunc func(
	ctx context.Context, blocks *slackClient.Blocks, threadTS string,
) (string, error)

// SendIncident posts one incident message. With a bot token, an incident
// is announced as one root message holding the full Note and its output;
// updates and the resolve are replies in its thread, keyed by Message.Key.
// When the status changes, the root is edited to the same announcement
// under the new status marker. A resolve that may reopen keeps the thread
// until its reopen window ends, so a "failing again" update replies in it
// and the root returns to the failing status. With a webhook, the Note is
// posted in full, and compact mode posts only the Short lead.
func (s *Slack) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	if s.compact {
		return s.SendMessage(ctx, m.ShortText())
	}
	if s.apiClient == nil && s.postBlocksFn == nil {
		return s.sendAPI(ctx, &slackClient.WebhookMessage{
			Blocks: noteBlocks(m), Text: fallbackText(m),
		})
	}
	if m.IsSummary() {
		return s.sendSummary(ctx, m)
	}
	lock := s.conversationLock(m.Key)
	lock.Lock()
	defer lock.Unlock()
	s.mu.Lock()
	state := s.conversations[m.Key]
	if state.expired(s.clockSource.Now()) {
		// The reopen window passed: the failure is a new conversation.
		s.removeLocked(m.Key)
		state = conversationState{}
	}
	s.mu.Unlock()
	post := s.poster(fallbackText(m))
	if state.ThreadTS == "" {
		return s.openConversation(ctx, post, m)
	}
	return s.continueConversation(ctx, post, state, m)
}

// openConversation posts the first message of a conversation as its root,
// with the full note. A resolve that opens its conversation also closes
// it: keeping the thread would persist a conversation nothing updates
// again. A failed post saves nothing, so the retry posts the root again.
func (s *Slack) openConversation(
	ctx context.Context, post postFunc, m notification.Message,
) error {
	ts, err := post(ctx, noteBlocks(m), "")
	if err != nil || m.Resolved() {
		return err
	}
	s.saveConversation(m.Key, conversationState{ThreadTS: ts, Root: &m})
	return nil
}

// continueConversation posts an update or resolve into the thread and
// then brings the root up to date. When the root edit is rate limited the
// delivery fails and is retried; the state remembers that this message is
// already in the thread, so the retry only repeats the edit instead of
// posting the update twice.
func (s *Slack) continueConversation(
	ctx context.Context, post postFunc, state conversationState,
	m notification.Message,
) error {
	threadTS := state.ThreadTS
	if !state.posted(m) {
		var err error
		threadTS, err = postWithThreadFallback(ctx, post,
			noteBlocks(m), state.ThreadTS)
		if err != nil {
			return err
		}
	}
	if threadTS != state.ThreadTS {
		// The thread was gone, so this message became a new root.
		state = conversationState{Root: &m}
	} else if state.Rollup == "" {
		root, err := s.editRoot(ctx, state, m)
		if err != nil {
			state.Posted = markOf(m)
			s.saveConversation(m.Key, state)
			return err
		}
		state.Root = root
	}
	// A roll-up member keeps the roll-up message as its root, which one
	// member never edits.
	state.ThreadTS = threadTS
	state.Posted = postedMark{}
	s.keepOrForget(m, state)
	return nil
}

// keepOrForget saves the conversation after a message. A resolve that
// may reopen keeps it until the reopen window ends, so the "failing
// again" update replies in the same thread and brings the root back to
// the failing status; any other resolve forgets it. A roll-up member is
// always forgotten: it never edits the roll-up's root.
func (s *Slack) keepOrForget(
	m notification.Message, state conversationState,
) {
	state.ReopenUntil = time.Time{}
	switch {
	case !m.Resolved():
	case m.ReopenWithin > 0 && state.Rollup == "":
		state.ReopenUntil = s.clockSource.Now().Add(m.ReopenWithin)
	default:
		s.deleteConversation(m.Key)
		return
	}
	s.saveConversation(m.Key, state)
}

// sendSummary posts a startup summary, roll-up or digest as one
// top-level message holding the whole note. Summaries are never updated,
// so a root line with the note in a thread would show the same text
// twice. The closing resolve of a summary is a short standalone line.
func (s *Slack) sendSummary(
	ctx context.Context, m notification.Message,
) error {
	if isRollup(m) {
		return s.sendRollup(ctx, m)
	}
	blocks := noteBlocks(m)
	if m.Resolved() {
		blocks = rootBlocks(m)
	}
	_, err := s.poster(fallbackText(m))(ctx, blocks, "")
	return err
}

// poster returns the bot post function; every post carries text as its
// plain-text fallback.
func (s *Slack) poster(text string) postFunc {
	if s.postBlocksFn != nil {
		return func(
			_ context.Context, blocks *slackClient.Blocks, threadTS string,
		) (string, error) {
			return s.postBlocksFn(blocks, threadTS)
		}
	}
	return func(
		ctx context.Context, blocks *slackClient.Blocks, threadTS string,
	) (string, error) {
		return s.postBlocks(ctx, blocks, text, threadTS)
	}
}

// fallbackText is the plain-text twin of an incident's blocks, shown in
// notification previews and read by screen readers. It is not escaped
// yet: every send path escapes text exactly once.
func fallbackText(m notification.Message) string {
	return notification.NeutralizeMentions(m.ShortText())
}

// outputLabel introduces the quoted output, so it never runs on from the
// last line of the note when the blocks are copied as plain text.
const outputLabel = "_Its recent output:_"

// noteBlocks is the full narrative, then the workload's last output as a
// labelled code block when there is one. Each section escapes its text once.
func noteBlocks(m notification.Message) *slackClient.Blocks {
	blocks := []slackClient.Block{textSection(m.NoteText())}
	if len(m.Output) > 0 {
		output := notification.NeutralizeMentions(
			strings.Join(m.Output, "\n"))
		blocks = append(blocks,
			markdownSection(outputLabel), codeSection(output))
	}
	return &slackClient.Blocks{BlockSet: capBlocks(blocks)}
}

// textSection is one mrkdwn section, mention-neutralized, escaped and cut
// to the section limit so Slack never rejects the whole message.
func textSection(text string) slackClient.SectionBlock {
	return markdownSection(notification.NeutralizeMentions(text))
}
