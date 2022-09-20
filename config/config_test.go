package config

import (
	"reflect"
	"testing"
)

func TestGetAllowForbidSlices(t *testing.T) {
	testCases := []map[string][]string{
		{
			"input":  {},
			"allow":  {},
			"forbid": {},
		},
		{
			"input":  {"hello", "!world"},
			"allow":  {"hello"},
			"forbid": {"world"},
		},
		{
			"input":  {"hello"},
			"allow":  {"hello"},
			"forbid": {},
		},
		{
			"input":  {"!hello"},
			"allow":  {},
			"forbid": {"hello"},
		},
	}

	for _, tc := range testCases {
		actualAllow, actualForbid := getAllowForbidSlices(tc["input"])
		if !reflect.DeepEqual(actualAllow, tc["allow"]) {
			t.Fatalf(
				"getAllowForbidSlices allow returned %v expected %v",
				actualAllow,
				tc["allow"])
		}

		if !reflect.DeepEqual(actualForbid, tc["forbid"]) {
			t.Fatalf(
				"getAllowForbidSlices forbid returned %v expected %v",
				actualForbid,
				tc["forbid"])
		}
	}
}
