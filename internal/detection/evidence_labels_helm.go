package detection

// Evidence labels of a Helm release finding, read by the message and
// its steps.
const (
	// EvidenceHelmRelease is "<namespace>/<release>".
	EvidenceHelmRelease = "helm release"
	// EvidenceHelmRevision is the latest revision number.
	EvidenceHelmRevision = "helm revision"
	// EvidenceHelmStatus is its status label, as Helm wrote it.
	EvidenceHelmStatus = "helm status"
	// EvidenceHelmGoodRevision is the newest earlier revision that was
	// deployed: the one a rollback goes back to.
	EvidenceHelmGoodRevision = "helm deployed revision"
	// EvidenceHelmWorkload is one workload the release deploys, such as
	// "deployment/payments"; it repeats.
	EvidenceHelmWorkload = "helm workload"
)
