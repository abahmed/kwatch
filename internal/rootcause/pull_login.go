package rootcause

import "time"

// Login states: what kwatch knows of the Secret a failing pod pulls
// with.
const (
	// LoginFound: the Secret exists. Type and Changed describe it.
	LoginFound = "found"
	// LoginMissing: the pod names the Secret and it does not exist.
	LoginMissing = "missing"
	// LoginUnwatched: the Secret is named, but kwatch does not list
	// Secrets, so it cannot tell whether it exists.
	LoginUnwatched = "unwatched"
	// LoginNone: the pod names no image pull Secret at all.
	LoginNone = "none"
)

// PullLogin is one registry login the failing pods pull with. It holds
// the Secret's name, type and last change, never anything inside it.
type PullLogin struct {
	Namespace string
	// Secret is empty for State LoginNone.
	Secret string
	State  string
	// Type is the Secret's type, such as "kubernetes.io/dockerconfigjson".
	Type string `json:",omitempty"`
	// Changed is when the Secret last changed, when known.
	Changed time.Time `json:",omitempty"`
}
