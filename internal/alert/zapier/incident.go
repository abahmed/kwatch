package zapier

import (
	"context"
	"encoding/json"

	"github.com/abahmed/kwatch/internal/alert/structured"
	"github.com/abahmed/kwatch/internal/notification"
)

// SendIncident posts the message as structured JSON.
func (z *Zapier) SendIncident(
	ctx context.Context, m notification.Message,
) error {
	body, err := json.Marshal(structured.NewIncident(z.clusterName, m))
	if err != nil {
		return err
	}
	return z.post(ctx, body)
}

// ReceivesPagingOnly says this receiver tracks incidents by key and must
// get a PagingOnly close for an incident it opened, even though it also
// takes plain messages. Delivery reads it next to SkipsPlainMessages.
func (z *Zapier) ReceivesPagingOnly() bool { return true }
