package slack

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/abahmed/kwatch/internal/message"
	"github.com/abahmed/kwatch/internal/model"

	slackClient "github.com/slack-go/slack"
)

type conversationState struct {
	ThreadTS       string
	RootDeliveryID string
	LastDeliveryID string
	LastDetailHash string
}

// SendNotification renders one semantic notification as a concise root and
// bounded thread details. DeliveryID makes a successful root retry-safe for
// the lifetime of this provider instance.
func (s *Slack) SendNotification(
	ctx context.Context,
	n *message.Notification,
) error {
	if n == nil || n.Action == model.ActionSkip {
		return nil
	}
	if s.compact || (s.apiClient == nil && s.postBlocksFn == nil) {
		return s.SendMessage(ctx, renderNotificationText(n))
	}
	key := n.ConversationKey
	conversationLock := s.conversationLock(key)
	conversationLock.Lock()
	defer conversationLock.Unlock()
	s.mu.Lock()
	state := s.conversations[key]
	s.mu.Unlock()
	post := s.postBlocks
	if s.postBlocksFn != nil {
		post = func(
			ctx context.Context,
			blocks *slackClient.Blocks,
			threadTS string,
		) (string, error) {
			return s.postBlocksFn(blocks, threadTS)
		}
	}
	if n.Action == model.ActionCreate {
		if state.RootDeliveryID != n.DeliveryID {
			ts, err := post(ctx, notificationRootBlocks(n), "")
			if err != nil {
				return err
			}
			state.ThreadTS = ts
			state.RootDeliveryID = n.DeliveryID
			s.saveConversation(key, state)
		}
		return s.postNotificationDetails(
			ctx, post, n, state.ThreadTS, state,
		)
	}
	if state.ThreadTS == "" {
		state.ThreadTS = s.loadThread(key)
	}
	detailsHash := notificationDetailsHash(n)
	if n.DeliveryID == state.LastDeliveryID &&
		detailsHash == state.LastDetailHash {
		return nil
	}
	if _, err := post(
		ctx, notificationDetailBlocks(n), state.ThreadTS,
	); err != nil {
		return err
	}
	state.LastDeliveryID = n.DeliveryID
	state.LastDetailHash = detailsHash
	if n.Action == model.ActionResolved {
		s.deleteConversation(key)
	} else {
		s.saveConversation(key, state)
	}
	return nil
}

func (s *Slack) postNotificationDetails(
	ctx context.Context,
	post func(context.Context, *slackClient.Blocks, string) (string, error),
	n *message.Notification,
	threadTS string,
	state conversationState,
) error {
	detailsHash := notificationDetailsHash(n)
	if len(n.Details) == 0 ||
		(state.LastDeliveryID == n.DeliveryID &&
			state.LastDetailHash == detailsHash) {
		return nil
	}
	if _, err := post(ctx, notificationDetailBlocks(n), threadTS); err != nil {
		return err
	}
	state.LastDeliveryID = n.DeliveryID
	state.LastDetailHash = detailsHash
	s.saveConversation(n.ConversationKey, state)
	return nil
}

func notificationDetailsHash(n *message.Notification) string {
	if n == nil || len(n.Details) == 0 {
		return ""
	}
	var text strings.Builder
	for _, section := range n.Details {
		text.WriteString(section.Title)
		text.WriteByte('\n')
		for _, line := range section.Lines {
			text.WriteString(line)
			text.WriteByte('\n')
		}
	}
	sum := sha256.Sum256([]byte(text.String()))
	return hex.EncodeToString(sum[:])
}

func notificationRootBlocks(n *message.Notification) *slackClient.Blocks {
	return &slackClient.Blocks{BlockSet: []slackClient.Block{
		markdownSection(notificationSummaryText(n)),
	}}
}

func notificationDetailBlocks(n *message.Notification) *slackClient.Blocks {
	blocks := []slackClient.Block{markdownSection(notificationSummaryText(n))}
	for _, section := range n.Details {
		for _, line := range section.Lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			blocks = append(blocks, markdownSection(
				"*"+section.Title+"*\n"+truncateField(line),
			))
		}
	}
	return &slackClient.Blocks{BlockSet: capBlocks(blocks)}
}

func notificationSummaryText(n *message.Notification) string {
	s := n.Summary
	text := s.Emoji + " *" + s.Title + "*"
	if s.Location.Namespace != "" {
		text += " — " + s.Location.Namespace + "/" + s.Location.Name
	} else if s.Location.Name != "" {
		text += " — " + s.Location.Name
	}
	location := []string{}
	if s.Location.Cluster != "" {
		location = append(location, s.Location.Cluster)
	}
	if s.Location.Resource != "" {
		location = append(location, s.Location.Resource)
	}
	if len(location) > 0 {
		text += "\n📍 " + strings.Join(location, " · ")
	}
	if s.Impact != "" {
		text += "\n🧩 " + s.Impact
	}
	if s.Cause != "" {
		text += "\n🔍 " + s.Cause
	}
	if s.Timing != "" {
		text += "\n⏱ " + s.Timing
	}
	if s.PrimaryAction != nil {
		text += "\n🛠 " + s.PrimaryAction.Label + ": `" +
			strings.Join(s.PrimaryAction.Args, " ") + "`"
	}
	if s.Severity != "" && s.Severity != "normal" {
		text += " · " + s.Severity
	}
	return truncateField(text)
}

func renderNotificationText(n *message.Notification) string {
	var lines []string
	lines = append(lines, notificationSummaryText(n))
	for _, detail := range n.Details {
		for _, line := range detail.Lines {
			lines = append(lines, detail.Title+": "+line)
		}
	}
	return strings.Join(lines, "\n")
}
