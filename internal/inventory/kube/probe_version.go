package kube

import (
	"strings"

	"k8s.io/klog/v2"
)

// MinimumServerMinor is the oldest Kubernetes 1.x minor every typed
// watch is served on: autoscaling/v2 HorizontalPodAutoscalers are GA
// from 1.23, policy/v1 PodDisruptionBudgets, batch/v1 CronJobs and
// discovery.k8s.io/v1 EndpointSlices from 1.21. On an older server those
// kinds report api_unavailable and their checks do not run.
const MinimumServerMinor = 23

// BelowMinimumVersion reports whether a server version such as
// "v1.22.17-eks-1" is older than 1.MinimumServerMinor. A version it
// cannot read is not reported.
func BelowMinimumVersion(version string) bool {
	if !strings.HasPrefix(strings.TrimPrefix(version, "v"), "1.") {
		return false
	}
	minor, ok := ParseMinor(version)
	return ok && minor < MinimumServerMinor
}

// warnOldServer logs once per prober that the API server is older than
// the documented minimum. kwatch keeps running: only the kinds the
// server does not serve are left out.
func (p *Prober) warnOldServer(version string) {
	if p.versionWarned || !BelowMinimumVersion(version) {
		return
	}
	p.versionWarned = true
	klog.Warningf("API server %s is older than the supported minimum "+
		"v1.%d; HPA, PodDisruptionBudget, CronJob and EndpointSlice "+
		"checks may be unavailable", version, MinimumServerMinor)
}
