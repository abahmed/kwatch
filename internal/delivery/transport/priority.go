package transport

// IntSetting reads an integer provider setting. YAML and JSON decoders
// produce int, int64 or float64; any other type, or a missing key, is
// reported as unset so callers can tell "not configured" from zero.
func IntSetting(value interface{}) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}

// IntSettingPtr is IntSetting returning nil when the setting is unset.
func IntSettingPtr(value interface{}) *int {
	v, ok := IntSetting(value)
	if !ok {
		return nil
	}
	return &v
}

// ResolvePriority picks the priority of a resolve so it is never louder
// than the incident it closes. An explicit override wins. Otherwise the
// configured priority is capped at the provider's normal level, and an
// unset priority gives the normal level itself.
func ResolvePriority(configured, override *int, normal int) int {
	if override != nil {
		return *override
	}
	if configured == nil || *configured > normal {
		return normal
	}
	return *configured
}
