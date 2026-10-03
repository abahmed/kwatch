//go:build e2e

package scenarios

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
)

func TestScenarioActiveProbeFailureAndRecovery(t *testing.T) {
	onCluster(t, "integration.active-probe", func(s *Scenario) {
		lifecycleReceiverMode(s, "http-500")
		lifecycleCreateProbeConfig(s)
		s.ExpectClusterIncident("", "ActiveProbeFailure", 2*time.Minute)

		lifecycleReceiverMode(s, "success")
		s.ExpectResolved("", "ActiveProbeFailure")
	})
}

func TestScenarioHeartbeatDelivery(t *testing.T) {
	onCluster(t, "integration.heartbeat", func(s *Scenario) {
		s.Must(s.Env.Receiver.Clear(s.Ctx))
		lifecycleStartHeartbeatKwatch(s)
		lifecycleWaitPodRunning(s, "kwatch-heartbeat")

		s.Must(wait.PollUntilContextTimeout(s.Ctx,
			250*time.Millisecond, 2*time.Minute, true,
			func(ctx context.Context) (bool, error) {
				requests, _ := s.Env.Receiver.Requests(ctx)
				for _, request := range requests {
					if request.Method == "GET" && len(request.Body) == 0 {
						return true, nil
					}
				}
				return false, nil
			}))
	})
}
