package explain

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/detection"
)

// Endpoint classes: how a call to an endpoint failed. They are also the
// finer modes of a blamed endpoint ("EndpointFailing.Refused").
const (
	endpointRefused     detection.Mode = "Refused"
	endpointUnresolved  detection.Mode = "Unresolved"
	endpointTLS         detection.Mode = "TLS"
	endpointUnreachable detection.Mode = "Unreachable"
)

// endpointClasses classify a failed call, first match wins.
var endpointClasses = []struct {
	class   detection.Mode
	pattern *regexp.Regexp
}{
	{endpointTLS, regexp.MustCompile(`x509:|tls: |handshake failure`)},
	{endpointUnresolved, regexp.MustCompile(`no such host`)},
	{endpointRefused, regexp.MustCompile(`connection refused|` +
		`connection reset`)},
	{endpointUnreachable, regexp.MustCompile(`timed out|i/o timeout|` +
		`deadline exceeded|no route to host|network is unreachable`)},
}

// endpointPattern finds the endpoint a failed call was made to, in
// lower-case text. Group 1 is the host, group 2 the port when the
// pattern has one.
type endpointPattern struct {
	pattern *regexp.Regexp
	// bare marks a host:port with no words around it that say it is a
	// call. Stack frames read the same ("server.go:123"), so its host
	// must look like a domain name or an address.
	bare bool
}

// endpointPatterns are tried most precise first.
var endpointPatterns = []endpointPattern{
	// "dial tcp db.example.com:5432: connect: connection refused"
	{pattern: regexp.MustCompile(`dial (?:tcp|udp)[46]? ` +
		`(\[[0-9a-f:]+\]|[a-z0-9][a-z0-9.-]*):(\d{1,5})`)},
	// "lookup db.example.com on 10.96.0.10:53: no such host"
	{pattern: regexp.MustCompile(`lookup ([a-z0-9][a-z0-9.-]*[a-z0-9])()` +
		`(?: on \S+)?: no such host`)},
	// `Get "https://api.example.com/v1": x509: certificate …`
	{pattern: regexp.MustCompile(`[a-z][a-z0-9+]*://` +
		`([a-z0-9][a-z0-9.-]*[a-z0-9])(?::(\d{1,5}))?`)},
	// "could not connect to redis.example.com:6379"
	{pattern: regexp.MustCompile(
		`\b((?:[a-z0-9-]+\.)+[a-z0-9-]+):(\d{2,5})\b`), bare: true},
}

// sourceExtensions end source file names, which stack frames print
// before a line number.
var sourceExtensions = map[string]bool{
	"go": true, "java": true, "py": true, "js": true, "ts": true,
	"rb": true, "rs": true, "cs": true, "kt": true, "php": true,
	"c": true, "cc": true, "cpp": true, "h": true, "scala": true,
	"swift": true,
}

// ipv4 is a dotted IPv4 address.
var ipv4 = regexp.MustCompile(`^\d{1,3}(?:\.\d{1,3}){3}$`)

