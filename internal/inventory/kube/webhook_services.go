package kube

import (
	"sort"
	"strings"

	admissionv1 "k8s.io/api/admissionregistration/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// AttrWebhookClosedServices lists the Services a webhook configuration
// calls with a webhook that refuses requests when it cannot be called,
// as "namespace/name", sorted and comma-separated. Each webhook has its
// own failure policy, so a Service that only an Ignore webhook calls is
// not in it, however the other webhooks of the configuration are set.
// It is always present: empty means no Service is called that way.
const AttrWebhookClosedServices = "webhook.closed.services"

// closedServices are the Services called by the webhooks whose policy is
// not Ignore, which are the ones that fail closed.
func closedServices(
	clients []admissionv1.WebhookClientConfig, policies []string,
) string {
	seen := map[string]bool{}
	var out []string
	for i, client := range clients {
		if client.Service == nil || strings.EqualFold(
			policies[i], string(admissionv1.Ignore)) {
			continue
		}
		name := client.Service.Namespace + "/" + client.Service.Name
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

// withClosedServices adds the closed Services to the attributes of a
// webhook configuration.
func withClosedServices(
	attrs map[string]inventory.Value,
	clients []admissionv1.WebhookClientConfig, policies []string,
) map[string]inventory.Value {
	attrs[AttrWebhookClosedServices] = inventory.Text(
		closedServices(clients, policies))
	return attrs
}
