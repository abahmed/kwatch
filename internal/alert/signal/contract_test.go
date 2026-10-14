package signal

import (
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact signal-cli REST send request for the
// canonical messages. Update with KWATCH_UPDATE_ALL=1 and -update.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewSignal(map[string]interface{}{
		"number": "+12025550199", "to": "+12025550100", "url": rec.URL(),
	}, "dev", rec.Dependencies())
	providertest.RunContract(t, providertest.Contract{
		Send: c.SendIncident, Rec: rec, Check: checkSend,
	})
}

func checkSend(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if len(reqs) != 1 || reqs[0].Method != "POST" ||
		reqs[0].Path != "/v2/send" {
		t.Fatalf("requests = %+v", reqs)
	}
	b := reqs[0].JSON(t)
	to, _ := b["recipients"].([]any)
	if b["message"] == "" || b["number"] != "+12025550199" ||
		len(to) != 1 || to[0] != "+12025550100" {
		t.Fatalf("send lacks message, number or recipients: %v", b)
	}
}
