package filter

import (
	"fmt"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/labels"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

// Scope decides which signals people want to hear about: the configured
// namespaces and reasons, the namespace label selector, and silence rules.
// It is immutable after construction and safe for concurrent use.
type Scope struct {
	allowedNamespaces   set
	forbiddenNamespaces set
	allowedReasons      set
	forbiddenReasons    set
	selector            labels.Selector
	silences            []silence
}

// NewScope compiles the scope policy. An invalid selector or pattern is a
// configuration error rather than a silently wider scope.
func NewScope(
	scope config.ScopeRuntime, rules []config.SilenceRule,
) (*Scope, error) {
	s := &Scope{
		allowedNamespaces:   newSet(scope.AllowedNamespaces()),
		forbiddenNamespaces: newSet(scope.ForbiddenNamespaces()),
		allowedReasons:      newSet(scope.AllowedReasons()),
		forbiddenReasons:    newSet(scope.ForbiddenReasons()),
	}
	if raw := scope.NamespaceSelector(); raw != "" {
		selector, err := labels.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("namespace selector: %w", err)
		}
		s.selector = selector
	}
	for i, rule := range rules {
		compiled, err := compileSilence(rule)
		if err != nil {
			return nil, fmt.Errorf("silence %d: %w", i, err)
		}
		s.silences = append(s.silences, compiled)
	}
	return s, nil
}

// Allows reports whether a signal is in scope and not silenced. model
// supplies namespace labels for the selector.
func (s *Scope) Allows(model knowledge.Reader, sig signal.Signal) bool {
	if s == nil {
		return true
	}
	if !s.namespaceAllowed(model, sig.Entity.Namespace) ||
		!s.allowedReasons.allows(sig.Reason) ||
		s.forbiddenReasons.has(sig.Reason) {
		return false
	}
	for _, rule := range s.silences {
		if rule.matches(sig) {
			return false
		}
	}
	return true
}

// namespaceAllowed keeps cluster-scoped signals: a node or storage class
// failure affects every namespace.
func (s *Scope) namespaceAllowed(
	model knowledge.Reader, namespace string,
) bool {
	if namespace == "" {
		return true
	}
	if !s.allowedNamespaces.allows(namespace) ||
		s.forbiddenNamespaces.has(namespace) {
		return false
	}
	if s.selector == nil {
		return true
	}
	return s.selector.Matches(namespaceLabels(model, namespace))
}

func namespaceLabels(model knowledge.Reader, namespace string) labels.Set {
	entity, ok := model.Entity(
		knowledge.NewEntityID(kube.KindNamespace, "", namespace))
	if !ok {
		return nil
	}
	attribute, ok := entity.Attribute(kube.AttrLabels)
	if !ok {
		return nil
	}
	text := attribute.Value.AsText()
	set, err := labels.ConvertSelectorToLabelsMap(text)
	if err != nil {
		return nil
	}
	return set
}

// silence matches a signal when every field the rule sets matches.
type silence struct {
	namespaces        set
	reasons           set
	podNames          []*regexp.Regexp
	containerNames    set
	containerMessages []string
	eventMessages     []string
	nodeReasons       set
	nodeMessages      []string
}

func compileSilence(rule config.SilenceRule) (silence, error) {
	out := silence{
		namespaces:        newSet(rule.Namespaces),
		reasons:           newSet(rule.Reasons),
		containerNames:    newSet(rule.ContainerNames),
		containerMessages: rule.ContainerMessages,
		eventMessages:     rule.EventMessages,
		nodeReasons:       newSet(rule.NodeReasons),
		nodeMessages:      rule.NodeMessages,
	}
	for _, pattern := range rule.PodNamePatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return silence{}, fmt.Errorf("pod name pattern %q: %w",
				pattern, err)
		}
		out.podNames = append(out.podNames, re)
	}
	return out, nil
}

func (r silence) matches(sig signal.Signal) bool {
	id := sig.Entity
	pod, container := podAndContainer(id)
	onNode := id.Kind == kube.KindNode
	text := signalText(sig)
	return r.namespaces.allows(id.Namespace) &&
		r.reasons.allows(sig.Reason) &&
		(len(r.podNames) == 0 || (pod != "" && anyRegex(r.podNames, pod))) &&
		r.containerNames.allows(container) &&
		(len(r.containerMessages) == 0 ||
			(pod != "" && containsAny(text, r.containerMessages))) &&
		(len(r.eventMessages) == 0 || containsAny(text, r.eventMessages)) &&
		(r.nodeReasons.empty() || (onNode && r.nodeReasons.has(sig.Reason))) &&
		(len(r.nodeMessages) == 0 ||
			(onNode && containsAny(text, r.nodeMessages)))
}

// podAndContainer names the pod and container a signal is about.
func podAndContainer(id knowledge.EntityID) (string, string) {
	switch id.Kind {
	case kube.KindPod:
		return id.Name, ""
	case kube.KindContainer:
		pod, container, _ := strings.Cut(id.Name, "/")
		return pod, container
	default:
		return "", ""
	}
}

// signalText is what message matchers search: the summary and evidence.
func signalText(sig signal.Signal) string {
	parts := []string{sig.Summary}
	for _, evidence := range sig.Evidence {
		parts = append(parts, evidence.Value)
	}
	return strings.Join(parts, "\n")
}

func anyRegex(patterns []*regexp.Regexp, value string) bool {
	for _, re := range patterns {
		if re.MatchString(value) {
			return true
		}
	}
	return false
}

func containsAny(value string, candidates []string) bool {
	for _, candidate := range candidates {
		if candidate != "" && strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

// set is a string set where an empty set places no constraint.
type set map[string]struct{}

func newSet(values []string) set {
	out := make(set, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func (s set) empty() bool { return len(s) == 0 }

func (s set) has(value string) bool {
	_, ok := s[value]
	return ok
}

// allows is true when the set is unconstrained or contains value.
func (s set) allows(value string) bool { return s.empty() || s.has(value) }
