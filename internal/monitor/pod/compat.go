package pod

// SetSources adapts the retired setter-style wiring for compatibility tests
// and embedded callers. Production composition uses ConfigureSources.
func (r *Runtime) SetSources(sources RuntimeSources) {
	_ = r.ConfigureSources(sources)
}
