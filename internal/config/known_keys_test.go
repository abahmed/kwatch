package config

import "testing"

func TestKnownKeyFollowsTheConfigStruct(t *testing.T) {
	for _, tc := range []struct {
		path []string
		want bool
	}{
		{[]string{"license"}, true},
		{[]string{"license", "key"}, true},
		{[]string{"watch", "secrets"}, true},
		{[]string{"healthCheck", "statusEndpoint"}, true},
		{[]string{"alert", "slack", "webhook"}, true},
		{[]string{"silences", "namespaces"}, true},
		{[]string{"app", "proxyURL"}, true},
		{[]string{"license", "keyy"}, false},
		{[]string{"workers"}, false},
		{[]string{"namespaces", "x"}, false},
		{[]string{"Runtime"}, false},
	} {
		if got := KnownKey(tc.path...); got != tc.want {
			t.Errorf("KnownKey(%v) = %v, want %v", tc.path, got, tc.want)
		}
	}
}
