package threema

import (
	"net/url"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact Gateway push_simple request for the
// canonical messages. Update with KWATCH_UPDATE_ALL=1 and -update.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewThreema(map[string]interface{}{
		"gatewayId": "*KWATCH1", "secret": "gw-s3cr3t", "to": "ABCD1234",
	}, "dev", rec.Dependencies())
	c.url = rec.URL() + "/send_simple"
	providertest.RunContract(t, providertest.Contract{
		Send: c.SendIncident, Rec: rec,
		Secrets: []string{"gw-s3cr3t"}, LimitBytes: threemaTextLimit,
		Check: checkPush,
	})
}

func checkPush(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if len(reqs) != 1 || reqs[0].Method != "POST" {
		t.Fatalf("requests = %+v", reqs)
	}
	form, err := url.ParseQuery(string(reqs[0].Body))
	if err != nil {
		t.Fatal(err)
	}
	if form.Get("from") != "*KWATCH1" || form.Get("to") != "ABCD1234" ||
		form.Get("secret") != "gw-s3cr3t" || form.Get("text") == "" {
		t.Fatalf("form lacks from, to, secret or text: %v", form)
	}
	if len(form.Get("text")) > threemaTextLimit {
		t.Fatalf("text is %d bytes", len(form.Get("text")))
	}
}
