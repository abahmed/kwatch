package explain

import "testing"

// Of several clients a line names, the one nearest the failure, in its
// clause, or on the port the call used is the one that failed; a path
// segment is not a client at all.
func TestDependencyInPicksTheClientOfTheFailure(t *testing.T) {
	cases := map[string]Dependency{
		"health: redis ok, postgres timed out": {Name: "PostgreSQL"},
		"loaded redis config from /etc/app.yaml, request to " +
			"api.example.com:443 timed out": {Host: "api.example.com:443"},
		"redis via mongo dial tcp 10.0.0.9:6379: connect: " +
			"connection refused": {Name: "Redis", Host: "10.0.0.9:6379"},
		"redis cache and the postgres pool: connection refused": {
			Name: "PostgreSQL"},
		"dial unix /var/run/redis/redis.sock: connect: " +
			"connection refused": {Name: "Redis"},
	}
	for line, want := range cases {
		got, ok := DependencyIn(line)
		if !ok || got != want {
			t.Errorf("%q: got %+v (%v), want %+v", line, got, ok, want)
		}
	}
	for _, line := range []string{
		"GET /users/redis 504 upstream timed out",
		"POST /api/kafka/events timed out",
	} {
		if d, ok := DependencyIn(line); ok {
			t.Errorf("%q read as %+v", line, d)
		}
	}
}

// Clients that say a connection failed in their own words are read:
// Kafka's Java client, libpq through psycopg2 and in its newer wording,
// and .NET's SqlClient.
func TestDependencyInReadsClientSentences(t *testing.T) {
	cases := map[string]Dependency{
		"[Consumer clientId=c] Connection to node -1 (broker.prod/" +
			"10.0.0.7:9093) could not be established. Broker may not " +
			"be available.": {Name: "Kafka", Host: "10.0.0.7:9093"},
		"could not connect to server: Connection refused\n\tIs the " +
			"server running on host \"db.prod\" (10.0.0.5) and " +
			"accepting\n\tTCP/IP connections on port 5433?": {
			Name: "PostgreSQL", Host: "db.prod:5433"},
		"connection to server at \"db.prod\" (10.0.0.5), port 5433 " +
			"failed: Connection refused": {Name: "PostgreSQL",
			Host: "db.prod:5433"},
		"Microsoft.Data.SqlClient.SqlException (0x80131904): A " +
			"network-related or instance-specific error occurred while " +
			"establishing a connection to SQL Server.": {
			Name: "SQL Server"},
		"psycopg.OperationalError: the call timed out": {
			Name: "PostgreSQL"},
	}
	for line, want := range cases {
		got, ok := DependencyIn(line)
		if !ok || got != want {
			t.Errorf("%q: got %+v (%v), want %+v", line, got, ok, want)
		}
	}
}
