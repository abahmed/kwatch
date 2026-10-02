package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// warn records a Warning event about an entity.
func warn(m *inventory.Model, id inventory.EntityID, at time.Time,
	reason, message string,
) {
	m.Apply(inventory.Observation{
		Kind: inventory.Noted, Source: "test", At: at, Entity: id,
		Note: inventory.Note{
			At: at, Reason: reason, Message: message, Count: 1,
			Warning: true,
		},
	})
}
