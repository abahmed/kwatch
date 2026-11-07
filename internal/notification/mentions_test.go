package notification

import "testing"

func TestNeutralizeMentionsBreaksBroadcasts(t *testing.T) {
	got := NeutralizeMentions("ping @channel @All @here me@example.com")
	want := "ping @\u200bchannel @\u200bAll @\u200bhere me@example.com"
	if got != want {
		t.Fatalf("NeutralizeMentions = %q, want %q", got, want)
	}
}

func TestNeutralizeProviderMentions(t *testing.T) {
	const zw = "\u200b"
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
		{"telegram username", NeutralizeTelegramMentions,
			"ping @alice_ops now", "ping @" + zw + "alice_ops now"},
		{"telegram at start and broadcast", NeutralizeTelegramMentions,
			"@bobby123 @channel", "@" + zw + "bobby123 @" + zw + "channel"},
		{"telegram email kept", NeutralizeTelegramMentions,
			"mail me@example.com", "mail me@example.com"},
		{"telegram too short to be a name", NeutralizeTelegramMentions,
			"at @ab ok", "at @ab ok"},
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
	if got != "ping @\u200broom" {
		t.Fatalf("NeutralizeMentions = %q", got)
	}
}

func TestNeutralizeMentionsBreaksSlackSpecials(t *testing.T) {
	got := NeutralizeMentions("<!here> <!channel> <!subteam^S1> a <!-- b")
	want := "<​!here> <​!channel> <​!subteam^S1> a <!-- b"
	if got != want {
		t.Fatalf("NeutralizeMentions = %q, want %q", got, want)
	}
}

// User, bot and channel mentions of the Slack syntax are broken too, for
// the webhooks that expand them without Slack's own escaping.
func TestNeutralizeMentionsBreaksUserAndChannelRefs(t *testing.T) {
	const zw = "​"
	got := NeutralizeMentions("<@U123> <@W456> <#C1|general> <!subteam^S1>")
	want := "<" + zw + "@U123> <" + zw + "@W456> <" + zw +
		"#C1|general> <" + zw + "!subteam^S1>"
	if got != want {
		t.Fatalf("NeutralizeMentions = %q, want %q", got, want)
	}
	if got := NeutralizeMentions("a <#1> <!-- x -->"); got != "a <"+zw+
		"#1> <!-- x -->" {
		t.Fatalf("NeutralizeMentions = %q", got)
	}
}

// An @ inside a word is an e-mail address, a URL's user part or a DSN,
// never a mention, and must reach the reader byte for byte.
func TestNeutralizeMentionsKeepsAddressesWhole(t *testing.T) {
	for _, in := range []string{
		"ops@all-hands.io",
		"postgres://app@everyone-db:5432/x",
		"git@here.example.com",
		"me@channel.dev",
	} {
		if got := NeutralizeMentions(in); got != in {
			t.Errorf("NeutralizeMentions(%q) = %q", in, got)
		}
		if got := NeutralizeUserMentions(in); got != in {
			t.Errorf("NeutralizeUserMentions(%q) = %q", in, got)
		}
	}
}

func TestNeutralizeUserMentionsBreaksEveryName(t *testing.T) {
	const zw = "​"
	tests := []struct{ in, want string }{
		{"paged @alice and @devops-team",
			"paged @" + zw + "alice and @" + zw + "devops-team"},
		{"@here now", "@" + zw + "here now"},
		{"(@bob)", "(@" + zw + "bob)"},
		{"<!channel> <@U1>",
			"<" + zw + "!channel> <" + zw + "@" + zw + "U1>"},
		{"mail ops@all-hands.io", "mail ops@all-hands.io"},
		{"lone @ sign", "lone @ sign"},
	}
	for _, tc := range tests {
		if got := NeutralizeUserMentions(tc.in); got != tc.want {
			t.Errorf("NeutralizeUserMentions(%q) = %q, want %q",
				tc.in, got, tc.want)
		}
	}
}
