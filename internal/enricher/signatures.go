package enricher

import "regexp"

var signatureHints = []struct {
	re   *regexp.Regexp
	hint string
}{

	// --- existing patterns (preserved exactly) ---

	{regexp.MustCompile(`(?i)connection refused.*:5432|dial tcp.*:5432`), "Postgres unreachable — check the DB Service/endpoints + connection string."},
	{regexp.MustCompile(`(?i)x509|certificate (has expired|signed by unknown|verify failed)`), "TLS/cert issue — check the CA bundle and cert validity."},
	{regexp.MustCompile(`(?i)no space left on device`), "Disk full — check PVC / ephemeral-storage usage."},
	{regexp.MustCompile(`(?i)i/o timeout|context deadline exceeded|dial tcp .* timeout`), "Network/dependency timeout — check the target Service / NetworkPolicy."},
	{regexp.MustCompile(`(?i)permission denied|forbidden|unauthorized|RBAC`), "AuthZ/RBAC or filesystem-permission issue."},
	{regexp.MustCompile(`(?i)out of memory|cannot allocate memory`), "Memory exhaustion — raise limits or fix the leak."},

	// --- language runtimes ---

	{regexp.MustCompile(`java\.lang\.OutOfMemoryError`), "Java OOM — increase heap (-Xmx) or fix memory leak"},
	{regexp.MustCompile(`java\.lang\.StackOverflowError`), "Java stack overflow — check for infinite recursion or deep call chains"},
	{regexp.MustCompile(`java\.lang\.NullPointerException`), "Java null pointer — check for uninitialized objects and add null checks"},
	{regexp.MustCompile(`Traceback \(most recent call last\):`), "Python unhandled exception — check traceback for root cause"},
	{regexp.MustCompile(`(?i)ModuleNotFoundError|ImportError: No module named|ImportError: cannot import name`), "Python missing module — check requirements.txt and deployment"},
	{regexp.MustCompile(`(?i)uncaughtException|UnhandledPromiseRejection`), "Node.js unhandled exception — add error handlers or fix the rejection"},
	{regexp.MustCompile(`invalid memory address or nil pointer dereference`), "Go nil pointer dereference — check for missing error handling"},
	{regexp.MustCompile(`panic: runtime error:`), "Go runtime panic — check goroutine stacks for root cause"},
	{regexp.MustCompile(`(?i)segmentation fault|SIGSEGV|signal 11`), "Segmentation fault (SIGSEGV) — check for null pointer, buffer overflow, or stack corruption"},

	// --- infrastructure dependencies ---

	{regexp.MustCompile(`(?i)dial tcp.*:6379|connection refused.*:6379|READONLY You can't write against`), "Redis unreachable — check the Redis Service, NetworkPolicy, and connection string"},
	{regexp.MustCompile(`(?i)dial tcp.*:3306|connection refused.*:3306|Access denied for user.*@`), "MySQL unreachable — check the MySQL Service, credentials, and connectivity"},
	{regexp.MustCompile(`(?i)dial tcp.*:27017|connection refused.*:27017|MongoNetworkError|MongoError.*not primary`), "MongoDB unreachable — check the MongoDB Service, replica set, and URI"},
	{regexp.MustCompile(`(?i)no alive nodes|:9200.*connection refused|connection refused.*:9200|master_not_discovered`), "Elasticsearch unreachable — check cluster health and connectivity"},
	{regexp.MustCompile(`(?i)dial tcp.*:5672|connection refused.*:5672|amqp:.*closed`), "RabbitMQ unreachable — check the RabbitMQ Service and connection URI"},
	{regexp.MustCompile(`(?i)upstream timed out|no live upstreams|connect\(\) failed.*upstream`), "Upstream/ingress backend failure — check target Service health and connectivity"},

	// --- common app failures ---

	{regexp.MustCompile(`(?i)address already in use|EADDRINUSE`), "Port conflict — check port assignments and competing processes"},
	{regexp.MustCompile(`(?i)No such file or directory|ENOENT`), "File or resource missing — check volume mounts, file paths, and ConfigMap/Secret mounts"},
	{regexp.MustCompile(`(?i)stack overflow|stack exhausted`), "Stack overflow — check for infinite recursion or deep call chains"},
	{regexp.MustCompile(`(?i)invalid configuration|failed to load config|error parsing|parse error`), "Configuration error — check ConfigMap, env vars, and config file syntax"},
	{regexp.MustCompile(`(?i)rate limit exceeded|too many requests|429 Too Many Requests`), "Rate limited by upstream — reduce request rate or increase quota"},
	{regexp.MustCompile(`(?i)connection reset by peer|broken pipe|connection closed by remote`), "Connection reset by peer — check upstream stability and timeout settings"},
	{regexp.MustCompile(`(?i)lookup.*: no such host|could not resolve host|Name or service not known|Temporary failure in name resolution|nodename nor servname provided`), "DNS resolution failure — check CoreDNS / DNS Service and network policy"},
	{regexp.MustCompile(`(?i)connection pool exhausted|connection pool.*(full|empty|max)|no available connections`), "Connection pool exhausted — increase pool size or check for connection leaks"},
	{regexp.MustCompile(`(?i)checksum mismatch|hash verification failed|sha256 mismatch`), "File integrity failure — check for corrupted downloads or storage issues"},
}

func SignatureHint(logs string) string {
	for _, s := range signatureHints {
		if s.re.MatchString(logs) {
			return s.hint
		}
	}
	return ""
}
