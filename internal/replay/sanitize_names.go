package replay

import (
	"regexp"
	"strings"
)

// Names inside kept text. The text is split into tokens of letters,
// digits, dots and hyphens; everything else (spaces, quotes, "_", ":",
// "/") separates them, so "volume_billing-api" and "pvc/data-0" still
// show their names. Names match whatever their case.
//
//   - A name the log hid gets its pseudonym.
//   - A name right after a Kubernetes kind ("configmap \"billing\"",
//     "pvc/data-0", "Secret db-creds") is hidden even when the log never
//     saw the object: a message about a missing object names it.
//   - A dotted name is kept when it is a number, an address, a version,
//     a duration, a code identifier ("app.ConfigError"), a file name
//     ("main.go") or one of Kubernetes' own domains; a cluster DNS name is
//     hidden label by label; any other is an outside host and is hidden.

// kindWords are the Kubernetes kinds, plural forms and short names that
// name an object right after them.
var kindWords = map[string]bool{}

func init() {
	for _, k := range []string{"configmap", "secret", "service", "pod",
		"deployment", "statefulset", "daemonset", "job", "cronjob",
		"persistentvolumeclaim", "persistentvolume", "ingress",
		"serviceaccount", "namespace", "node", "replicaset", "endpoint",
		"role", "rolebinding", "clusterrole", "clusterrolebinding"} {
		kindWords[k] = true
		kindWords[k+"s"] = true
	}
	for _, k := range []string{"svc", "pvc", "pv", "cm", "sa", "ns", "po",
		"deploy", "sts", "ds", "rs", "ing", "ingresses", "endpoints"} {
		kindWords[k] = true
	}
}

// namespaceKinds name a namespace, hidden as namespaces are.
var namespaceKinds = map[string]bool{"namespace": true,
	"namespaces": true, "ns": true}

// clusterLabels are the fixed labels of in-cluster DNS names.
var clusterLabels = map[string]bool{"svc": true, "cluster": true,
	"local": true, "pod": true}

// fileExtensions end a file name, kept: it says where code failed.
var fileExtensions = map[string]bool{"go": true, "py": true, "js": true,
	"ts": true, "mjs": true, "java": true, "kt": true, "cs": true,
	"rb": true, "rs": true, "php": true, "c": true, "cc": true,
	"cpp": true, "h": true, "sh": true, "yaml": true, "yml": true,
	"json": true, "toml": true, "ini": true, "conf": true, "cfg": true,
	"xml": true, "properties": true, "env": true, "log": true,
	"txt": true, "sql": true, "jar": true, "dll": true, "so": true,
	"html": true, "css": true, "lock": true, "pid": true, "sock": true,
	"db": true, "pem": true, "crt": true, "key": true}

// hostEndings are top-level domains seen in capitalized hosts
// ("Corp.Example.Com"), which are hosts, not code identifiers.
var hostEndings = map[string]bool{"com": true, "net": true, "org": true,
	"io": true, "dev": true, "app": true, "cloud": true, "local": true,
	"internal": true, "corp": true, "lan": true, "co": true, "ai": true,
	"us": true, "uk": true, "de": true, "eu": true, "info": true}

// numberLabel starts a label of a number, address, version or duration.
var numberLabel = regexp.MustCompile(`^v?[0-9]`)

// dnsName is what an object name after a kind word looks like.
var dnsName = regexp.MustCompile(
	`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)

func isTokenByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' ||
		b >= '0' && b <= '9' || b == '.' || b == '-'
}

// piece is a token and the separator text before it.
type piece struct{ sep, token string }

// split cuts text into pieces; a token's leading and trailing dots and
// hyphens belong to the separators. The last piece may have no token.
func split(text string) []piece {
	var out []piece
	i, start := 0, 0
	for i < len(text) {
		if !isTokenByte(text[i]) {
			i++
			continue
		}
		j := i
		for j < len(text) && isTokenByte(text[j]) {
			j++
		}
		tok := strings.TrimLeft(text[i:j], ".-")
		lead := j - i - len(tok)
		tok = strings.TrimRight(tok, ".-")
		if tok == "" {
			i = j
			continue
		}
		out = append(out, piece{sep: text[start : i+lead], token: tok})
		start = i + lead + len(tok)
		i = j
	}
	return append(out, piece{sep: text[start:]})
}

// scrubNames hides the names in text (see above).
func (s *Sanitizer) scrubNames(text string) string {
	pieces := split(text)
	var b strings.Builder
	b.Grow(len(text))
	kind := ""
	for _, p := range pieces {
		b.WriteString(p.sep)
		if p.token == "" {
			continue
		}
		b.WriteString(s.scrubToken(p, kind))
		kind = kindOf(p.token)
	}
	return b.String()
}

// kindOf is the kind word a token is ("Secret", "deployments.apps"),
// lowercased, or "". A kind with a group is marked "."-suffixed: it names
// an object after a quote or a space, never after "/", where it is a
// label or taint key ("node.kubernetes.io/not-ready").
func kindOf(token string) string {
	k, group, dotted := strings.Cut(strings.ToLower(token), ".")
	if !kindWords[k] {
		return ""
	}
	if dotted && group != "" {
		return k + "."
	}
	return k
}

// scrubToken hides one token; kind is the kind word just before it.
func (s *Sanitizer) scrubToken(p piece, kind string) string {
	lower := strings.ToLower(p.token)
	if pseudo, ok := s.known.lookup(lower); ok {
		if kind == "" && commonWords[lower] {
			// "using default value": a namespace or object named like
			// a common word stays a word unless a kind word names it.
			return p.token
		}
		return pseudo
	}
	if kind != "" && s.namesObject(p, lower, kind) {
		if namespaceKinds[strings.TrimSuffix(kind, ".")] {
			return s.optional("ns", lower)
		}
		return s.optional("name", lower)
	}
	if !strings.Contains(lower, ".") {
		return p.token
	}
	return s.dottedToken(p.token, lower)
}

// namesObject reports a token that names an object after a kind word:
// quoted or after "/", any name; after spaces only, a name with a digit,
// a hyphen or a dot, so plain words ("pod is pending") stay.
func (s *Sanitizer) namesObject(p piece, lower, kind string) bool {
	if !dnsName.MatchString(lower) || !strings.ContainsAny(lower,
		"abcdefghijklmnopqrstuvwxyz") {
		return false
	}
	sep := strings.TrimLeft(p.sep, " ")
	switch {
	case sep == "/":
		return !strings.HasSuffix(kind, ".")
	case sep == `"` || sep == "'" || sep == "`":
		return true
	case sep == "" && p.sep != "":
		return strings.ContainsAny(lower, "0123456789-.")
	}
	return false
}

