package sns

import (
	"testing"

	deliveryapi "github.com/abahmed/kwatch/internal/delivery/api"
)

// SNS carries the incident key on every message, so it asks for the
// paging-only announcements too.
func TestSnsReceivesPagingOnly(t *testing.T) {
	var p any = &Sns{}
	receiver, ok := p.(deliveryapi.PagingOnlyReceiver)
	if !ok || !receiver.ReceivesPagingOnly() {
		t.Fatal("SNS must receive paging-only messages")
	}
}
