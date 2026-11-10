package incident

// Scope records the delivery scope of an incident's announcement.
type Scope uint8

// Scopes.
const (
	// ScopeUnknown means no announcement was judged yet.
	ScopeUnknown Scope = iota
	// ScopeIn means the announcement was delivered.
	ScopeIn
	// ScopeOut means the announcement was dropped as out of scope.
	ScopeOut
)