// dottedToken hides a dotted name (see above).
func (s *Sanitizer) dottedToken(token, lower string) string {
	for _, domain := range systemDomains {
		if lower == domain || strings.HasSuffix(lower, "."+domain) {
			return token
		}
	}
	labels := strings.Split(token, ".")
	lowers := strings.Split(lower, ".")
	if numberLabel.MatchString(lowers[0]) && allNumbery(lowers) {
		return token
	}
	lastLower := lowers[len(lowers)-1]
	// A known object or namespace among the labels is never code or a
	// file name: "ledger.Shop.Example" names the object ledger.
	if !s.anyKnown(lowers) {
		if fileExtensions[lastLower] {
			return token
		}
		if codeIdentifier(labels) && !hostEndings[lastLower] {
			return token
		}
	}
	if inCluster(lowers) {
		return s.clusterName(lowers)
	}
	if out, ok := s.knownLabels(lowers); ok {
		return out
	}
	return s.pseudonym("host", lower)
}

// allNumbery reports labels that each start with a digit (an address,
// a version, a duration): "10.0.0.5", "1.29.4", "v1.2", "2.5s".
func allNumbery(lowers []string) bool {
	for _, l := range lowers {
		if !numberLabel.MatchString(l) {
			return false
		}
	}
	return true
}

// codeIdentifier reports a dotted code name: its last label is written
// like a type ("app.ConfigError", "java.lang.Error"), or a lower-case
// method follows one ("Main.run"). A capitalized label elsewhere is not
// enough: "db.Prod.acme.xyz" and "Billing.Example.Fr" are hosts. Hosts
// ending in a top-level domain are ruled out by the caller.
func codeIdentifier(labels []string) bool {
	last := labels[len(labels)-1]
	if typeName(last) {
		return true
	}
	// A method of a type: "Main.run", "Resolver.visit".
	return len(labels) >= 2 && typeName(labels[len(labels)-2]) &&
		last != "" && last[0] >= 'a' && last[0] <= 'z'
}

// typeName reports a label written like a type: an upper-case start, a
// lower-case letter, at least 3 long ("Error", "Main").
func typeName(l string) bool {
	return len(l) >= 3 && l[0] >= 'A' && l[0] <= 'Z' &&
		strings.ContainsAny(l, "abcdefghijklmnopqrstuvwxyz")
}

// anyKnown reports a label that is a known object or namespace name.
func (s *Sanitizer) anyKnown(lowers []string) bool {
	for _, l := range lowers {
		if _, ok := s.known.lookup(l); ok {
			return true
		}
	}
	return false
}

// commonWords are words a namespace or object is often named, kept as
// words in text unless a kind word names them.
var commonWords = map[string]bool{"default": true, "public": true,
	"system": true, "main": true, "app": true, "api": true, "web": true}

func inCluster(lowers []string) bool {
	last := lowers[len(lowers)-1]
	for _, l := range lowers {
		if l == "svc" {
			return true
		}
	}
	return last == "local" && len(lowers) > 2
}

// clusterName hides a cluster DNS name label by label: the fixed labels
// stay, the first is an object name, the one before "svc" a namespace.
func (s *Sanitizer) clusterName(lowers []string) string {
	out := make([]string, len(lowers))
	for i, l := range lowers {
		switch {
		case clusterLabels[l]:
			out[i] = l
		case i+1 < len(lowers) && lowers[i+1] == "svc":
			out[i] = s.optional("ns", l)
		default:
			if pseudo, ok := s.known.lookup(l); ok {
				out[i] = pseudo
				continue
			}
			out[i] = s.optional("name", l)
		}
	}
	return strings.Join(out, ".")
}

// knownLabels hides a dotted name whose every label the log hid
// ("ledger.shop"), label by label; false when one label is unknown.
func (s *Sanitizer) knownLabels(lowers []string) (string, bool) {
	out := make([]string, len(lowers))
	for i, l := range lowers {
		pseudo, ok := s.known.lookup(l)
		if !ok {
			return "", false
		}
		out[i] = pseudo
	}
	return strings.Join(out, "."), true
}
