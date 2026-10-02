package kube

// Generic attributes every watched object carries, whatever its kind.
// Detection derives health from them for kinds it has no specialised
// detector for (generation lag, stuck deletion, failing phase or
// condition). Kind-specific attributes live in attributes.go.
const (
	// AttrDeletingSince is when deletion was requested
	// (metadata.deletionTimestamp).
	AttrDeletingSince = "deleting.since"
	// AttrFinalizers lists the finalizers still blocking deletion,
	// comma-separated.
	AttrFinalizers = "finalizers"
	// AttrWatchMode records how the object is watched: full, status,
	// metadata or hashed.
	AttrWatchMode = "watch.mode"
)
