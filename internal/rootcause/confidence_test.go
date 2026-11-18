package rootcause

import "testing"

func TestCertaintyOfHedgesACauseWithARival(t *testing.T) {
	cases := []struct {
		score float64
		rival bool
		want  Level
	}{
		{0.9, false, LevelHigh}, {High, false, LevelHigh},
		{0.6, false, LevelLikely}, {Likely, false, LevelLikely},
		{0.3, false, LevelPossible},
		{0.9, true, LevelPossible}, {0.6, true, LevelPossible},
		{RivalCap, true, LevelPossible},
	}
	for _, c := range cases {
		cause := &CauseRecord{Score: c.score}
		if c.rival {
			cause.Rival = &CauseRecord{}
		}
		if got := CertaintyOf(cause); got != c.want {
			t.Errorf("score %v rival %v: got %q, want %q",
				c.score, c.rival, got, c.want)
		}
	}
	if got := CertaintyOf(nil); got != LevelNone {
		t.Errorf("no cause: got %q, want none", got)
	}
}

func TestLevelOfIgnoresRivals(t *testing.T) {
	if LevelOf(0.9) != LevelHigh || LevelOf(0.55) != LevelLikely ||
		LevelOf(0.1) != LevelPossible {
		t.Error("LevelOf does not follow High and Likely")
	}
}
