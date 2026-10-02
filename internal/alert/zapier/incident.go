package zapier

import (
	"context"
	"encoding/json"

	"github.com/abahmed/kwatch/internal/notification"
)

// incidentPayload is the structured incident schema shared with the
// generic webhook provider. Field meanings are documented in
// docs/providers.md.
type incidentPayload struct {
	Cluster    string              `json:"cluster"`
	Key        string              `json:"key"`
	AlertKey   string              `json:"alertKey"`
	Revision   int                 `json:"revision"`
	Status     string              `json:"status"`
	Resolved   bool                `json:"resolved"`
	Marker     string              `json:"marker"`
	Short      string              `json:"short"`
	Note       string              `json:"note"`
	Title      string              `json:"title"`
	Lines      []string            `json:"lines,omitempty"`
	Timeline   []string            `json:"timeline,omitempty"`
	Output     []string            `json:"output,omitempty"`
	Steps      []notification.Step `json:"steps,omitempty"`
	Confidence string              `json:"confidence,omitempty"`
	Route      notification.Route  `json:"route"`
}

func newIncidentPayload(
	cluster string, m notification.Message,
) incidentPayload {
	return incidentPayload{
		Cluster: cluster, Key: m.Key, AlertKey: m.AlertKey(cluster),
		Revision: m.Revision, Status: m.Status.String(),
		Resolved: m.Resolved(), Marker: m.Marker,
		Short: m.ShortText(), Note: m.NoteText(), Title: m.Title,
		Lines: m.Lines, Timeline: m.Timeline, Output: m.Output,
		Steps: m.Steps, Confidence: m.Confidence, Route: m.Route,
	}
}

// SendIncident posts the message as structured JSON.
func (z *Zapier) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	body, err := json.Marshal(newIncidentPayload(z.clusterName, m))
	if err != nil {
		return err
	}
	return z.post(ctx, body)
}
