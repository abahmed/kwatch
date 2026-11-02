package transport

import "testing"

func intp(v int) *int { return &v }

func TestResolvePriority(t *testing.T) {
	cases := []struct {
		name       string
		configured *int
		override   *int
		normal     int
		want       int
	}{
		{"unset gives normal", nil, nil, 3, 3},
		{"high is capped", intp(5), nil, 3, 3},
		{"equal stays", intp(3), nil, 3, 3},
		{"low stays low", intp(1), nil, 3, 1},
		{"negative stays", intp(-2), nil, 0, -2},
		{"override beats cap", intp(5), intp(4), 3, 4},
		{"override beats low", intp(1), intp(2), 3, 2},
		{"override without config", nil, intp(1), 3, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolvePriority(tc.configured, tc.override, tc.normal)
			if got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestIntSetting(t *testing.T) {
	for _, v := range []interface{}{4, int64(4), 4.0} {
		if got, ok := IntSetting(v); !ok || got != 4 {
			t.Fatalf("IntSetting(%T) = %d, %v", v, got, ok)
		}
	}
	for _, v := range []interface{}{nil, "4", true} {
		if _, ok := IntSetting(v); ok {
			t.Fatalf("IntSetting(%v) reported set", v)
		}
		if IntSettingPtr(v) != nil {
			t.Fatalf("IntSettingPtr(%v) not nil", v)
		}
	}
	if p := IntSettingPtr(0); p == nil || *p != 0 {
		t.Fatal("IntSettingPtr(0) must be set to zero")
	}
}
