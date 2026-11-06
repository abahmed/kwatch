// Package release keeps a verdict for each rollout of a workload. In: the
// inventory model's change journal (a pod-template change of a
// Deployment, StatefulSet or DaemonSet is a release), the workload's
// rollout status and the incidents kwatch holds. Out: a bounded book of
// release records, each with what changed, who changed it when that is
// known, how the rollout went and a verdict: rolling, watching, healthy,
// regressed, failed (the workload still serves), down, replaced or
// unknown. Each record also says how to undo it: a revert in Git for a
// workload a GitOps controller manages, a Helm rollback for a Helm
// release, a rollout undo otherwise.
//
// The package only reads. It sends nothing: a release that goes wrong
// is reported by the incident it causes, and a healthy one only shows on
// the dashboard's Releases page.
package release
