package redact

import (
	"fmt"
	"regexp"
)

// commandFlag is a command whose password is the value of a short flag:
// "redis-cli -a hunter2". The tool must come before the flag on the line,
// so the same flag of another tool (ls -a, docker run -u) is not touched.
type commandFlag struct {
	tool string
	flag string
	// needs is a fragment of the command line the rule needs.
	needs string
}

var commandFlags = []commandFlag{
	{`redis-cli`, `-a|--pass`, "redis-cli"},
	{`sshpass`, `-p`, "sshpass"},
	{`(?:docker|podman|nerdctl)\s+login`, `-p`, "login"},
}

// commandRedactions are the rules of commandFlags and of the user:password
// flag of web clients.
func commandRedactions() []redactionRule {
	var rules []redactionRule
	for _, c := range commandFlags {
		rules = append(rules, redactionRule{pattern: regexp.MustCompile(
			fmt.Sprintf(`(\b%s\b[^\n|;&]*?\s(?:%s)\s*)([^\s\[-]\S*)`,
				c.tool, c.flag)),
			placeholder: redacted, needs: []string{c.needs}})
	}
	// curl -u user:password, curl --user=user:password.
	rules = append(rules, redactionRule{pattern: regexp.MustCompile(
		`(\b(?:curl|wget|svn|https?)\b[^\n|;&]*?` +
			`\s(?:-u|--user|--username)(?:\s+|=)[^\s:@'"]+:)([^\s\[]\S*)`,
	), placeholder: redacted,
		needs: []string{"curl", "wget", "svn", "http"}})
	return append(rules, quotedUserRules()...)
}

// quotedUserRules are the user:password flags written inside quotes, which
// may hold spaces: curl -u 'user:my pass'. Groups: 1 everything up to the
// password, 2 the password, 3 its closing quote when there is one.
func quotedUserRules() []redactionRule {
	const head = `(\b(?:curl|wget|svn|https?)\b[^\n|;&]*?` +
		`\s(?:-u|--user|--username)(?:\s+|=)`
	keep := func(g []string) string { return g[1] + redacted + g[3] }
	needs := []string{"curl", "wget", "svn", "http"}
	return []redactionRule{
		{pattern: regexp.MustCompile(
			head + `"[^":@\s]*:)((?:[^"\\\n]|\\.)*)("?)`),
			rebuild: keep, needs: needs},
		{pattern: regexp.MustCompile(head + `'[^':@\s]*:)([^'\n]*)('?)`),
			rebuild: keep, needs: needs},
	}
}
