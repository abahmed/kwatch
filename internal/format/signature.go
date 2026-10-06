package format

import (
	"regexp"
	"strconv"
	"strings"
)

// signatureRules replace what differs between occurrences of one error:
// timestamps, IDs, addresses and hashes. Order matters: a whole
// identifier is replaced before the digits inside it are. The text is
// lower-cased first, so the patterns are lower-case. Addresses with
// ports and plain numbers need more than a pattern; see replaceAddresses
// and replaceNumbers.
var signatureRules = []struct {
	pattern *regexp.Regexp
	with    string
}{
	{regexp.MustCompile(`\b\d{4}-\d\d-\d\d[t ]\d\d:\d\d:\d\d` +
		`(?:\.\d+)?(?:z|[+-]\d\d:?\d\d)?`), "<time>"},
	{regexp.MustCompile(`\b\d\d:\d\d:\d\d(?:\.\d+)?\b`), "<time>"},
	{regexp.MustCompile(`\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-` +
		`[0-9a-f]{4}-[0-9a-f]{12}\b`), "<id>"},
	{regexp.MustCompile(`\b0x[0-9a-f]+\b`), "<addr>"},
}

// ipv4 and ipv6 find addresses, each with an optional port. IPv6 is
// either bracketed ("[::1]:5432") or written out with colons.
var (
	ipv4 = regexp.MustCompile(`\b\d{1,3}(?:\.\d{1,3}){3}(?::(\d+))?\b`)
	ipv6 = regexp.MustCompile(`\[[0-9a-f:.]*:[0-9a-f:.]*\](?::(\d+))?|` +
		`\b(?:[0-9a-f]{1,4}:){7}[0-9a-f]{1,4}\b|` +
		`\b(?:[0-9a-f]{1,4}:){1,6}:[0-9a-f]{1,4}\b|\B::1\b`)
)

// podHash is a ReplicaSet pod name tail: name-<hash>-<suffix>. Kubernetes
// builds both parts from consonants and digits only, so ordinary
// hyphenated words ("x-forwarded-proto") never match.
var podHash = regexp.MustCompile(`-[bcdfghjklmnpqrstvwxz2456789]{8,10}-` +
	`[bcdfghjklmnpqrstvwxz2456789]{5}\b`)

var (
	hexID      = regexp.MustCompile(`\b[0-9a-f]{12,}\b`)
	numberWord = regexp.MustCompile(`\b[0-9a-z_]*\d[0-9a-z_]*\b`)
	// hexWord is a number or short hash: hex letters and digits only.
	hexWord = regexp.MustCompile(`^[0-9a-f]+$`)
	digits  = regexp.MustCompile(`\d+`)
	spaces  = regexp.MustCompile(`\s+`)
	// statusBefore ends text that introduces an HTTP status code.
	statusBefore = regexp.MustCompile(
		`(?:http|status)(?: code)?[ :=]*$|http/[0-9.]+ $`)
	statusCode = regexp.MustCompile(`^[1-5]\d\d$`)
	// hostBefore ends text that names a host a port follows: a dotted
	// name ("db.example.com:") or localhost.
	hostBefore = regexp.MustCompile(
		`(?:[a-z][a-z0-9-]*(?:\.[a-z0-9-]+)+|localhost):$`)
	allDigits = regexp.MustCompile(`^\d+$`)
)

// ephemeralPorts start here: a port at or above it is picked per
// connection, so it says nothing about which service was called.
const ephemeralPorts = 32768

// keptPort marks a port that replaceAddresses keeps in the signature.
const keptPort = "<ip>:"

// Signature is the replica-independent form of one error line: the
// same error from different pods, runs or requests gives the same
// text. It only replaces what differs (times, IDs, addresses, ports,
// numbers, pod name hashes); it never interprets the words.
func Signature(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	for _, rule := range signatureRules {
		text = rule.pattern.ReplaceAllString(text, rule.with)
	}
	text = replaceAddresses(text)
	text = podHash.ReplaceAllString(text, "-<pod>")
	text = hexID.ReplaceAllString(text, "<id>")
	text = replaceNumbers(text)
	return strings.TrimSpace(spaces.ReplaceAllString(text, " "))
}

// replaceAddresses turns every IP address into <ip>. A service port
// (below ephemeralPorts) stays, so a refusal from Postgres on :5432 is
// not the same error as one from Redis on :6379; the port of the
// client side, picked per connection, goes with the address.
func replaceAddresses(text string) string {
	keep := func(match string) string {
		// Only "ip:port" and "[ip]:port" end in ":digits" after a
		// closing bracket or a dotted address.
		i := strings.LastIndex(match, ":")
		isAddrPort := i >= 0 && (strings.Contains(match, "]:") ||
			strings.Count(match, ":") == 1)
		if !isAddrPort {
			return "<ip>"
		}
		port, err := strconv.Atoi(match[i+1:])
		if err != nil || port >= ephemeralPorts {
			return "<ip>"
		}
		return keptPort + match[i+1:]
	}
	return ipv6.ReplaceAllStringFunc(
		ipv4.ReplaceAllStringFunc(text, keep), keep)
}

// replaceNumbers turns numbers into #. A count, port or short hash
// ("31", "db2", "7f9c") becomes # whole. A name with other letters
// keeps them and only its digits go: worker7 becomes worker#, so
// replicas join while DB_PASSWORD_V2 and API_TOKEN_2 stay apart. It
// keeps a service port and an HTTP status code, because 503 and 404
// are different failures.
func replaceNumbers(text string) string {
	var out strings.Builder
	last := 0
	for _, at := range numberWord.FindAllStringIndex(text, -1) {
		word, before := text[at[0]:at[1]], text[:at[0]]
		out.WriteString(text[last:at[0]])
		last = at[1]
		switch {
		case keepNumber(word, before):
			out.WriteString(word)
		case hexWord.MatchString(word):
			out.WriteString("#")
		default:
			out.WriteString(digits.ReplaceAllString(word, "#"))
		}
	}
	out.WriteString(text[last:])
	return out.String()
}

// keepNumber reports whether a word with a digit in it must stay: the
// port after an address replaceAddresses kept, a service port after a
// named host (db.example.com:5432), or an HTTP status code.
func keepNumber(word, before string) bool {
	if strings.HasSuffix(before, keptPort) {
		return true
	}
	if hostBefore.MatchString(before) && allDigits.MatchString(word) {
		port, err := strconv.Atoi(word)
		return err == nil && port < ephemeralPorts
	}
	return statusCode.MatchString(word) && statusBefore.MatchString(before)
}

// genericSignatures are signatures too vague to say two failures are
// one. Why: every crash prints something like them, so grouping on
// them would join unrelated failures into one incident. They are bare
// level words, stack-trace headers, and the exit and kill notices the
// runtime writes for any crash.
var genericSignatures = map[string]bool{
	"": true, "error": true, "err": true, "fatal": true, "panic": true,
	"fatal error": true, "unknown": true, "failed": true,
	"killed": true, "signal: killed": true, "terminated": true,
	"completed": true, "oomkilled": true,
	"exit status #": true, "exit code #": true,
	"goroutine # [running]": true, "traceback (most recent call last)": true,
}

// IsGenericSignature reports whether a signature made by Signature is
// on the list of lines too vague to group failures by. A trailing
// colon is ignored, so "panic:" alone is as generic as "panic".
func IsGenericSignature(signature string) bool {
	signature = strings.TrimSpace(strings.TrimSuffix(signature, ":"))
	return genericSignatures[signature]
}
