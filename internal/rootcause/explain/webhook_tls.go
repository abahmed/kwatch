package explain

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// servingCert finds, for a webhook whose calls fail the TLS handshake,
// the Secret its backend pods mount whose certificate already expired.
// The expiry is the one the existing certificate check reads from the
// public certificate; a certificate that has not ended is no cause, so
// nothing is said about it. When several Secrets qualify the
// first by name wins.
func (s Snapshot) servingCert(c Cause) *rootcause.ServingCert {
	if c.Mode != ModeWebhookTLS || !webhookKind(c.Root.Kind) {
		return nil
	}
	var best *rootcause.ServingCert
	for _, service := range s.Model.Related(
		c.Root, inventory.Serves, inventory.Outgoing) {
		for _, pod := range s.servicePods(service) {
			for _, secret := range s.Model.Related(pod,
				inventory.References, inventory.Outgoing) {
				found := s.expiredSecret(secret)
				if found != nil && (best == nil ||
					found.Secret.Name < best.Secret.Name) {
					best = found
				}
			}
		}
	}
	return best
}

// servicePods are the pods a Service selects.
func (s Snapshot) servicePods(service inventory.EntityID,
) []inventory.EntityID {
	var out []inventory.EntityID
	for _, link := range kube.Links(s.Model, service) {
		if link.Type == inventory.Selects {
			out = append(out, link.To)
		}
	}
	return out
}

// expiredSecret is the Secret with the certificate expiry it recorded,
// when that moment has passed.
func (s Snapshot) expiredSecret(id inventory.EntityID) *rootcause.ServingCert {
	if id.Kind != kube.KindSecret {
		return nil
	}
	e, ok := s.Model.Entity(id)
	if !ok {
		return nil
	}
	attribute, ok := e.Attribute(kube.AttrCertExpiry)
	if !ok {
		return nil
	}
	notAfter := attribute.Value.AsTime()
	if notAfter.IsZero() || !notAfter.Before(s.Now) {
		return nil
	}
	return &rootcause.ServingCert{Secret: id, Expired: notAfter}
}

// webhookKind reports an admission webhook configuration.
func webhookKind(kind inventory.Kind) bool {
	return kind == kube.KindValidatingHook ||
		kind == kube.KindMutatingWebhook
}
