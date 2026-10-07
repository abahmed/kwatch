package rootcause

// Proof codes of the config version check: pods of one workload read
// different content of a ConfigMap or Secret that changed, because
// environment variables and subPath mounts are read only when a
// container starts (explain's config-version scorer). Count is how many
// pods fail and Total how many pods on the other version run healthy.
const (
	// ProofNewConfigFails: the pods started since the change fail and
	// the older ones, still on the old content, are healthy.
	ProofNewConfigFails ProofCode = "new-config-fails"
	// ProofOldConfigFails: the pods still on the old content fail and
	// the ones started since the change are healthy.
	ProofOldConfigFails ProofCode = "old-config-fails"
)
