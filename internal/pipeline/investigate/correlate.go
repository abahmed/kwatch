package investigate

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// signatureClasses classify a failure line, first match wins.
var signatureClasses = []struct {
	class   string
	pattern *regexp.Regexp
}{
	{"panic", regexp.MustCompile(`(?i)\bpanic:|goroutine \d+ \[|` +
		`traceback|sigsegv|segmentation fault`)},
	{"out of memory", regexp.MustCompile(`(?i)out of memory|` +
		`outofmemoryerror|cannot allocate memory`)},
	{"connection refused", regexp.MustCompile(`(?i)connection refused|` +
		`econnrefused`)},
	{"DNS failure", regexp.MustCompile(`(?i)no such host|nxdomain|` +
		`name or service not known|could not resolve|getaddrinfo`)},
	{"TLS error", regexp.MustCompile(`(?i)x509:|tls: |certificate`)},
	{"timeout", regexp.MustCompile(`(?i)i/o timeout|deadline exceeded|` +
		`timed out`)},
	{"permission denied", regexp.MustCompile(`(?i)permission denied|` +
		`eacces|forbidden`)},
	{"missing setting or file", regexp.MustCompile(`(?i)no such file|` +
		`enoent|is not set|missing required|must be set`)},
	{"migration or lock", regexp.MustCompile(`(?i)migration|deadlock|` +
		`lock (wait|timeout)|could not obtain lock`)},
}

// hostPort finds "host:port" or a bare address in a line.
var hostPort = regexp.MustCompile(
	`\b([a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9-]+)*):(\d{2,5})\b`)

// signature is the classified failure line of a report and the cluster
// object it points to, if any.
type signature struct {
	class, line string
	target      inventory.EntityID
	found       bool
}

// correlate concludes across the report: the failure signature and the
// object it names, the nearest change before the first failure, the
// spread of the signature and, for a rollout, what changed. It is nil
// when it has nothing to say.
func correlate(
	model inventory.Reader, p incident.Incident, report []incident.Section,
) *incident.Correlation {
	out := &incident.Correlation{}
	sig := failureSignature(model, p, report)
	if sig.class != "" {
		out.Facts = append(out.Facts, incident.SectionFact{
			Label: incident.CorrelationSignature,
			Value: sig.class + ": " + sig.line})
	}
	if sig.found {
		out.Facts = append(out.Facts, incident.SectionFact{
			Label: incident.CorrelationMatches,
			Value: describeTarget(model, sig.target)})
	}
	add := func(label, value string) {
		if value != "" {
			out.Facts = append(out.Facts,
				incident.SectionFact{Label: label, Value: value})
		}
	}
	add(incident.CorrelationChange, nearestChange(model, p))
	add(incident.CorrelationSpread, spreadOf(model, p, report, sig))
	add(incident.CorrelationDiff, rolloutDiff(report))
	if p.Cause != nil || rootFails(p) {
		out.Confirmed = confirm(model, p, report, sig)
	}
	if len(out.Facts) == 0 && out.Confirmed == "" {
		return nil
	}
	return out
}

// failureSignature classifies the first matched line of the log
// sections and links the host it names to a Service, Pod or Node.
func failureSignature(
	model inventory.Reader, p incident.Incident, report []incident.Section,
) signature {
	for _, s := range report {
		if s.Check != incident.CheckLogs && s.Check != incident.CheckCrash {
			continue
		}
		for _, line := range matchedLines(s) {
			class := classify(line)
			if class == "" {
				continue
			}
			sig := signature{class: class, line: clip(line)}
			sig.target, sig.found = hostTarget(model, p, line)
			return sig
		}
	}
	return signature{}
}

func matchedLines(s incident.Section) []string {
	if len(s.Matched) == 0 {
		return s.Lines
	}
	var out []string
	for _, i := range s.Matched {
		if i >= 0 && i < len(s.Lines) {
			out = append(out, s.Lines[i])
		}
	}
	return out
}

func classify(line string) string {
	for _, c := range signatureClasses {
		if c.pattern.MatchString(line) {
			return c.class
		}
	}
	return ""
}

// hostTarget is the cluster object a line's address or host names: an
// address owned by a Service, Pod or Node, or a Service name.
func hostTarget(
	model inventory.Reader, p incident.Incident, line string,
) (inventory.EntityID, bool) {
	for _, m := range ipv4Address.FindAllStringSubmatch(line, -1) {
		if id, ok := ipOwner(model, m[1]); ok {
			return id, true
		}
	}
	for _, ref := range kube.ServiceRefsIn(line, p.Root.Namespace) {
		if model.Exists(ref.Service) {
			return ref.Service, true
		}
	}
	for _, m := range hostPort.FindAllStringSubmatch(line, -1) {
		name, _, _ := strings.Cut(m[1], ".")
		id := inventory.CoreID(kube.KindService, p.Root.Namespace, name)
		if model.Exists(id) {
			return id, true
		}
	}
	return inventory.EntityID{}, false
}

// ipOwner is the Service, Pod or Node that owns ip.
func ipOwner(model inventory.Reader, ip string) (inventory.EntityID, bool) {
	for _, own := range []struct {
		kind inventory.Kind
		attr string
	}{
		{kube.KindService, kube.AttrClusterIP},
		{kube.KindPod, kube.AttrPodIP},
		{kube.KindNode, kube.AttrNodeIP},
	} {
		for _, id := range model.Entities(own.kind) {
			if attributeText(model, id, own.attr) == ip {
				return id, true
			}
		}
	}
	return inventory.EntityID{}, false
}

// describeTarget names the object and its state: "Service shop/db, 0
// ready endpoints, selects 0 pods".
func describeTarget(model inventory.Reader, id inventory.EntityID) string {
	text := string(id.Kind) + " " + ownerName(id)
	if id.Kind != kube.KindService {
		return text
	}
	if ready, ok := modelEndpoints(model, id); ok {
		text += ", " + strconv.Itoa(ready) + " ready endpoints"
	}
	selected := relatedOf(model, id, inventory.Selects, inventory.Outgoing,
		kube.KindPod)
	return text + ", selects " + strconv.Itoa(len(selected)) + " pods"
}

// rootFails reports an incident whose root has a finding of its own:
// what failed is the cause, with or without a cause record.
func rootFails(p incident.Incident) bool {
	for _, m := range p.Members {
		if m.Entity == p.Root && !m.Advisory {
			return true
		}
	}
	return false
}
