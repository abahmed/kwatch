package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// webhookCallCase is a webhook configuration with ready endpoints whose
// calls fail as text says, blocking two ReplicaSets.
func webhookCallCase(f *fixture, text string) (
	inventory.EntityID, inventory.EntityID,
) {
	hook := inventory.CoreID(kube.KindValidatingHook, "", "image-policy")
	f.observe(hook, map[string]inventory.Value{
		kube.AttrWebhookNames: inventory.Text(
			"other.example.com,image.example.com"),
		kube.AttrFailurePolicy: inventory.Text("Fail,Fail"),
	})
	return hook, createCase(f, text)
}

func TestWebhookCallFailures(t *testing.T) {
	const prefix = "Error creating: Internal error occurred: failed " +
		"calling webhook \"image.example.com\": failed to call webhook: "
	cases := map[string]struct {
		text string
		mode detection.Mode
	}{
		"deadline": {prefix + "Post \"https://p.svc:443\": context " +
			"deadline exceeded", ModeWebhookTimeout},
		"client timeout": {prefix + "net/http: request canceled " +
			"(Client.Timeout exceeded while awaiting headers)",
			ModeWebhookTimeout},
		"refused": {prefix + "dial tcp 10.0.0.4:443: connect: " +
			"connection refused", ModeWebhookCallFailed},
		// The API server puts "?timeout=10s" in every webhook URL; that
		// is the configured limit, not a sign the call timed out.
		"refused with timeout query": {prefix + "Post \"https://" +
			"p.svc:443/validate?timeout=10s\": dial tcp 10.0.0.4:443: " +
			"connect: connection refused", ModeWebhookCallFailed},
		"no endpoints": {prefix + "Post \"https://p.svc:443/validate" +
			"?timeout=10s\": no endpoints available for service " +
			"\"p\"", ModeWebhookCallFailed},
		"real timeout with query": {prefix + "Post \"https://p.svc:443" +
			"/validate?timeout=10s\": context deadline exceeded",
			ModeWebhookTimeout},
		"unknown authority": {prefix + "Post \"https://p.svc:443/validate" +
			"?timeout=10s\": x509: certificate signed by unknown " +
			"authority", ModeWebhookTLS},
		"expired certificate": {prefix + "Post \"https://p.svc:443" +
			"\": tls: failed to verify certificate: x509: certificate " +
			"has expired or is not yet valid", ModeWebhookTLS},
		"wrong name": {prefix + "Post \"https://p.svc:443\": x509: " +
			"certificate is valid for a.svc, not p.svc", ModeWebhookTLS},
		// A handshake that runs out of time is a timeout, not a bad
		// certificate.
		"handshake timeout": {prefix + "Post \"https://p.svc:443\": " +
			"net/http: TLS handshake timeout", ModeWebhookTimeout},
		"i/o timeout with query": {prefix + "Post \"https://p.svc:443" +
			"/validate?timeout=10s\": dial tcp 10.0.0.4:443: i/o " +
			"timeout", ModeWebhookTimeout},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			hook, effect := webhookCallCase(f, c.text)
			cause := requireCause(t, f.explain(), effect, hook.String())
			if cause.Row != "webhook-rejects" || cause.Mode != c.mode {
				t.Fatalf("cause = %s (%s), want webhook-rejects (%s)",
					cause.Row, cause.Mode, c.mode)
			}
		})
	}
}

// TestWebhookCallNamesTheWebhook: a failed call blames only the
// configuration whose webhook the error names, and another webhook's
// denial is not this one's.
func TestWebhookCallNamesTheWebhook(t *testing.T) {
	cases := map[string]string{
		"another webhook": "failed calling webhook \"x.example.com\": " +
			"context deadline exceeded",
		"another denial": "admission webhook \"x.example.com\" denied " +
			"the request: image is not signed",
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			hook, effect := webhookCallCase(f, text)
			v := newView(f.snapshot())
			if modes := v.admissionModes(hook, effect, LinkAdmits); len(
				modes) != 0 {
				t.Fatalf("modes = %+v, want none", modes)
			}
		})
	}
}

// hookCase observes a webhook configuration with the given webhook
// names and failure policies, failing with NoEndpoints.
func hookCase(f *fixture, name, hooks, policies string) inventory.EntityID {
	hook := inventory.CoreID(kube.KindValidatingHook, "", name)
	attrs := map[string]inventory.Value{}
	if hooks != "" {
		attrs[kube.AttrWebhookNames] = inventory.Text(hooks)
		attrs[kube.AttrFailurePolicy] = inventory.Text(policies)
	}
	f.observe(hook, attrs)
	f.fail(hook, detection.ModeNoEndpoints, failingH, 1, "")
	return hook
}

