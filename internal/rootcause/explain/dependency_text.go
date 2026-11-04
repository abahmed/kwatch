package explain

import (
	"net"
	"regexp"
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// connectionFailure matches, in lower-case text, the words clients of
// any language use when a connection could not be made: the socket
// errors ("connection refused", "i/o timeout", "no such host",
// ECONNREFUSED), the client libraries' own sentences ("could not
// connect to", "It was not possible to connect", "Connection to db:5432
// refused", "Communications link failure") and their timeouts.
var connectionFailure = regexp.MustCompile(`connection refused|` +
	`connection reset|connection to \S+ refused|i/o timeout|timed out|` +
	`timeout expired|deadline exceeded|no such host|` +
	`name or service not known|unknown host|` +
	`temporary failure in name resolution|no route to host|` +
	`network is unreachable|econnrefused|etimedout|enotfound|` +
	`ehostunreach|(?:could not|couldn't|cannot|can't|unable to|` +
	`failed to|not possible to) (?:connect|reach|establish)|` +
	`communications link failure|server selection timeout|` +
	`no brokers? (?:are )?available|brokers? (?:is |are )?not available|` +
	`connection (?:attempt )?failed|could not be established|` +
	`broker may not be available|is the server running on host|` +
	`connection to server at "[^"\n]*"[^\n]*?, port \d+ failed|` +
	`error occurred while establishing a connection`)

// hostPort finds "host:port" in lower-case text. Group 1 is the host,
// group 2 the port. A time ("12:30:45") reads the same; hostName
// rejects it.
var hostPort = regexp.MustCompile(
	`(?:^|[\s"'(=/@,\[])([a-z0-9](?:[a-z0-9.-]*[a-z0-9])?):(\d{2,5})\b`)

// quotedHost finds a host the client quotes, with its port said
// apart: libpq's `host "db.prod" (10.0.0.5) and accepting TCP/IP
// connections on port 5432` and `connection to server at "db.prod"
// (10.0.0.5), port 5432 failed`. Group 1 is the host, group 2 the port.
var quotedHost = regexp.MustCompile(`(?:on host|server at) ` +
	`"([a-z0-9](?:[a-z0-9.-]*[a-z0-9])?)"[^"]{0,120}?\bport (\d{1,5})\b`)

// connectTo finds the host after the words that connect to one: "could
// not connect to cache", "connecting to db.prod".
var connectTo = regexp.MustCompile(`connect(?:ing|ion)? to ` +
	`([a-z0-9](?:[a-z0-9.-]*[a-z0-9])?)\b`)

// nearWords is how many words may stand between a dependency's name or
// host and the failure it is blamed for, in one clause. A client named
// further away ("timed out; kafka consumer started") is another
// sentence of the log, not the call that failed.
const nearWords = 6

// nearAny reports a word of which some occurrence is near a failure.
func nearAny(lower string, failures [][]int, word string) bool {
	for from := 0; ; {
		i := strings.Index(lower[from:], word)
		if i < 0 {
			return false
		}
		start := from + i
		if nearFailure(lower, failures, start, start+len(word),
			nearWords) {
			return true
		}
		from = start + 1
	}
}

// dependencyHost is the host and port a failed connection names, most
// precise first, near the failure. A host that reads like a source file
// ("main.go:42"), a stack frame or a time ("12:30:45") is not one.
func dependencyHost(lower string, failures [][]int) (host, port string) {
	if m := quotedHost.FindStringSubmatch(lower); m != nil &&
		hostName(m[1]) {
		return m[1], m[2]
	}
	if host, port, ok := endpointMatch(lower); ok && port != "53" &&
		nearAny(lower, failures, host) {
		return host, port
	}
	for _, m := range hostPort.FindAllStringSubmatchIndex(lower, -1) {
		host, port := lower[m[2]:m[3]], lower[m[4]:m[5]]
		if hostName(host) && port != "53" &&
			nearFailure(lower, failures, m[2], m[5], nearWords) {
			return host, port
		}
	}
	m := connectTo.FindStringSubmatchIndex(lower)
	if m != nil && strings.Contains(lower[m[2]:m[3]], ".") &&
		hostName(lower[m[2]:m[3]]) &&
		nearFailure(lower, failures, m[2], m[3], nearWords) {
		return lower[m[2]:m[3]], ""
	}
	return "", ""
}

// hostName reports a word that may name a host: an IP address, or a
// DNS-like name that starts with a letter, so never a time, a date or a
// number; and not a source file or a local address.
func hostName(word string) bool {
	if net.ParseIP(word) == nil &&
		(word == "" || word[0] < 'a' || word[0] > 'z') {
		return false
	}
	return notFile(word)
}

// notFile reports a word that may be a host: not a source file
// ("main.go") and not a local address.
func notFile(word string) bool {
	return !localHosts[word] &&
		!sourceExtensions[word[strings.LastIndex(word, ".")+1:]]
}

// sortedFindingIDs are the pods and containers with findings, in order.
func sortedFindingIDs(s Snapshot) []inventory.EntityID {
	ids := make([]inventory.EntityID, 0, len(s.Findings))
	for id := range s.Findings {
		if id.Kind == kube.KindPod || id.Kind == kube.KindContainer {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		return inventory.CompareIDs(ids[i], ids[j]) < 0
	})
	return ids
}

// ownedBy reports a pod, or a container's pod, whose top owner is root.
func ownedBy(s Snapshot, id, root inventory.EntityID) bool {
	pod := id
	if id.Kind == kube.KindContainer {
		pods := s.Model.Related(id, inventory.PartOf, inventory.Outgoing)
		if len(pods) == 0 {
			return false
		}
		pod = pods[0]
	}
	return rootcause.TopOwner(s.Model, pod) == root
}
