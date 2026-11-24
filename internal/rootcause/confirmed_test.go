package rootcause

import "testing"

// A cause at likely or high is always named. A possible one is named
// only alone and corroborated: by a counterfactual check or a proof
// for it.
func TestConfirmedNeedsCorroborationBelowLikely(t *testing.T) {
	support := []Proof{{Text: "3 of 3 failing pods run there",
		Supports: true}}
	against := []Proof{{Text: "it looks healthy", Supports: false}}
	rival := &CauseRecord{Score: 0.68}
	tests := []struct {
		name  string
		cause *CauseRecord
		want  bool
	}{
		{"none", nil, false},
		{"high", &CauseRecord{Score: High}, true},
		{"likely", &CauseRecord{Score: Likely}, true},
		{"possible alone", &CauseRecord{Score: 0.46}, false},
		{"possible with a proof", &CauseRecord{Score: 0.46,
			Proof: support}, true},
		{"possible with a check", &CauseRecord{Score: 0.46,
			Checked: []string{"node n3 is healthy for 12 other pods"}},
			true},
		{"possible with only a proof against", &CauseRecord{Score: 0.46,
			Proof: against}, false},
		{"close rival", &CauseRecord{Score: 0.69, Proof: support,
			Rival: rival}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Confirmed(tt.cause); got != tt.want {
				t.Errorf("Confirmed = %v, want %v", got, tt.want)
			}
		})
	}
}