// hostLike reports whether host is an IPv4 address or a domain name
// whose last label is a top-level domain: letters only, at least two,
// and not a source file extension.
func hostLike(host string) bool {
	if ipv4.MatchString(host) {
		return true
	}
	tld := host[strings.LastIndex(host, ".")+1:]
	if len(tld) < 2 || sourceExtensions[tld] {
		return false
	}
	for _, r := range tld {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}

// endpointMatch returns the host and port of the first endpoint text
// names, trying the patterns in order.
func endpointMatch(text string) (host, port string, ok bool) {
	for _, p := range endpointPatterns {
		for _, m := range p.pattern.FindAllStringSubmatch(text, -1) {
			host := strings.TrimSuffix(m[1], ".")
			if !p.bare || hostLike(host) {
				return host, m[2], true
			}
		}
	}
	return "", "", false
}

// dnsServerFailure matches a lookup the DNS server could not answer.
// That is a cluster DNS problem, not one of the name looked up.
var dnsServerFailure = regexp.MustCompile(`lookup \S+ on \S+: ` +
	`(read udp|dial udp|i/o timeout|server misbehaving|` +
	`connection refused)`)

// endpointCall is the endpoint one failed call named and how it failed.
type endpointCall struct {
	endpoint string
	class    detection.Mode
}

// endpointIn finds the endpoint a connection, DNS or TLS error names:
// "db.example.com:5432", or a bare host when the error has no port.
// It reports false when the text shows no failed call, or when the
// failure is the cluster DNS server's rather than the endpoint's.
func endpointIn(text string) (endpointCall, bool) {
	text = strings.ToLower(text)
	class := endpointClass(text)
	if class == "" || dnsServerFailure.MatchString(text) {
		return endpointCall{}, false
	}
	host, port, ok := endpointMatch(text)
	// A call to port 53 is a DNS query: the cluster DNS rows own it.
	if !ok || port == "53" {
		return endpointCall{}, false
	}
	endpoint := host
	if port != "" {
		endpoint += ":" + port
	}
	return endpointCall{endpoint: endpoint, class: class}, true
}

func endpointClass(text string) detection.Mode {
	for _, c := range endpointClasses {
		if c.pattern.MatchString(text) {
			return c.class
		}
	}
	return ""
}

// endpointHost is the host part of an endpoint name.
func endpointHost(endpoint string) string {
	if strings.HasPrefix(endpoint, "[") {
		host, _, _ := strings.Cut(strings.TrimPrefix(endpoint, "["), "]")
		return host
	}
	host, _, _ := strings.Cut(endpoint, ":")
	return host
}

// localHosts never leave the pod.
var localHosts = map[string]bool{
	"localhost": true, "127.0.0.1": true, "::1": true, "0.0.0.0": true,
}

// clusterSuffixes end the names of in-cluster Services. A failing
// Service is blamed through its own findings, not by its name in text.
var clusterSuffixes = []string{".svc", ".svc.cluster.local",
	".cluster.local"}

// clusterLocal reports whether host is the pod itself or an in-cluster
// Service name.
func clusterLocal(host string) bool {
	if localHosts[host] {
		return true
	}
	for _, suffix := range clusterSuffixes {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

// signatureNoise turns the parts of an error that differ between
// replicas and runs into placeholders, so one error reads the same in
// every pod. Order matters: whole identifiers go before numbers.
var signatureNoise = []struct {
	pattern *regexp.Regexp
	with    string
}{
	{regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-` +
		`[0-9a-f]{4}-[0-9a-f]{12}`), "<id>"},
	{regexp.MustCompile(`\b\d{4}-\d\d-\d\d[t ][0-9:.]+z?\b`), "<time>"},
	{regexp.MustCompile(`\b\d{1,3}(?:\.\d{1,3}){3}(?::\d+)?\b`), "<ip>"},
	{regexp.MustCompile(`\b[0-9a-f]*\d[0-9a-z]*\b`), "#"},
	{regexp.MustCompile(`\s+`), " "},
}

// Signature limits: a signature must say something, and stays short
// enough to quote.
const (
	signatureMinLength = 16
	signatureMinWords  = 3
	signatureMaxLength = 80
)

// normalizeSignature returns the replica-independent form of an error
// message, or "" when what is left is too short to tell errors apart
// ("exit status 1").
func normalizeSignature(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	for _, noise := range signatureNoise {
		text = noise.pattern.ReplaceAllString(text, noise.with)
	}
	text = strings.TrimSpace(text)
	if len(text) < signatureMinLength ||
		len(strings.Fields(text)) < signatureMinWords ||
		onlyGeneric(text) {
		return ""
	}
	return cutRunes(text, signatureMaxLength)
}

// genericWords say how a call failed, never why. Every network failure
// shares them, so on their own they cannot tell one cause from another.
var genericWords = regexp.MustCompile(`context deadline exceeded|` +
	`connection reset by peer|connection refused|i/o timeout|` +
	`broken pipe|unexpected eof|<ip>|<id>|<time>|#|` +
	`\b(?:eof|read|write|dial|tcp|udp|error|err|fatal|panic)\b`)

// hasLetter matches a word that says something.
var hasLetter = regexp.MustCompile(`[a-z]`)

// onlyGeneric reports whether a normalised error says too little once
// its generic words are taken out.
func onlyGeneric(text string) bool {
	words := 0
	for _, word := range strings.Fields(
		genericWords.ReplaceAllString(text, " "),
	) {
		if hasLetter.MatchString(word) {
			words++
		}
	}
	return words < signatureMinWords
}

// cutRunes shortens text to at most n runes, never inside a rune.
func cutRunes(text string, n int) string {
	if utf8.RuneCountInString(text) <= n {
		return text
	}
	return strings.TrimSpace(string([]rune(text)[:n]))
}

// signatureName names a failure-signature entity: the failing mode,
// then the normalised error ("CrashLoop panic: license key rejected").
// The mode keeps different failures with the same words apart.
func signatureName(mode detection.Mode, signature string) string {
	return string(mode.Family()) + " " + signature
}

// SignatureText is the error a failure-signature entity stands for,
// without the mode signatureName puts first.
func SignatureText(name string) string {
	_, text, found := strings.Cut(name, " ")
	if !found {
		return name
	}
	return text
}
