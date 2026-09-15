package controller

import workloadmonitor "github.com/abahmed/kwatch/internal/monitor/workload"

// configureDirectWorkloadRuntimes supplies each family runtime with its
// informer cache and controller clock.
func configureDirectWorkloadRuntimes(
	c *Controller,
	components RuntimeSet,
) error {
	if components.Workload.SourceConfig != nil {
		return components.Workload.SourceConfig.ConfigureSources(
			workloadmonitor.Sources{
				Deployments: c.deployLister, ReplicaSets: c.rsLister,
				DaemonSets: c.dsLister, StatefulSets: c.ssLister,
				Jobs: c.jobLister, CronJobs: c.cronJobLister,
				HPAs: c.hpaLister, PDBs: c.pdbLister,
			},
		)
	}
	return nil
}