// requireNotBlamed fails when root is the stated cause of effect.
func requireNotBlamed(
	t *testing.T, e Explanation, effect, root inventory.EntityID,
) {
	t.Helper()
	if c, ok := e.CauseOf(effect); ok && c.Root == root {
		t.Fatalf("%s blamed on %s (%s)", effect, root, c.Row)
	}
}

// TestExplainWebhookBlamedOnlyWhenNamed: a create denied by one
// webhook is not blamed on another unhealthy webhook configuration.
func TestExplainWebhookBlamedOnlyWhenNamed(t *testing.T) {
	f := newFixture(t)
	stale := hookCase(f, "stale-hook", "", "")
	effect := createCase(f, `admission webhook "policy.corp.example" `+
		`denied the request: image not allowed`)
	requireNotBlamed(t, f.explain(), effect, stale)
}

// TestExplainNamedWebhookIsBlamed: the configuration that holds the
// named webhook is the root.
func TestExplainNamedWebhookIsBlamed(t *testing.T) {
	f := newFixture(t)
	hookCase(f, "stale-hook", "other.example.com", "Fail")
	hook := hookCase(f, "policy", "policy.corp.example", "Fail")
	effect := createCase(f, `failed calling webhook `+
		`"policy.corp.example": no endpoints available for service`)
	requireCause(t, f.explain(), effect, hook.String())
}

// TestExplainIgnoredWebhookIsNotARejectingRoot: a webhook that fails
// open (failurePolicy Ignore) cannot reject a create by failing.
func TestExplainIgnoredWebhookIsNotARejectingRoot(t *testing.T) {
	cases := map[string]string{
		"named": `failed calling webhook "audit.example.com": ` +
			`no endpoints available for service`,
		"unnamed": "Internal error occurred: webhook call failed",
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			hook := hookCase(f, "audit", "audit.example.com", "Ignore")
			effect := createCase(f, text)
			requireNotBlamed(t, f.explain(), effect, hook)
		})
	}
}

// TestWebhookCallReadsItsOwnSegment: when one error names two failed
// calls, the class of each webhook's failure is read from its own
// part of the text.
func TestWebhookCallReadsItsOwnSegment(t *testing.T) {
	f := newFixture(t)
	hook := hookCase(f, "image-policy", "image.example.com", "Fail")
	effect := createCase(f, `failed calling webhook "other.example.com"`+
		`: context deadline exceeded; failed calling webhook `+
		`"image.example.com": dial tcp 10.0.0.4:443: connect: `+
		`connection refused`)
	v := newView(f.snapshot())
	modes := v.admissionModes(hook, effect, LinkAdmits)
	if len(modes) != 1 || modes[0].mode != ModeWebhookCallFailed {
		t.Fatalf("modes = %+v, want %s", modes, ModeWebhookCallFailed)
	}
}

// quotaCase adds exhausted quotas to the shop namespace.
func quotaCase(f *fixture, names ...string) []inventory.EntityID {
	namespace := inventory.CoreID(kube.KindNamespace, "", "shop")
	var out []inventory.EntityID
	for _, name := range names {
		quota := inventory.CoreID(kube.KindQuota, "shop", name)
		f.add(quota)
		f.relate(quota, inventory.Constrains, namespace)
		f.fail(quota, detection.ModeQuotaExhausted, degradedH, 1, "")
		out = append(out, quota)
	}
	return out
}

// TestExplainQuotaMustBeTheNamedOne: a create refused by one quota is
// not blamed on another exhausted quota of the namespace.
func TestExplainQuotaMustBeTheNamedOne(t *testing.T) {
	f := newFixture(t)
	quotas := quotaCase(f, "compute", "objects")
	effect := createCase(f, `pods "api-1-x" is forbidden: exceeded `+
		`quota: objects, requested: pods=1, used: pods=20`)
	v := newView(f.snapshot())
	cs := v.candidates(failures(f.findings))
	if c, ok := cs.byID[quotas[0]]; ok && c.covers[effect].chain != nil {
		t.Fatalf("the other quota explains %s", effect)
	}
	requireCause(t, f.explain(), effect, quotas[1].String())
}

// TestExplainQuotaNeedsAQuotaError: the word "quota" alone is not a
// create refused by a quota.
func TestExplainQuotaNeedsAQuotaError(t *testing.T) {
	f := newFixture(t)
	quotas := quotaCase(f, "compute")
	effect := createCase(f, "error syncing quota tracker: timeout")
	requireNotBlamed(t, f.explain(), effect, quotas[0])
}
