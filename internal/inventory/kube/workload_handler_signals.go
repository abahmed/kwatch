package kube

import (
	"regexp"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/redact"
)

// Attributes of a workload's pod template that show which API group or
// finalizer its pods serve. They are matched against names, never shown.
const (
	// AttrTemplateAnnotationDomains lists the domains of the annotation
	// keys of the pods a workload creates ("widgets.example.com" for
	// "widgets.example.com/owner"), space-separated and sorted. Domains
	// that Kubernetes itself owns are left out.
	AttrTemplateAnnotationDomains = "template.annotation.domains"
	// AttrTemplateQuoted lists the qualified names (an API group such as
	// "widgets.example.com" or a finalizer such as
	// "widgets.example.com/cleanup") that the containers' arguments and
	// literal environment values contain, space-separated. Values of
	// anything named like a credential are never read.
	AttrTemplateQuoted = "template.quoted"
)

// Bounds on what a template keeps.
const (
	maxTemplateDomains = 16
	maxQuotedNames     = 16
	maxQuotedNameLen   = 128
)

// qualifiedName finds an API group or a finalizer inside a text: DNS
// labels with at least one dot, and an optional "/name".
var qualifiedName = regexp.MustCompile(
	`[a-z0-9](?:[-a-z0-9]*[a-z0-9])?(?:\.[a-z0-9](?:[-a-z0-9]*[a-z0-9])?)+` +
		`(?:/[A-Za-z0-9._-]+)?`)

// addressURL finds an address in a text: its scheme, an optional
// "user:password@", the host, and whatever path or query follows. Only
// the host is a name; the rest can hold a credential.
var addressURL = regexp.MustCompile(
	`(?i)[a-z][a-z0-9+.-]*://(?:[^\s/?#]*@)?([^\s/?#@]*)(?:[/?#]\S*)?`)

// setHandlerSignals adds the attributes above for a pod template.
func setHandlerSignals(
	attrs map[string]inventory.Value, template *corev1.PodTemplateSpec,
) {
	if domains := annotationDomains(template.Annotations); len(domains) > 0 {
		attrs[AttrTemplateAnnotationDomains] = inventory.Text(
			strings.Join(domains, " "))
	}
	if names := quotedNames(template.Spec); len(names) > 0 {
		attrs[AttrTemplateQuoted] = inventory.Text(
			strings.Join(names, " "))
	}
}

// annotationDomains are the distinct domains of annotation keys, except
// the ones Kubernetes owns.
func annotationDomains(annotations map[string]string) []string {
	seen := map[string]bool{}
	var out []string
	for key := range annotations {
		domain, _, found := strings.Cut(key, "/")
		domain = strings.ToLower(domain)
		if found && !seen[domain] && !kubernetesDomain(domain) {
			seen[domain] = true
			out = append(out, domain)
		}
	}
	sort.Strings(out)
	if len(out) > maxTemplateDomains {
		out = out[:maxTemplateDomains]
	}
	return out
}

func kubernetesDomain(domain string) bool {
	for _, owner := range []string{"kubernetes.io", "k8s.io"} {
		if domain == owner || strings.HasSuffix(domain, "."+owner) {
			return true
		}
	}
	return false
}

// quotedNames are the qualified names the containers' arguments and
// literal environment values contain, in order of appearance.
func quotedNames(spec corev1.PodSpec) []string {
	var out []string
	add := func(text string) {
		for _, name := range qualifiedName.FindAllString(
			nameSource(text), -1) {
			out = addQuoted(out, name)
		}
	}
	containers := append(append([]corev1.Container{}, spec.InitContainers...),
		spec.Containers...)
	for _, c := range containers {
		skipNext := false
		for _, arg := range append(append([]string{}, c.Command...), c.Args...) {
			skipNext = readArg(arg, skipNext, add)
		}
		for _, env := range c.Env {
			if env.ValueFrom == nil && !secretNamed(env.Name) {
				add(env.Value)
			}
		}
	}
	return out
}

// readArg reads one argument. A flag named like a credential hides its
// own value, or the next argument when it has none. It returns whether
// the next argument is hidden.
func readArg(arg string, hidden bool, add func(string)) bool {
	if hidden {
		return false
	}
	if !strings.HasPrefix(arg, "-") {
		add(arg)
		return false
	}
	flag, value, hasValue := strings.Cut(arg, "=")
	switch {
	case secretNamed(flag):
		return !hasValue
	case hasValue:
		add(value)
	}
	return false
}

// nameSource is text with every credential removed, so that no fragment
// of a password or a user name can be taken for a name: a connection URL
// keeps its host only.
func nameSource(text string) string {
	return addressURL.ReplaceAllString(redact.Credentials(text), "$1")
}

// secretNamed reports a flag or variable name whose value may be a
// credential. It shares its words with the rule that hides values from
// messages (env_visibility.go).
func secretNamed(name string) bool {
	name = strings.ToLower(name)
	for _, word := range secretNameWords {
		if strings.Contains(name, word) {
			return true
		}
	}
	return false
}

// addQuoted appends name unless it is an address or a version (no
// letters in its last label), too long, a repeat, or the list is full.
func addQuoted(names []string, name string) []string {
	domain, _, _ := strings.Cut(name, "/")
	last := domain[strings.LastIndex(domain, ".")+1:]
	if len(names) >= maxQuotedNames || len(name) > maxQuotedNameLen ||
		!strings.ContainsAny(last, "abcdefghijklmnopqrstuvwxyz") {
		return names
	}
	for _, have := range names {
		if have == name {
			return names
		}
	}
	return append(names, name)
}
