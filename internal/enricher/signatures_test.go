package enricher

import "testing"

func TestSignatureHint(t *testing.T) {
	tests := []struct {
		name string
		logs string
		want string
	}{
		// --- existing patterns ---
		{
			name: "postgres unreachable",
			logs: "dial tcp 10.0.0.1:5432: connect: connection refused",
			want: "Postgres unreachable — check the DB Service/endpoints + connection string.",
		},
		{
			name: "tls cert expired",
			logs: "certificate has expired",
			want: "TLS/cert issue — check the CA bundle and cert validity.",
		},
		{
			name: "disk full",
			logs: "no space left on device",
			want: "Disk full — check PVC / ephemeral-storage usage.",
		},
		{
			name: "network timeout",
			logs: "dial tcp 10.0.0.1:80: i/o timeout",
			want: "Network/dependency timeout — check the target Service / NetworkPolicy.",
		},
		{
			name: "permission denied",
			logs: "permission denied",
			want: "AuthZ/RBAC or filesystem-permission issue.",
		},
		{
			name: "oom",
			logs: "out of memory",
			want: "Memory exhaustion — raise limits or fix the leak.",
		},

		// --- language runtimes ---
		{
			name: "java oom",
			logs: "java.lang.OutOfMemoryError: Java heap space",
			want: "Java OOM — increase heap (-Xmx) or fix memory leak",
		},
		{
			name: "java stack overflow",
			logs: "Exception: java.lang.StackOverflowError at com.example.MyClass.method",
			want: "Java stack overflow — check for infinite recursion or deep call chains",
		},
		{
			name: "java npe",
			logs: "Caused by: java.lang.NullPointerException",
			want: "Java null pointer — check for uninitialized objects and add null checks",
		},
		{
			name: "python traceback",
			logs: "Traceback (most recent call last):\n  File \"app.py\", line 42, in main\n    do_stuff()",
			want: "Python unhandled exception — check traceback for root cause",
		},
		{
			name: "python module not found",
			logs: "ModuleNotFoundError: No module named 'requests'",
			want: "Python missing module — check requirements.txt and deployment",
		},
		{
			name: "python import error",
			logs: "ImportError: cannot import name 'get_data' from 'mylib'",
			want: "Python missing module — check requirements.txt and deployment",
		},
		{
			name: "nodejs uncaught exception",
			logs: "UnhandledPromiseRejectionWarning: Error: connect ECONNREFUSED",
			want: "Node.js unhandled exception — add error handlers or fix the rejection",
		},
		{
			name: "go nil pointer",
			logs: "http: panic serving 10.0.0.1: invalid memory address or nil pointer dereference",
			want: "Go nil pointer dereference — check for missing error handling",
		},
		{
			name: "go runtime panic",
			logs: "panic: runtime error: index out of range [0] with length 0",
			want: "Go runtime panic — check goroutine stacks for root cause",
		},
		{
			name: "segfault",
			logs: "signal 11 (SIGSEGV) received by worker process",
			want: "Segmentation fault (SIGSEGV) — check for null pointer, buffer overflow, or stack corruption",
		},

		// --- infrastructure dependencies ---
		{
			name: "redis connection refused",
			logs: "dial tcp 10.0.0.1:6379: connect: connection refused",
			want: "Redis unreachable — check the Redis Service, NetworkPolicy, and connection string",
		},
		{
			name: "redis readonly",
			logs: "READONLY You can't write against a read only replica.",
			want: "Redis unreachable — check the Redis Service, NetworkPolicy, and connection string",
		},
		{
			name: "mysql connection refused",
			logs: "dial tcp 10.0.0.1:3306: connect: connection refused",
			want: "MySQL unreachable — check the MySQL Service, credentials, and connectivity",
		},
		{
			name: "mysql access denied",
			logs: "Access denied for user 'app'@'10.0.0.2' (using password: YES)",
			want: "MySQL unreachable — check the MySQL Service, credentials, and connectivity",
		},
		{
			name: "mongodb connection refused",
			logs: "dial tcp 10.0.0.1:27017: connect: connection refused",
			want: "MongoDB unreachable — check the MongoDB Service, replica set, and URI",
		},
		{
			name: "mongodb not primary",
			logs: "MongoError: not primary and secondaryOk=false",
			want: "MongoDB unreachable — check the MongoDB Service, replica set, and URI",
		},
		{
			name: "elasticsearch no alive nodes",
			logs: "no alive nodes found in cluster",
			want: "Elasticsearch unreachable — check cluster health and connectivity",
		},
		{
			name: "elasticsearch connection refused",
			logs: "dial tcp 10.0.0.1:9200: connect: connection refused",
			want: "Elasticsearch unreachable — check cluster health and connectivity",
		},
		{
			name: "rabbitmq connection refused",
			logs: "dial tcp 10.0.0.1:5672: connect: connection refused",
			want: "RabbitMQ unreachable — check the RabbitMQ Service and connection URI",
		},
		{
			name: "upstream timeout",
			logs: "upstream timed out (110: Connection timed out) while connecting to upstream",
			want: "Upstream/ingress backend failure — check target Service health and connectivity",
		},
		{
			name: "no live upstreams",
			logs: "no live upstreams while connecting to upstream",
			want: "Upstream/ingress backend failure — check target Service health and connectivity",
		},

		// --- common app failures ---
		{
			name: "port conflict",
			logs: "listen tcp :8080: bind: address already in use",
			want: "Port conflict — check port assignments and competing processes",
		},
		{
			name: "file not found",
			logs: "open /etc/config/app.yaml: no such file or directory",
			want: "File or resource missing — check volume mounts, file paths, and ConfigMap/Secret mounts",
		},
		{
			name: "stack overflow",
			logs: "goroutine stack exceeds 1000000000-byte limit: stack overflow",
			want: "Stack overflow — check for infinite recursion or deep call chains",
		},
		{
			name: "config error",
			logs: "failed to load config: parse error at line 42: unexpected key",
			want: "Configuration error — check ConfigMap, env vars, and config file syntax",
		},
		{
			name: "rate limited",
			logs: "HTTP 429 Too Many Requests",
			want: "Rate limited by upstream — reduce request rate or increase quota",
		},
		{
			name: "connection reset",
			logs: "write tcp 10.0.0.1:8080->10.0.0.2:34567: write: connection reset by peer",
			want: "Connection reset by peer — check upstream stability and timeout settings",
		},
		{
			name: "dns failure no such host",
			logs: "lookup api.example.com on 10.0.0.53:53: no such host",
			want: "DNS resolution failure — check CoreDNS / DNS Service and network policy",
		},
		{
			name: "dns failure temporary",
			logs: "Temporary failure in name resolution",
			want: "DNS resolution failure — check CoreDNS / DNS Service and network policy",
		},
		{
			name: "connection pool exhausted",
			logs: "connection pool exhausted - try increasing max pool size",
			want: "Connection pool exhausted — increase pool size or check for connection leaks",
		},
		{
			name: "checksum mismatch",
			logs: "ERROR: sha256 mismatch for downloaded file",
			want: "File integrity failure — check for corrupted downloads or storage issues",
		},

		// --- edge cases ---
		{
			name: "empty logs",
			logs: "",
			want: "",
		},
		{
			name: "no match",
			logs: "everything is fine, no issues detected",
			want: "",
		},
		{
			name: "case insensitive redis",
			logs: "Readonly You can't write against a read only replica.",
			want: "Redis unreachable — check the Redis Service, NetworkPolicy, and connection string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SignatureHint(tt.logs)
			if got != tt.want {
				t.Errorf("SignatureHint() = %q, want %q", got, tt.want)
			}
		})
	}
}
