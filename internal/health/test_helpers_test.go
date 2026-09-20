package health

import (
	"context"
)

func startForTest(server *HealthServer) error {
	if err := server.Open(); err != nil {
		return err
	}
	go func() {
		_ = server.Serve(context.Background())
	}()
	return nil
}
