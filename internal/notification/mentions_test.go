package notification

import "testing"

func TestNeutralizeMentionsBreaksBroadcasts(t *testing.T) {
	got := NeutralizeMentions("ping @channel @All @here me@example.com")
	want := "ping @​channel @​All @​here me@example.com"
	if got != want {
		t.Fatalf("NeutralizeMentions = %q, want %q", got, want)
	}
}

func TestNeutralizeProviderMentions(t *testing.T) {
	const zw = "​"
	tests := []struct {
		name       string
		neutralize func(string) string
		in, want   string
	}{
		{"google chat all", NeutralizeGoogleChatMentions,
			"hi <users/all>", "hi <" + zw + "users/all>"},
		{"google chat person", NeutralizeGoogleChatMentions,
			"<users/1234>", "<" + zw + "users/1234>"},
		{"google chat plain", NeutralizeGoogleChatMentions,
			"@here", "@" + zw + "here"},
		{"webex all", NeutralizeWebexMentions,
			"<@all> look", "<" + zw + "@" + zw + "all> look"},
		{"webex email", NeutralizeWebexMentions,
			"<@personEmail:a@b.c|A>", "<" + zw + "@personEmail:a@b.c|A>"},
		{"zulip all", NeutralizeZulipMentions,
			"@**all** and @**everyone**",
			"@" + zw + "**all** and @" + zw + "**everyone**"},
		{"zulip silent", NeutralizeZulipMentions,
			"@_**Ann**", "@" + zw + "_**Ann**"},
		{"zulip bold text kept", NeutralizeZulipMentions,
			"**bold** a@b.c", "**bold** a@b.c"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.neutralize(tc.in); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNeutralizeMentionsBreaksRoom(t *testing.T) {
	got := NeutralizeMentions("ping @room")
	if got != "ping @​room" {
		t.Fatalf("NeutralizeMentions = %q", got)
	}
}
