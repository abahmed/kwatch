package kube

// WithGenericAttributes wraps s as the watchers do, so objects it
// describes carry deletion, finalizers and generation. Tests that replay
// watched objects use it to see what a cluster's own watch records.
func WithGenericAttributes(s Schema, mode WatchMode) Schema {
	return withGenericAttributes(s, mode)
}
