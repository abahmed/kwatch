package config

// Warnings reports configuration that is legal but changes how well kwatch
// works, so it can be logged at startup instead of failing the load.
//
// Validate is for config that cannot work. This is for combinations that
// work and mislead: they do not stop kwatch from starting, and the only
// evidence that something is wrong shows up much later as notifications that
// are subtly untrue.
func Warnings(cfg *Config) []string {
	if cfg == nil {
		return nil
	}
	warnings := providerWarnings(cfg)
	warnings = append(warnings, removedOptionWarnings(cfg)...)
	warnings = append(warnings, unknownProviderOptionWarnings(cfg)...)
	warnings = append(warnings, routeWarnings(cfg)...)
	return append(warnings, unknownKeyWarnings(cfg)...)
}

// LintWarnings is Warnings plus the findings only `kwatch lint` reports.
// A running kwatch with no provider is a legitimate configuration (it still
// serves its health endpoints and metrics), but it is almost never what
// someone linting a file intended, so lint says so without failing.
func LintWarnings(cfg *Config) []string {
	warnings := Warnings(cfg)
	if cfg != nil && len(cfg.Alert) == 0 {
		warnings = append(warnings, "no alert providers configured")
	}
	return warnings
}

// providerWarnings flags providers whose upstream service no longer exists.
func providerWarnings(cfg *Config) []string {
	if _, ok := cfg.Alert["line"]; ok {
		return []string{"alert.line uses LINE Notify, which LINE shut down " +
			"on 2025-03-31; notifications to it will fail. Use another " +
			"provider such as webhook."}
	}
	return nil
}
