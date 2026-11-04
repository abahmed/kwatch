package explain

import (
	"math"
	"regexp"
	"strings"
)

// clientNames name the kind of dependency from client library and
// exception names, and from sentences only one client prints (libpq's
// "is the server running on host", Kafka's "broker may not be
// available"). They only name what the text is about; the failure
// itself is connectionFailure's.
var clientNames = []struct {
	name    string
	pattern *regexp.Regexp
}{
	{"Redis", regexp.MustCompile(`redis|jedis|lettuce`)},
	{"PostgreSQL", regexp.MustCompile(`postgres|psql|npgsql|psycopg|` +
		`\bpq: |\bpgx\b|is the server running (?:on host|locally)|` +
		`connection to server at "`)},
	{"MySQL", regexp.MustCompile(`mysql|mariadb`)},
	{"MongoDB", regexp.MustCompile(`mongo`)},
	{"Kafka", regexp.MustCompile(`kafka|broker may not be available|` +
		`connection to node -?\d+ \(`)},
	{"RabbitMQ", regexp.MustCompile(`rabbitmq|\bamqp`)},
	{"Elasticsearch", regexp.MustCompile(`elasticsearch|opensearch`)},
	{"Memcached", regexp.MustCompile(`memcache`)},
	{"Cassandra", regexp.MustCompile(`cassandra|nohostavailable`)},
	{"SQL Server", regexp.MustCompile(`sqlclient|sql server|mssql`)},
}

// portNames name a dependency by its well-known port when no client
// name is in the text: "dial tcp 10.0.0.12:5432" is PostgreSQL's port.
var portNames = map[string]string{
	"5432": "PostgreSQL", "3306": "MySQL", "6379": "Redis",
	"27017": "MongoDB", "9092": "Kafka", "5672": "RabbitMQ",
	"9200": "Elasticsearch", "11211": "Memcached", "9042": "Cassandra",
	"1433": "SQL Server",
}

// className is the exception or client class a name may stand in
// ("RedisConnectionException:", "jedis.JedisClient"): such a class
// names the failing call wherever the failure words are.
var className = regexp.MustCompile(`(?:exception|error|client)\W*$`)

// clientName names the dependency by its client, tied to the failure,
// else by its port. Of several tied clients, the one whose well-known
// port the call used wins ("mongo …, dial tcp 10.0.0.9:6379" is Redis),
// then the one nearest the failure words ("redis ok, postgres timed
// out" is PostgreSQL), then the first in clientNames.
func clientName(lower, port string, failures [][]int) string {
	best, bestGap := "", math.MaxInt
	for _, c := range clientNames {
		for _, m := range c.pattern.FindAllStringIndex(lower, -1) {
			if inPath(lower, m[0], m[1]) ||
				!clientTied(lower, failures, m[0], m[1]) {
				continue
			}
			if c.name == portNames[port] {
				return c.name
			}
			if gap := failureGap(lower, failures, m[0], m[1]); gap < bestGap ||
				best == "" {
				best, bestGap = c.name, gap
			}
		}
	}
	if best != "" {
		return best
	}
	return portNames[port]
}

// clientTied reports a client name at [start, end) that names the
// failing call: near the failure words, inside an exception or client
// class, or the library prefix of the error ("kafka: client has run
// out of available brokers: …") in the failure's clause.
func clientTied(lower string, failures [][]int, start, end int) bool {
	if nearFailure(lower, failures, start, end, nearWords) ||
		inClassName(lower, start, end) {
		return true
	}
	prefix := strings.HasPrefix(lower[end:], ": ") &&
		(start == 0 || strings.ContainsRune(" \t\n", rune(lower[start-1])))
	return prefix && nearFailure(lower, failures, start, end, -1)
}

// clauseBreak ends a clause: a ";", a new line, or a comma before a
// space ("redis ok, postgres timed out"). A comma inside a number or a
// time ("12:30:45,123") does not.
var clauseBreak = regexp.MustCompile(`[;\n]|,\s`)

// clauseGap is how many words stand between text at [start, end) and
// the failure phrase f, and false when a clause break is between them.
func clauseGap(lower string, f []int, start, end int) (int, bool) {
	var gap string
	switch {
	case end <= f[0]:
		gap = lower[end:f[0]]
	case start >= f[1]:
		gap = lower[f[1]:start]
	default:
		return 0, true
	}
	if clauseBreak.MatchString(gap) {
		return 0, false
	}
	return len(strings.Fields(gap)), true
}

// nearFailure reports text at [start, end) within limit words of a
// failure phrase (any number when limit is negative), in its clause.
func nearFailure(
	lower string, failures [][]int, start, end, limit int,
) bool {
	for _, f := range failures {
		if words, ok := clauseGap(lower, f, start, end); ok &&
			(limit < 0 || words <= limit) {
			return true
		}
	}
	return false
}

// failureGap is the fewest words between text at [start, end) and a
// failure phrase in its clause; math.MaxInt when none shares it (a
// client tied by its class name alone).
func failureGap(lower string, failures [][]int, start, end int) int {
	least := math.MaxInt
	for _, f := range failures {
		if words, ok := clauseGap(lower, f, start, end); ok &&
			words < least {
			least = words
		}
	}
	return least
}

// inClassName reports text at [start, end) inside the word of an
// exception or client class.
func inClassName(lower string, start, end int) bool {
	start, end = wordBounds(lower, start, end)
	return className.MatchString(lower[start:end])
}

// wordBounds widens [start, end) to the whitespace-separated word
// around it.
func wordBounds(lower string, start, end int) (int, int) {
	if i := strings.LastIndexAny(lower[:start], " \t\n"); i >= 0 {
		start = i + 1
	} else {
		start = 0
	}
	if i := strings.IndexAny(lower[end:], " \t\n"); i >= 0 {
		end += i
	} else {
		end = len(lower)
	}
	return start, end
}

// inPath reports a client name at [start, end) that is a segment of a
// path ("GET /users/redis"): a route or a file, not a client. A Unix
// socket ("/var/run/redis/redis.sock") is the client's own address.
func inPath(lower string, start, end int) bool {
	if start == 0 || lower[start-1] != '/' {
		return false
	}
	from, to := wordBounds(lower, start, end)
	return !strings.Contains(lower[from:to], ".sock")
}
