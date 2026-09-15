package workload

// SetSources adapts the retired setter-style wiring for compatibility tests
// and embedded callers. Production composition uses ConfigureSources.
func (s *SourceConfiguration) SetSources(sources Sources) {
	_ = s.ConfigureSources(sources)
}
