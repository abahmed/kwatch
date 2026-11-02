package incident

// String names the state for diagnostics.
func (s State) String() string {
	switch s {
	case Settling:
		return "settling"
	case Open:
		return "open"
	case Recovering:
		return "recovering"
	case Flapping:
		return "flapping"
	case Resolved:
		return "resolved"
	}
	return "unknown"
}

// String names the tier for diagnostics.
func (t Tier) String() string {
	switch t {
	case Silent:
		return "silent"
	case Digest:
		return "digest"
	case Notify:
		return "notify"
	case Page:
		return "page"
	}
	return "unknown"
}
