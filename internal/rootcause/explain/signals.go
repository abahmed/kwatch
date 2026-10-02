package explain

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
)

// Signal names a class of error text. Rows use signals where only the
// words of an error connect the effect to the cause, such as a pod that
// crashes because a name does not resolve.
type Signal string

// Signals recognised in error text.
const (
	SignalDNS     Signal = "dns-error"
	SignalWebhook Signal = "webhook-error"
	SignalQuota   Signal = "quota-error"
	// SignalMetricsAPI is an autoscaler that cannot read the metrics
	// API: the APIService is failing even before its own status says.
	SignalMetricsAPI Signal = "metrics-api-error"
	// SignalForbidden is the API server refusing a request for lack
	// of permission (RBAC), not for a quota or an admission policy.
	SignalForbidden Signal = "forbidden-error"
	// SignalTLS is a TLS handshake refusing a certificate: expired,
	// not yet valid or signed by an unknown authority.
	SignalTLS Signal = "tls-error"
	// SignalConnection is a call over the network that failed:
	// refused, reset, timed out, unresolved or a failed handshake.
	SignalConnection Signal = "connection-error"
)

// signalPatterns match each signal in lower-case text.
var signalPatterns = map[Signal]*regexp.Regexp{
	SignalDNS: regexp.MustCompile(`no such host|temporary failure in ` +
		`name resolution|name or service not known|could not resolve ` +
		`host|server misbehaving|lookup .*: i/o timeout`),
	SignalWebhook: regexp.MustCompile(`webhook`),
	SignalTimeout: regexp.MustCompile(`deadline exceeded|timeout|` +
		`timed out`),
	SignalDiskFull: regexp.MustCompile(`no space left on device|enospc|` +
		`disk quota exceeded|file system is full|filesystem is full`),
	SignalQuota: regexp.MustCompile(`exceeded quota|failed quota`),
	SignalMetricsAPI: regexp.MustCompile(`unable to fetch metrics from|` +
		`unable to handle the request \(get [a-z.]*metrics\.k8s\.io`),
	SignalForbidden: regexp.MustCompile(`cannot (get|list|watch|create|` +
		`update|patch|delete|deletecollection) resource`),
	SignalTLS: regexp.MustCompile(`x509:|certificate has expired|` +
		`tls: (bad|expired|unknown) certificate|certificate is not ` +
		`yet valid`),
	SignalConnection: regexp.MustCompile(`connection refused|` +
		`connection reset|no route to host|network is unreachable|` +
		`i/o timeout|timed out|deadline exceeded|no such host|x509:|` +
		`tls: |handshake failure`),
}

// hasSignal reports whether text shows the signal. The empty signal is
// always present.
func hasSignal(signal Signal, text string) bool {
	if signal == "" {
		return true
	}
	pattern, ok := signalPatterns[signal]
	return ok && pattern.MatchString(strings.ToLower(text))
}

// Pull error classes: the registry-level class of a failed pull, which
// is also the pseudo mode of a registry candidate. pullImage is an error
// of one image (a typo, a missing tag) and never blames the registry.
const (
	pullImage     detection.Mode = ""
	pullAuth      detection.Mode = "Auth"
	pullRateLimit detection.Mode = "RateLimit"
	pullServer    detection.Mode = "Server"
	pullTLS       detection.Mode = "TLS"
	pullNetwork   detection.Mode = "Unreachable"
	// pullStatus is a registry that answered a pull with an error
	// status whose code was cut from the message. Image-level answers
	// say "not found", so what is left is the registry refusing.
	pullStatus detection.Mode = "Status"
)

// pullPatterns classify pull errors, first match wins. Image-level
// markers come first: Docker Hub answers "pull access denied,
// repository does not exist or may require authorization" for a typo.
var pullPatterns = []struct {
	class   detection.Mode
	pattern *regexp.Regexp
}{
	{pullImage, regexp.MustCompile(`not found|manifest unknown|` +
		`name unknown|does not exist|invalid reference`)},
	{pullRateLimit, regexp.MustCompile(`toomanyrequests|too many ` +
		`requests|rate limit|` + statusCode(`429`))},
	{pullAuth, regexp.MustCompile(`unauthorized|authentication ` +
		`required|no basic auth|failed to authorize|` +
		statusCode(`40[13]`) + `|forbidden`)},
	{pullServer, regexp.MustCompile(statusCode(`50[0234]`) +
		`|internal server error|bad gateway|service unavailable|` +
		`gateway timeout`)},
	{pullTLS, regexp.MustCompile(`x509|tls:|certificate`)},
	{pullNetwork, regexp.MustCompile(`timeout|timed out|connection ` +
		`refused|connection reset|no such host|no route to host|` +
		`network is unreachable|dial tcp`)},
	{pullStatus, regexp.MustCompile(`pulling from host \S+ failed ` +
		`with status code`)},
}

// statusCode matches an HTTP status among codes only where the error
// states one ("status code 503", "HTTP/1.1 429", ": 401"), so a
// number in an image tag ("app:1.503") is not read as an answer.
func statusCode(codes string) string {
	return `(?:status(?: code)?:? |http/[0-9.]+ |: )(?:` + codes + `)\b`
}

// classifyPull returns the registry-level class of a pull error.
func classifyPull(text string) detection.Mode {
	text = strings.ToLower(text)
	for _, p := range pullPatterns {
		if p.pattern.MatchString(text) {
			return p.class
		}
	}
	return pullImage
}
