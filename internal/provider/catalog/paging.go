package catalog

import (
	"sort"
	"strings"
)

// pagingProviders are the paging services: they page a person and keep
// the alert open by key until a resolve closes it. Issue trackers,
// webhooks and AWS SNS also keep one alert per key, but they page
// nobody, so they are not listed. The Community provider ranking and
// the dashboard's "on-call" wording both read this one list.
var pagingProviders = map[string]bool{
	"alerta": true, "datadog": true, "goalert": true, "ilert": true,
	"incident.io": true, "incidentio": true, "newrelic": true,
	"opsgenie": true, "pagerduty": true, "sensugo": true, "signl4": true,
	"splunk": true, "splunkoncall": true, "squadcast": true,
	"zenduty": true,
}

// PagesPerson reports whether the provider key (any case) is a paging
// service that pages a person.
func PagesPerson(name string) bool {
	return pagingProviders[strings.ToLower(strings.TrimSpace(name))]
}

// PagingProviders returns the keys of the paging services, sorted. The
// slice is the caller's own.
func PagingProviders() []string {
	out := make([]string, 0, len(pagingProviders))
	for name := range pagingProviders {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
