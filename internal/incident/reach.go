package incident

import "github.com/abahmed/kwatch/internal/inventory"

// MaxReach bounds the user endpoints one decision carries: the dashboard
// table lists as many, a message names two.
const MaxReach = 8

// Reach is one address users reach what an incident affects at, read
// from an Ingress or a route: a host, a path when the rule has a
// specific one, and how many of the endpoints behind it are ready.
type Reach struct {
	Host string
	// Path is empty for the root path.
	Path string
	// Source is the Ingress or route that publishes the address.
	Source inventory.EntityID
	// Ready and Total count the endpoints of the Service behind it.
	Ready, Total int
}

// Address is "host/path", or the bare host for the root path.
func (r Reach) Address() string { return r.Host + r.Path }

// Down reports an address with no ready endpoint left.
func (r Reach) Down() bool { return r.Ready == 0 }
