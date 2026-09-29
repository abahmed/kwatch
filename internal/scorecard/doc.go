// Package scorecard measures notification quality from a Kwatch audit log.
// It replays audit entries offline and reports the noise KPIs used to judge
// changes: volume, messages per problem, updates without a visible change,
// flapping, repeated recoveries, and diagnosis quality.
package scorecard
