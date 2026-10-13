package selfcheck

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 13, 10, 0, 0, 0, time.UTC)

func at(minutes int) time.Time {
	return t0.Add(time.Duration(minutes) * time.Minute)
}

func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		in   Evidence
		want Outcome
	}{
		{"cause changed before members recovered",
			Evidence{MembersRecovered: at(10), CauseChanged: at(5)},
			CauseConfirmed},
		{"cause recovered before members recovered",
			Evidence{MembersRecovered: at(10), CauseRecovered: at(9)},
			CauseConfirmed},
		{"cause changed after members recovered is not the fix",
			Evidence{MembersRecovered: at(10), CauseChanged: at(11)},
			NothingChanged},
		{"only something else changed",
			Evidence{MembersRecovered: at(10), OtherChanged: at(4)},
			OtherChanged},
		{"cause and other changed: the cause counts",
			Evidence{MembersRecovered: at(10), CauseChanged: at(6),
				OtherChanged: at(3)},
			CauseConfirmed},
		{"late cause change and an earlier other change",
			Evidence{MembersRecovered: at(10), CauseChanged: at(12),
				OtherChanged: at(3)},
			OtherChanged},
		{"nothing changed", Evidence{MembersRecovered: at(10)},
			NothingChanged},
		{"cause change at the same instant counts",
			Evidence{MembersRecovered: at(10), CauseChanged: at(10)},
			CauseConfirmed},
	}
	for _, c := range cases {
		if got := Classify(c.in); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
