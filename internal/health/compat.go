package health

import "context"

// Start is retained for embedded callers that expect the historical
// convenience API. Application composition should call Open and let its
// supervisor invoke Serve so serving failures are observed centrally.
func (h *HealthServer) Start(ctx context.Context) error {
	if err := h.Open(); err != nil {
		return err
	}
	go func() {
		_ = h.Serve(ctx)
	}()
	return nil
}
