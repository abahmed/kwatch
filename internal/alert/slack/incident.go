package slack

import (
	"context"
	"strings"

	slackClient "github.com/slack-go/slack"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

// postFunc posts blocks, into threadTS when it is set, and returns the
// timestamp of the posted message.
type postFunc func(
	ctx context.Context, blocks *slackClient.Blocks, threadTS string,
) (string, error)

// SendIncident posts one incident message. With a bot token, each incident
// is one root message (the Short lead) edited in place as the status
// changes, with every narrative (the Note) in its thread; the thread is
// keyed by Message.Key. With a webhook, the Note is posted in full, and
// compact mode posts only the Short lead.
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
	lock := s.conversationLock(m.Key)
	lock.Lock()
	defer lock.Unlock()
	s.mu.Lock()
	state := s.conversations[m.Key]
	s.mu.Unlock()
	post := s.poster(fallbackText(m))
	if state.ThreadTS == "" {
		ts, err := post(ctx, rootBlocks(m), "")
		if err != nil {
			return err
		}
		state.ThreadTS = ts
		_, err = post(ctx, noteBlocks(m), ts)
		// A resolve that opens its conversation also closes it: keeping
		// the thread would persist a conversation nothing updates again.
		// A failed note keeps the thread so the retry posts into it.
		if err == nil && m.Resolved() {
			s.deleteConversation(m.Key)
			return nil
		}
		s.saveConversation(m.Key, state)
		return err
	}
	threadTS, err := postWithThreadFallback(ctx, post,
		noteBlocks(m), state.ThreadTS)
	if err != nil {
		return err
	}
	if threadTS == state.ThreadTS {
		s.editRoot(ctx, post, state.ThreadTS, m)
	}
	state.ThreadTS = threadTS
	if m.Resolved() {
		s.deleteConversation(m.Key)
	} else {
		s.saveConversation(m.Key, state)
	}
	return nil
}

// editRoot rewrites the root message so the channel shows the current
// state without opening the thread. When the edit fails, the status line
// is posted into the thread instead so the change stays visible. Neither
// failure fails the delivery: the narrative is already in the thread.
func (s *Slack) editRoot(
	ctx context.Context, post postFunc, ts string, m notification.Message,
) {
	if s.apiClient == nil {
		return
	}
	_, _, _, err := s.apiClient.UpdateMessageContext(ctx,
		s.updateChannel(), ts,
		slackClient.MsgOptionBlocks(rootBlocks(m).BlockSet...),
		slackClient.MsgOptionText(escapeMrkdwn(fallbackText(m)), false))
	if err == nil {
		return
	}
	klog.InfoS("slack root edit failed; posting the status in the thread",
		"component", "delivery", "operation", "edit_root",
		"provider", s.Name(), "key", m.Key,
		"error", transport.RedactURLError(err))
	if _, err := post(ctx, rootBlocks(m), ts); err != nil {
		klog.InfoS("slack status post after a failed root edit failed",
			"component", "delivery", "operation", "edit_root_fallback",
			"provider", s.Name(), "key", m.Key,
			"error", transport.RedactURLError(err))
	}
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

// rootBlocks is the channel-level line: the Short lead with its marker.
func rootBlocks(m notification.Message) *slackClient.Blocks {
	return &slackClient.Blocks{BlockSet: []slackClient.Block{
		textSection(m.ShortText()),
	}}
}

// noteBlocks is the full narrative, then the workload's last output as a
// code block when there is one. Each section escapes its text once.
func noteBlocks(m notification.Message) *slackClient.Blocks {
	blocks := []slackClient.Block{textSection(m.NoteText())}
	if len(m.Output) > 0 {
		blocks = append(blocks, codeSection(
			notification.NeutralizeMentions(strings.Join(m.Output, "\n"))))
	}
	return &slackClient.Blocks{BlockSet: capBlocks(blocks)}
}

// textSection is one mrkdwn section, mention-neutralized, escaped and cut
// to the section limit so Slack never rejects the whole message.
func textSection(text string) slackClient.SectionBlock {
	return markdownSection(notification.NeutralizeMentions(text))
}
