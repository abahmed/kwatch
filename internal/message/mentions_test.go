package message

import "testing"

func TestNeutralizeMentionsBreaksBroadcasts(t *testing.T) {
	got := NeutralizeMentions("ping @channel @All @here me@example.com")
	want := "ping @​channel @​All @​here me@example.com"
	if got != want {
		t.Fatalf("NeutralizeMentions = %q, want %q", got, want)
	}
}
