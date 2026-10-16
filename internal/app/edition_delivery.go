package app

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/feature"
)

// providerRouteKeys are the provider options that route and pace alerts:
// per-scope routes, the fallback chain and the hourly budget.
var providerRouteKeys = []string{"routes", "fallback", "hourlyBudget"}

// deliveryPro lists the Pro delivery capabilities cfg sets: more
// providers than one, routing, native threads and templates.
func deliveryPro(cfg *config.Config) []feature.ID {
	var ids []feature.ID
	if len(activeProviders(cfg)) > 1 {
		ids = append(ids, extraProviders)
	}
	if setsAny(cfg.Alert, providerRouteKeys...) {
		ids = append(ids, feature.ProviderRouting)
	}
	if slackThreads(cfg.Alert) {
		ids = append(ids, feature.NativeThreads)
	}
	if len(cfg.Templates) > 0 || setsAny(cfg.Alert, "templates") {
		ids = append(ids, feature.CustomTemplates)
	}
	return ids
}

// slackThreads reports a Slack bot token: the only provider setup that
// threads a chat conversation. A webhook posts every message on its own.
func slackThreads(alert map[string]map[string]interface{}) bool {
	token, _ := alert["slack"]["token"].(string)
	return token != ""
}

func setsAny(
	alert map[string]map[string]interface{}, keys ...string,
) bool {
	for _, settings := range alert {
		for _, key := range keys {
			if isSet(settings[key]) {
				return true
			}
		}
	}
	return false
}

// isSet is false for a missing or empty option.
func isSet(value interface{}) bool {
	switch v := value.(type) {
	case nil:
		return false
	case string:
		return v != ""
	case int:
		return v != 0
	case []interface{}:
		return len(v) > 0
	case map[string]interface{}:
		return len(v) > 0
	}
	return true
}

// capProviders splits the configured providers into those the edition
// runs and those beyond its cap. The provider named by
// app.communityProvider is kept first when it is configured; the rest are
// kept by sorted name, so the choice does not depend on map order. A name
// that is not configured is ignored here (config warns about it).
func capProviders(
	e feature.Entitlements, cfg *config.Config,
) (kept, dropped []string) {
	names := activeProviders(cfg)
	limit := feature.OrCommunity(e).Limits().MaxProviders
	if limit <= 0 || len(names) <= limit {
		return names, nil
	}
	if preferred := cfg.App.CommunityProvider; slices.Contains(
		names, preferred) {
		names = append([]string{preferred}, slices.DeleteFunc(
			slices.Clone(names), func(n string) bool {
				return n == preferred
			})...)
	}
	return names[:limit], names[limit:]
}

// communitySends words the provider the capped edition sends to and the
// ones it leaves inactive, for /status, the startup log and lint:
// "slack (inactive: pagerduty)". It is empty when nothing is capped.
func communitySends(e feature.Entitlements, cfg *config.Config) string {
	kept, dropped := capProviders(e, cfg)
	if len(dropped) == 0 {
		return ""
	}
	sort.Strings(dropped)
	return fmt.Sprintf("%s (inactive: %s)", strings.Join(kept, ", "),
		strings.Join(dropped, ", "))
}

// activeProviders is the sorted names of the providers delivery would
// build; a removed provider's section is ignored and does not count.
func activeProviders(cfg *config.Config) []string {
	var names []string
	for _, p := range config.CompileRuntimeConfig(cfg).Delivery().Providers() {
		names = append(names, p.Name)
	}
	sort.Strings(names)
	return names
}

func providerCapNotice(e feature.Entitlements, dropped []string) string {
	return fmt.Sprintf("alert providers beyond %d (%d inactive: %s)",
		feature.OrCommunity(e).Limits().MaxProviders, len(dropped),
		strings.Join(dropped, ", "))
}

// restrictDelivery returns the alert settings and templates e runs. It
// drops the providers beyond the cap and, without delivery.routing, the
// routes, fallback and hourly budget of the rest, so every alert goes to
// the one provider. Without delivery.templates it drops operator
// templates. The inputs are not changed. Native threads are switched off
// in delivery itself.
func restrictDelivery(
	cfg *config.Config, e feature.Entitlements,
) (map[string]map[string]interface{}, map[string]string) {
	e = feature.OrCommunity(e)
	kept, _ := capProviders(e, cfg)
	alert := make(map[string]map[string]interface{}, len(kept))
	for _, name := range kept {
		settings := make(map[string]interface{}, len(cfg.Alert[name]))
		for key, value := range cfg.Alert[name] {
			settings[key] = value
		}
		if !e.Has(feature.ProviderRouting) {
			for _, key := range providerRouteKeys {
				delete(settings, key)
			}
		}
		if !e.Has(feature.CustomTemplates) {
			delete(settings, "templates")
		}
		alert[name] = settings
	}
	if cfg.Alert == nil {
		alert = nil
	}
	templates := cfg.Templates
	if !e.Has(feature.CustomTemplates) {
		templates = nil
	}
	return alert, templates
}
