package detection

import (
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Place is the exact setting a failure comes from: the object that
// holds it, the container inside that object (when the setting belongs
// to one), the field path and the value the field has now.
//
// A Place is built from the object's own attributes, never from
// message text. A Secret's value is never put in Value: for a Secret
// the Value is a name, or empty.
type Place struct {
	// Object is the object that holds the setting. A finding's Place
	// may leave it zero: the incident's root is the object then.
	Object inventory.EntityID `json:",omitempty"`
	// Container is the container name, empty for an object-level field.
	Container string `json:",omitempty"`
	// Path is the field path inside the object or the container:
	// "resources.limits.memory", "livenessProbe.periodSeconds".
	Path string
	// Value is what the field holds now, as a person writes it.
	Value string `json:",omitempty"`
}

// Valid reports whether the place names a field.
func (p Place) Valid() bool {
	return strings.TrimSpace(p.Path) != ""
}

// MemoryText writes a byte count as a Kubernetes memory quantity:
// "256Mi", "1.5Gi".
func MemoryText(bytes float64) string {
	const mebi, gibi = 1 << 20, 1 << 30
	if bytes >= gibi {
		value := strconv.FormatFloat(bytes/gibi, 'f', 1, 64)
		return strings.TrimSuffix(value, ".0") + "Gi"
	}
	return strconv.FormatFloat(bytes/mebi, 'f', 0, 64) + "Mi"
}

// CPUText writes millicores as a Kubernetes CPU quantity: "200m", "2".
func CPUText(milli float64) string {
	if milli >= 1000 && int64(milli)%1000 == 0 {
		return strconv.FormatInt(int64(milli)/1000, 10)
	}
	return strconv.FormatInt(int64(milli), 10) + "m"
}
