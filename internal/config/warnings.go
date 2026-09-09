package config

import "fmt"

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
	return resyncWarnings(cfg)
}

// resyncWarnings covers the relationship the resolve path depends on.
//
// The correlation engine closes an incident it has not heard about for a
// whole correlation.window. That is only sound because periodic informer
// resyncs re-deliver every object, the detectors re-run, and anything still
// broken re-reports itself inside the window -- silence therefore means
// recovery. Break that relationship and the same code starts closing
// incidents that are still happening, which reads in Slack as a problem that
// fixed itself. Neither field is wrong on its own, so neither can be
// validated on its own; the invariant lives between them.
func resyncWarnings(cfg *Config) []string {
	windowSeconds := int(cfg.Correlation.Window) * 60
	if windowSeconds <= 0 {
		return nil // validateCorrelation already rejects this
	}
	advised := windowSeconds / 2
	switch {
	case cfg.ResyncSeconds <= 0:
		return []string{fmt.Sprintf(
			"resyncSeconds is 0 (event-driven only), so nothing re-reports a "+
				"problem that is still happening: an incident whose object "+
				"stops emitting events is closed after correlation.window "+
				"(%dm) even though it never recovered. Set resyncSeconds to "+
				"at most %d to have every incident re-confirmed inside its "+
				"window.",
			int(cfg.Correlation.Window), advised)}
	case cfg.ResyncSeconds >= windowSeconds:
		return []string{fmt.Sprintf(
			"resyncSeconds (%d) is not shorter than correlation.window (%dm = "+
				"%ds), so an incident can go a full window without being "+
				"re-confirmed and be closed while it is still happening. Set "+
				"resyncSeconds to at most %d, or raise correlation.window.",
			cfg.ResyncSeconds, int(cfg.Correlation.Window), windowSeconds,
			advised)}
	}
	return nil
}
