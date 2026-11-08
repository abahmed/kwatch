package app

import (
	"context"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

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

// logRunners are the rounds that read container logs: the crash-log
// round while investigation reads logs, and the sample of healthy pods
// unless investigation.sampleLogs turns it off.
func (p *pipelineBuild) logRunners(
	submit kube.Submit,
) []func(context.Context) {
	var out []func(context.Context)
	if crashLogsEnabled(p.deps) {
		logs := kube.LogReader{Client: p.deps.clients.Kubernetes}
		p.crashLogs = kube.NewCrashLogRound(kube.CrashLogConfig{
			Logs:    logs,
			Current: logs,
			Model:   p.model,
			Now:     p.clock.Now,
			Submit:  submit,
		})
		out = append(out, p.crashLogs.Run)
	}
	if logSamplingEnabled(p.deps) {
		out = append(out, p.newLogSampler(submit).Run)
	}
	return out
}
