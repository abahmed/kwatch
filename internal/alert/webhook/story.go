package webhook

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/abahmed/kwatch/internal/notice"
)

// storyPayload is the webhook schema for problem messages. It is one shape
// for every message; a receiver groups updates by Key and orders them by
// Revision.
type storyPayload struct {
	Cluster    string        `json:"cluster"`
	Key        string        `json:"key"`
	Revision   int           `json:"revision"`
	Status     string        `json:"status"`
	Title      string        `json:"title"`
	Lines      []string      `json:"lines,omitempty"`
	Timeline   []string      `json:"timeline,omitempty"`
	Output     []string      `json:"output,omitempty"`
	Steps      []notice.Step `json:"steps,omitempty"`
	Confidence string        `json:"confidence,omitempty"`
	Route      notice.Route  `json:"route"`
	Text       string        `json:"text"`
}

// SendStory implements api.StoryProvider.
func (w *Webhook) SendStory(ctx context.Context, m notice.Message) error {
	payload, err := json.Marshal(storyPayload{
		Cluster: w.clusterName, Key: m.Key, Revision: m.Revision,
		Status: m.Status.String(), Title: m.Title, Lines: m.Lines,
		Timeline: m.Timeline, Output: m.Output, Steps: m.Steps,
		Confidence: m.Confidence, Route: m.Route, Text: notice.Text(m),
	})
	if err != nil {
		return fmt.Errorf("marshal webhook story: %w", err)
	}
	_, err = w.sender.Send(ctx, w.request(payload))
	return err
}
