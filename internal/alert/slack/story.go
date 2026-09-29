package slack

import (
	"context"
	"strings"

	slackClient "github.com/slack-go/slack"

	"github.com/abahmed/kwatch/internal/message"
	"github.com/abahmed/kwatch/internal/notice"
)

// SendStory implements api.StoryProvider. With a bot token, each problem
// is one root message edited in place as its status changes, with every
// update in its thread. With a webhook, each message is posted in full.
func (s *Slack) SendStory(ctx context.Context, m notice.Message) error {
	if s.compact || (s.apiClient == nil && s.postBlocksFn == nil) {
		return s.SendMessage(ctx, notice.Text(m))
	}
	lock := s.conversationLock(m.Key)
	lock.Lock()
	defer lock.Unlock()
	s.mu.Lock()
	state := s.conversations[m.Key]
	s.mu.Unlock()
	post := s.poster()
	if state.ThreadTS == "" {
		ts, err := post(ctx, storyRootBlocks(m), "")
		if err != nil {
			return err
		}
		state.ThreadTS = ts
		_, err = post(ctx, storyDetailBlocks(m), ts)
		s.saveConversation(m.Key, state)
		return err
	}
	threadTS, err := postWithThreadFallback(ctx, post,
		storyDetailBlocks(m), state.ThreadTS)
	if err != nil {
		return err
	}
	if threadTS == state.ThreadTS {
		s.editRoot(ctx, state.ThreadTS, m)
	}
	state.ThreadTS = threadTS
	if m.Status == notice.StatusResolved {
		s.deleteConversation(m.Key)
	} else {
		s.saveConversation(m.Key, state)
	}
	return nil
}

// editRoot rewrites the root message so the channel shows the current
// state without opening the thread. Failing to edit is not fatal: the
// update is already in the thread.
func (s *Slack) editRoot(ctx context.Context, ts string, m notice.Message) {
	if s.apiClient == nil {
		return
	}
	_, _, _, _ = s.apiClient.UpdateMessageContext(ctx, s.channel, ts,
		slackClient.MsgOptionBlocks(storyRootBlocks(m).BlockSet...))
}

func (s *Slack) poster() func(
	context.Context, *slackClient.Blocks, string,
) (string, error) {
	if s.postBlocksFn != nil {
		return func(
			_ context.Context, blocks *slackClient.Blocks, threadTS string,
		) (string, error) {
			return s.postBlocksFn(blocks, threadTS)
		}
	}
	return s.postBlocks
}

// storyRootBlocks is the channel-level summary: status and title, then the
// first story line.
func storyRootBlocks(m notice.Message) *slackClient.Blocks {
	text := strings.TrimSpace(m.Status.Emoji() + " *" +
		escapeMrkdwn(m.Title) + "*")
	if len(m.Lines) > 0 {
		text += "\n" + escapeMrkdwn(m.Lines[0])
	}
	return &slackClient.Blocks{BlockSet: []slackClient.Block{
		markdownSection(truncateField(message.NeutralizeMentions(text))),
	}}
}

// storyDetailBlocks carries the rest of the story into the thread.
func storyDetailBlocks(m notice.Message) *slackClient.Blocks {
	var blocks []slackClient.Block
	add := func(text string) {
		if strings.TrimSpace(text) != "" {
			blocks = append(blocks, markdownSection(truncateField(
				message.NeutralizeMentions(text))))
		}
	}
	lines := m.Lines
	if len(lines) > 0 {
		lines = lines[1:]
	}
	add(escapeMrkdwn(strings.Join(lines, "\n")))
	if m.Confidence != "" {
		add("_Confidence: " + m.Confidence + "_")
	}
	if len(m.Output) > 0 {
		add("*Last output*\n```" + strings.Join(m.Output, "\n") + "```")
	}
	if len(m.Timeline) > 0 {
		add("*Timeline (UTC)*\n" + escapeMrkdwn(strings.Join(m.Timeline, "\n")))
	}
	var steps []string
	for _, step := range m.Steps {
		line := "• " + escapeMrkdwn(step.Text)
		if step.Command != "" {
			line += "\n`" + step.Command + "`"
		}
		steps = append(steps, line)
	}
	add(strings.Join(steps, "\n"))
	if len(blocks) == 0 {
		add(m.Status.Emoji() + " " + escapeMrkdwn(m.Title))
	}
	return &slackClient.Blocks{BlockSet: capBlocks(blocks)}
}
