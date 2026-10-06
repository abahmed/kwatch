package webhook

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/abahmed/kwatch/internal/alert/structured"
	"github.com/abahmed/kwatch/internal/notification"
)

// SendIncident posts the message as structured JSON.
func (w *Webhook) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	payload, err := json.Marshal(structured.NewIncident(w.clusterName, m))
	if err != nil {
		return fmt.Errorf("marshal webhook incident: %w", err)
	}
	_, err = w.sender.Send(ctx, w.request(payload))
	return err
}

// ReceivesPagingOnly says this receiver tracks incidents by key and must
// get a PagingOnly close for an incident it opened, even though it also
// takes plain messages. Delivery reads it next to SkipsPlainMessages.
func (w *Webhook) ReceivesPagingOnly() bool { return true }
