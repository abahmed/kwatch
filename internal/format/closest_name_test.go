package format

import "testing"

func TestClosestNameAllowsFewEdits(t *testing.T) {
	cases := []struct {
		missing string
		names   []string
		want    string
	}{
		{"paymnts", []string{"payments", "orders"}, "payments"},
		{"openroute-eg-svc", []string{"openroute-svc", "web"}, ""},
		{"openroute-svc", []string{"openroute-svcs"}, "openroute-svcs"},
		{"pay", []string{"paw"}, ""},
		{"payment", []string{"payments", "paymens"}, ""},
	}
	for _, c := range cases {
		if got := ClosestName(c.missing, c.names); got != c.want {
			t.Errorf("ClosestName(%q) = %q, want %q", c.missing, got,
				c.want)
		}
	}
}
