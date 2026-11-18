package health

// ProveAccess records that the access a component needs was seen
// working: a read it makes succeeded, not only that nothing has
// refused it yet. Self alerts use it to tell a permission that was
// never granted from one that was lost.
func (h *HealthServer) ProveAccess(name string) {
	h.componentMu.Lock()
	defer h.componentMu.Unlock()
	if h.accessProven == nil {
		h.accessProven = make(map[string]bool)
	}
	h.accessProven[name] = true
}

// AccessProven reports whether ProveAccess was called for name.
func (h *HealthServer) AccessProven(name string) bool {
	h.componentMu.RLock()
	defer h.componentMu.RUnlock()
	return h.accessProven[name]
}
