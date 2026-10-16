package metrics

// IncStorageReset records one state file reset using a bounded reason
// label; an unknown reason counts as unreadable.
func (r *Registry) IncStorageReset(reason string) {
	for i, allowed := range storageResetReasons {
		if reason == allowed {
			r.StorageResets[i].Add(1)
			return
		}
	}
	r.StorageResets[1].Add(1)
}
