// Package scorecard measures notification quality. In: audit entries, from
// a Kwatch audit log or from a replay, and labelled root-cause cases. Out:
// the noise figures used to judge changes (volume, the peak hour, messages
// per incident, updates without a visible change, re-created incidents,
// repeated recoveries, diagnosis quality), root-cause accuracy with its
// calibration per confidence level, and pass or fail gates against the
// production goals in docs/production-goals.md.
package scorecard
