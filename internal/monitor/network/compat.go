package network

// SetSources adapts the retired setter-style wiring for compatibility tests
// and embedded callers. Production composition uses ConfigureSources.
func (r *Runtime) SetSources(sources Sources) {
	_ = r.ConfigureSources(sources)
}
