// Command kwatch-scorecard replays a Kwatch audit log and reports
// notification quality figures, optionally failing when a threshold is
// exceeded.
//
// The limits with a production goal (docs/production-goals.md) apply by
// default: peak hour, messages per incident (p95 and most), unchanged
// updates, re-created incidents and repeated recoveries. Goals that need
// a label are not checked here, because an audit log does not carry
// one: root-cause accuracy and calibration need the expected root, and
// notifications caused by non-events (healthy rollouts, scaling, drains)
// need to know which changes were healthy. The scenario scorecard
// (make alert-quality) measures those.
package main
