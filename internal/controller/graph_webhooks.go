package controller

import (
	admissionv1 "k8s.io/api/admissionregistration/v1"

	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/monitor/security"
)

func (c *Controller) rebuildMutatingWebhookGraph(obj interface{}) {
	cfg, ok := obj.(*admissionv1.MutatingWebhookConfiguration)
	if !ok || c.graph == nil {
		return
	}
	c.replaceWebhookEdges(
		"mutatingwebhookconfiguration", cfg.Name,
		security.MutatingWebhookServices(cfg),
	)
}

func (c *Controller) rebuildValidatingWebhookGraph(obj interface{}) {
	cfg, ok := obj.(*admissionv1.ValidatingWebhookConfiguration)
	if !ok || c.graph == nil {
		return
	}
	c.replaceWebhookEdges(
		"validatingwebhookconfiguration", cfg.Name,
		security.ValidatingWebhookServices(cfg),
	)
}

func (c *Controller) replaceWebhookEdges(
	kind, name string,
	refs []*admissionv1.ServiceReference,
) {
	targets := make([]kwcontext.EdgeTarget, 0, len(refs))
	seen := make(map[string]bool)
	for _, ref := range refs {
		if ref == nil || ref.Name == "" || ref.Namespace == "" {
			continue
		}
		key := ref.Namespace + "/" + ref.Name
		if seen[key] {
			continue
		}
		seen[key] = true
		targets = append(targets, kwcontext.EdgeTarget{
			Kind: "service", Namespace: ref.Namespace, Name: ref.Name,
			Type: "uses_webhook_backend",
		})
	}
	c.graph.ReplaceOutgoingEdges(kind, "", name, targets)
}
