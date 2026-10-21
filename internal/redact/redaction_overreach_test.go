package redact

import "testing"

// Words that happen to contain "pass" are not credentials: a Go test
// result, an nginx directive, a feature flag.
func TestRedactLeavesPassWordsThatAreNotCredentials(t *testing.T) {
	runTable(t, []struct{ name, in, want string }{
		{"go test result", "PASS: TestSomeLongName",
			"PASS: TestSomeLongName"},
		{"go test verbose", "--- PASS: TestSomeLongName (0.00s)",
			"--- PASS: TestSomeLongName (0.00s)"},
		{"go benchmark", "PASS: BenchmarkParseHeaders-8",
			"PASS: BenchmarkParseHeaders-8"},
		{"go example", "PASS: ExampleClientDo", "PASS: ExampleClientDo"},
		{"pass through flag", "pass_through=true",
			"pass_through=true"},
		{"pass through word", "pass-through: enabled",
			"pass-through: enabled"},
		{"proxy pass", "proxy_pass=http://upstream:8080/api",
			"proxy_pass=http://upstream:8080/api"},
		{"proxy pass json", `{"proxy_pass": "http://upstream:8080"}`,
			`{"proxy_pass": "http://upstream:8080"}`},
		{"proxy pass in a url", "http://h/x?proxy_pass=http://u",
			"http://h/x?proxy_pass=http://u"},
	})
}

// A real password next to those words is still redacted.
func TestRedactStillRedactsPassNextToTheHarmlessWords(t *testing.T) {
	runTable(t, []struct{ name, in, want string }{
		{"pass colon", "pass: Hunter2Hunter2", "pass: [redacted]"},
		{"pass equals", "pass=Hunter2Hunter2", "pass=[redacted]"},
		{"test word as password", "PASS: Hunter2Hunter2",
			"PASS: [redacted]"},
		{"db pass", "db_pass=s3cretValue", "db_pass=[redacted]"},
		{"test lookalike", "pass: TestPassw0rd9", "pass: [redacted]"},
	})
}

// A quoted user:password of curl, wget and friends is a credential.
func TestRedactQuotedUserPasswordFlags(t *testing.T) {
	runTable(t, []struct{ name, in, want string }{
		{"single", "curl -u 'alice:pw' https://h/x",
			"curl -u 'alice:[redacted]' https://h/x"},
		{"double", `curl --user "alice:pw" https://h/x`,
			`curl --user "alice:[redacted]" https://h/x`},
		{"equals", `curl --user="alice:pw" https://h/x`,
			`curl --user="alice:[redacted]" https://h/x`},
		{"spaces in password", `curl -u "alice:my pass word" https://h`,
			`curl -u "alice:[redacted]" https://h`},
		{"plain still works", "curl -u alice:hunter2 https://h/x",
			"curl -u alice:[redacted] https://h/x"},
		{"no password stays", `curl -u "alice" https://h`,
			`curl -u "alice" https://h`},
	})
}

// A value cut off before its closing quote (a truncated log line) must
// not leave its tail behind.
func TestRedactUnterminatedQuotedValues(t *testing.T) {
	runTable(t, []struct{ name, in, want string }{
		{"double", `login failed password="hunter two three`,
			"login failed password=[redacted]"},
		{"single", "login failed password='hunter two three",
			"login failed password=[redacted]"},
		{"json", `{"user":"bob","password":"hunter two three`,
			`{"user":"bob","password":"[redacted]"`},
		{"stops at the line", "password=\"hunter two\nnext line stays",
			"password=[redacted]\nnext line stays"},
		{"terminated is unchanged", `password="a b" next`,
			"password=[redacted] next"},
	})
}
