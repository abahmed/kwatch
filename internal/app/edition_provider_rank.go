package app

// The Community edition runs one alert provider. When the operator did not
// name it (app.communityProvider), the one kept is chosen by what an
// incident needs from it, not by the alphabet: a pager first, because it
// pages a person and closes its alert by key on the resolve; then chat and
// everything else; email last, because a mail cannot be closed and is read
// late. Inside a class the name decides, so the choice never depends on
// map order.
const (
	rankPager = iota
	rankChat
	rankEmail
)

// pagerProviders are the paging services: they page a person and keep the
// alert open by key until a resolve closes it.
var pagerProviders = map[string]bool{
	"alerta": true, "datadog": true, "goalert": true, "ilert": true,
	"incident.io": true, "incidentio": true, "newrelic": true,
	"opsgenie": true, "pagerduty": true, "sensugo": true, "signl4": true,
	"splunk": true, "splunkoncall": true, "squadcast": true,
	"zenduty": true,
}

// emailProviders send mail.
var emailProviders = map[string]bool{
	"email": true, "mailgun": true, "resend": true, "sendgrid": true,
	"ses": true,
}

// providerRank orders the providers Community may keep; lower is kept
// first.
func providerRank(name string) int {
	switch {
	case pagerProviders[name]:
		return rankPager
	case emailProviders[name]:
		return rankEmail
	}
	return rankChat
}

// isPagerProvider reports a provider that pages and holds alerts by key.
func isPagerProvider(name string) bool {
	return pagerProviders[name]
}
