package rootcause

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// ServingCert is a TLS Secret the pods behind a webhook mount whose
// certificate ended in the past. Only the Secret's name and the end of
// the certificate are kept, never any of its content.
type ServingCert struct {
	Secret  inventory.EntityID
	Expired time.Time
}
