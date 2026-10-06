package explain

import "testing"

// TestWebhookDenialIsAPolicyRejection: a webhook that answers and
// denies is blamed as a rejection, not as a webhook that is down.
func TestWebhookDenialIsAPolicyRejection(t *testing.T) {
	f := newFixture(t)
	hook, effect := webhookCallCase(f, "admission webhook "+
		`"image.example.com" denied the request: image is not signed`)
	cause := requireCause(t, f.explain(), effect, hook.String())
	if cause.Row != "webhook-rejects" || cause.Mode != ModeWebhookDenied {
		t.Fatalf("cause = %s (%s), want webhook-rejects (%s)",
			cause.Row, cause.Mode, ModeWebhookDenied)
	}
}

// TestWebhookDenialIgnoresFailurePolicy: a denial was answered, so a
// fail-open webhook still rejects.
func TestWebhookDenialIgnoresFailurePolicy(t *testing.T) {
	f := newFixture(t)
	hook := hookCase(f, "audit", "audit.example.com", "Ignore")
	effect := createCase(f, `admission webhook "audit.example.com" `+
		"denied the request: no latest tags")
	requireCause(t, f.explain(), effect, hook.String())
}

// TestWebhookDenialReasonIsNotATimeout: a denial reason that mentions a
// timeout, after another webhook's failed call, belongs to the denier.
func TestWebhookDenialReasonIsNotATimeout(t *testing.T) {
	f := newFixture(t)
	hook, effect := webhookCallCase(f, `failed calling webhook `+
		`"third.example.com": connection refused; admission webhook `+
		`"image.example.com" denied the request: timeout must be set`)
	v := newView(f.snapshot())
	modes := v.admissionModes(hook, effect, LinkAdmits)
	if len(modes) != 1 || modes[0].mode != ModeWebhookDenied {
		t.Fatalf("modes = %+v, want %s", modes, ModeWebhookDenied)
	}
}
