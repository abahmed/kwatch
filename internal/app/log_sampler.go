package app

import "github.com/abahmed/kwatch/internal/inventory/kube"

// newLogSampler builds the round that samples the logs of Running and
// Ready pods for a burst of errors. It shares the pipeline's model, clock
// and observation queue and reads logs through the application-owned
// Kubernetes client.
func (p *pipelineBuild) newLogSampler(
	submit kube.Submit,
) *kube.LogSampleRound {
	return kube.NewLogSampleRound(kube.LogSampleConfig{
		Logs:   kube.LogReader{Client: p.deps.clients.Kubernetes},
		Model:  p.model,
		Now:    p.clock.Now,
		Submit: submit,
	})
}
