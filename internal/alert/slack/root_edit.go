package slack

import (
	"context"
	"strings"

	slackClient "github.com/slack-go/slack"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

// editRoot keeps the channel-level message showing the current state: it
// rewrites the root as the original announcement under the status marker
// of m, and returns the message the root now shows. The announcement is
// kept, never replaced by the text of the update.
//
// Without a known root (state saved before roots were persisted, when
// only a bare timestamp was stored) the root is left alone: the original
// text cannot be rebuilt, and overwriting it with the update's short line
// would destroy the announcement. Only the thread grows. When the status
// marker is unchanged there is nothing to edit.
//
// A rate-limited or otherwise transient failure is returned, so delivery
// retries instead of the failure being swallowed (and no second status
// message is posted into the thread). A permanent refusal, or a root that
// no longer exists, is logged: the update is already in the thread.
func (s *Slack) editRoot(
	ctx context.Context, state conversationState, m notification.Message,
) (*notification.Message, error) {
	if s.apiClient == nil || state.Root == nil {
		return state.Root, nil
	}
	root := withStatus(*state.Root, m)
	if root.Short == state.Root.Short && root.Note == state.Root.Note {
		return state.Root, nil
	}
	_, _, _, err := s.apiClient.UpdateMessageContext(ctx,
		s.updateChannel(), state.ThreadTS,
		slackClient.MsgOptionBlocks(noteBlocks(root).BlockSet...),
		slackClient.MsgOptionText(escapeMrkdwn(fallbackText(root)), false))
	if err == nil {
		return &root, nil
	}
	err = wrapSlackRateLimit(err)
	if !transport.IsPermanent(err) && !isStaleThreadError(err) {
		return state.Root, err
	}
	klog.InfoS("slack root edit refused; the thread update still posts",
		"component", "delivery", "operation", "edit_root",
		"provider", s.Name(), "key", m.Key,
		"error", transport.RedactURLError(err))
	return state.Root, nil
}

// withStatus is root with the status marker of current: the same note,
// steps and output, only the leading marker (and status) changed.
func withStatus(
	root, current notification.Message,
) notification.Message {
	marker := current.Marker
	if marker == "" {
		marker = current.Status.Emoji()
	}
	if marker == "" {
		return root
	}
	root.Marker = marker
	root.Status = current.Status
	root.Short = swapMarker(root.ShortText(), marker)
	return root.WithMarker(marker)
}

// swapMarker replaces the status marker text starts with. Text without a
// leading marker is returned unchanged.
func swapMarker(text, marker string) string {
	for _, old := range notification.Markers() {
		if strings.HasPrefix(text, old) {
			return marker + strings.TrimPrefix(text, old)
		}
	}
	return text
}

// rootBlocks is the short status line: the Short lead with its marker.
func rootBlocks(m notification.Message) *slackClient.Blocks {
	return &slackClient.Blocks{BlockSet: []slackClient.Block{
		textSection(m.ShortText()),
	}}
}
