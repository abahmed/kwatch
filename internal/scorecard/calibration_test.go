package scorecard

import "testing"

// casesAt returns n cases scored at confidence, the first wrong ones
// wrong and the rest right.
func casesAt(confidence float64, n, wrong int) []Case {
	out := make([]Case, n)
	for i := range out {
		out[i] = Case{Confidence: confidence, Correct: i >= wrong}
	}
	return out
}

func joinCases(groups ...[]Case) []Case {
	var out []Case
	for _, group := range groups {
		out = append(out, group...)
	}
	return out
}

func TestCalibratedHighBoundaries(t *testing.T) {
	cases := map[string]struct {
		cases []Case
		want  float64
		ok    bool
	}{
		"a full, accurate range moves the boundary down": {
			joinCases(casesAt(1, 20, 1), casesAt(0.7, 13, 1),
				casesAt(0.6, 4, 0)),
			0.7, true,
		},
		"too few cases below keep it where it is": {
			joinCases(casesAt(0.9, 12, 0), casesAt(0.65, 9, 0)),
			0.9, true,
		},
		"an inaccurate range stops the walk": {
			joinCases(casesAt(0.95, 10, 0), casesAt(0.8, 10, 5),
				casesAt(0.7, 30, 0)),
			0.95, true,
		},
		"a score just under an edge belongs to the range below": {
			joinCases(casesAt(0.75, 10, 0), casesAt(0.699, 10, 0)),
			0.65, true,
		},
		"cases under the likely floor are ignored": {
			joinCases(casesAt(0.9, 10, 0), casesAt(0.3, 40, 0)),
			0.9, true,
		},
		"nothing qualifies": {
			joinCases(casesAt(0.9, 5, 0), casesAt(0.6, 5, 4)), 0, false,
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got, ok := CalibratedHigh(c.cases)
			if got != c.want || ok != c.ok {
				t.Fatalf("CalibratedHigh = %v, %v; want %v, %v", got, ok,
					c.want, c.ok)
			}
		})
	}
}
