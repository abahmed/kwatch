package redact

import (
	"net"
	"regexp"
	"strings"
)

// bareUserinfoRule redacts the password of a "user:password@host" written
// without a scheme, as in "connecting to admin:s3cret@db:5432". The user
// must start a word (after a space, a quote, a backtick, a bracket, a
// brace, "=", "," or ":") with a letter, so the tail of an image path, a
// time or a duration never matches. The password may hold any of
// "#/?;^{}:@" (it runs to the last "@" before the host), but no "*",
// "[" or space, so emphasis before a mention and an address already
// redacted stay. Groups: 1 the lead and the user with its ":", 2 the
// password, 3 the host.
var bareUserinfoRule = redactionRule{
	pattern: regexp.MustCompile(
		"((?:^|[\\s\"'(\\[<=,`{:])[A-Za-z_][A-Za-z0-9._~%+-]*:)" +
			`([A-Za-z0-9._~%!$&+=@#/?;^{}:-]+)@` +
			`([A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?` +
			`(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?)*|` +
			`\[[0-9A-Fa-f:.]+(?:%[A-Za-z0-9]+)?\])`,
	),
	skip: skipNotUserinfo,
	rebuild: func(g []string) string {
		return g[1] + redacted + "@" + g[3]
	},
	needs: []string{"@"},
}

var userinfoRedactions = []redactionRule{bareUserinfoRule}

// addressKeys are words that put an e-mail address after a colon
// ("mailto:bob@example.com", "from:ops@example.com"); the word is not a
// user name and the address is not a password.
var addressKeys = map[string]bool{
	"mailto": true, "email": true, "e-mail": true, "mail": true,
	"from": true, "to": true, "cc": true, "bcc": true, "reply-to": true,
	"sip": true, "sips": true, "xmpp": true, "contact": true,
	"author": true, "owner": true, "maintainer": true,
	// Reply prefixes of a mail subject: "Subject: re:bob@example.com".
	"re": true, "fw": true, "fwd": true,
}

// mentionHosts are chat mentions, not hosts: "ping:all@here".
var mentionHosts = map[string]bool{
	"channel": true, "here": true, "everyone": true,
}

var digestAlgo = regexp.MustCompile(`(?i)^(?:sha\d+|md5|blake\d*)$`)

// version is a package version after "@" ("npm:lodash@4.17.21",
// "@1.0.0-beta.1"); an IPv4 address reads the same and is a host.
var version = regexp.MustCompile(`^\d+(?:\.\d+)+(?:-[0-9A-Za-z.-]+)?$`)

// nameValue is a plain word, as the value of a key that names
// something ("cluster_name:rabbit@rabbitmq-0") is.
var nameValue = regexp.MustCompile(`^[A-Za-z]+$`)

// skipNotUserinfo keeps matches that only look like userinfo: an address
// after a key word, a chat mention, an image reference pinned by digest
// ("nginx:1.25@sha256:..."), a package version, the rest of a URL
// ("http://h/p?x=1@y") or a plain name after a "…name" key. A real
// user is rarely called "…name" with a letters-only password, so that
// lookalike is kept; anything else after "@" a host may be is redacted,
// since a leak costs more than a hidden name.
func skipNotUserinfo(g []string) bool {
	if len(g) < 4 {
		return true
	}
	user := strings.ToLower(strings.TrimRight(
		strings.TrimLeft(g[1], " \t\r\n\"'`(<[{=,:"), ":"))
	// A key word before a "user:password" ("email:admin:pw@db") does
	// not make the password an address.
	keyed := addressKeys[user] && !strings.Contains(g[2], ":")
	return keyed || mentionHosts[strings.ToLower(g[3])] ||
		digestAlgo.MatchString(g[3]) ||
		(version.MatchString(g[3]) && net.ParseIP(g[3]) == nil) ||
		strings.HasPrefix(g[2], "/") ||
		(strings.HasSuffix(user, "name") && nameValue.MatchString(g[2]))
}

// portPath is a port followed by a path: what stands after "host:" in
// "http://registry:4873/@scope%2fpkg".
var portPath = regexp.MustCompile(`^\d{1,5}/`)

// skipHostPort keeps a URL whose "user:password" is really "host:port"
// followed by a path with an "@" in it ("https://api:8443/v1/users/@me").
// A password may hold "/", so the match cannot stop there; a number
// and a "/" right after the colon is a port, not a password.
func skipHostPort(g []string) bool {
	if len(g) < 2 || len(g[0]) <= len(g[1]) {
		return false
	}
	rest := g[0][len(g[1]):]
	_, after, ok := strings.Cut(rest, ":")
	return ok && portPath.MatchString(after)
}
