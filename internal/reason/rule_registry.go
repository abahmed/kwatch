package reason

import (
	"fmt"
	"strings"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

// KindRegistry is the virtual entity for an image registry.
const KindRegistry = kube.KindRegistry

// RegistryRule blames an image registry when workloads that pull from the
// same registry cannot pull their images: an outage, an expired
// credential or a rate limit, not a typo in one image.
type RegistryRule struct{}

// Name implements Rule.
func (RegistryRule) Name() string { return "registry" }

// Explain implements Rule.
func (RegistryRule) Explain(q Query, symptom signal.Signal) []Hypothesis {
	if symptom.Reason != constant.ReasonImagePullBackOff &&
		symptom.Reason != constant.ReasonErrImagePull {
		return nil
	}
	images := q.Model.Related(symptom.Entity, knowledge.Pulls,
		knowledge.Outgoing)
	if len(images) == 0 {
		return nil
	}
	host := RegistryHost(images[0].Name)
	failing := map[knowledge.EntityID]bool{}
	for _, container := range q.Model.Entities(kube.KindContainer) {
		for _, s := range q.Signals.Active(container) {
			if s.Reason != constant.ReasonImagePullBackOff &&
				s.Reason != constant.ReasonErrImagePull {
				continue
			}
			for _, image := range q.Model.Related(container,
				knowledge.Pulls, knowledge.Outgoing) {
				if RegistryHost(image.Name) != host {
					continue
				}
				if pod, ok := PodOf(q.Model, container); ok {
					failing[TopOwner(q.Model, pod)] = true
				}
			}
		}
	}
	if len(failing) < 2 {
		return nil
	}
	s := newScorer(0.4)
	s.support(0.3, fmt.Sprintf("%d workloads cannot pull from %s",
		len(failing), host))
	if registryError(evidenceText(symptom)) {
		s.support(0.15, "the pull error is a registry or network error")
	}
	score, points := s.result()
	root := knowledge.NewEntityID(KindRegistry, "", host)
	return []Hypothesis{{
		Root:    root,
		Chain:   []knowledge.EntityID{root, symptom.Entity},
		Summary: "image registry " + host + " is unreachable or refusing pulls",
		Points:  points, Score: score,
	}}
}

// RegistryHost returns the registry of an image reference; references
// without a host come from Docker Hub.
func RegistryHost(image string) string {
	first, _, found := strings.Cut(image, "/")
	if !found || (!strings.ContainsAny(first, ".:") && first != "localhost") {
		return "docker.io"
	}
	return first
}

func registryError(text string) bool {
	text = strings.ToLower(text)
	for _, marker := range []string{
		"timeout", "connection refused", "no such host", "unauthorized",
		"toomanyrequests", "429", "503", "i/o timeout",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
