package redact

import "testing"

func TestRedactPinAndPasscodeKeys(t *testing.T) {
	runTable(t, []struct{ name, in, want string }{
		{"pin equals", "PIN=482913", "PIN=[redacted]"},
		{"pin colon", "pin: 4829", "pin: [redacted]"},
		{"pincode", "pincode=123456", "pincode=[redacted]"},
		{"env prefix", "CARD_PIN=0042", "CARD_PIN=[redacted]"},
		{"json string", `{"pin": "4829"}`, `{"pin": "[redacted]"}`},
		{"passcode word", "passcode=abc", "passcode=[redacted]"},
		{"pin word value", "pin: node-1", "pin: node-1"},
		{"spin", "spin=1234", "spin=1234"},
		{"pin prose", "pin the version", "pin the version"},
	})
}

func TestRedactBareKeyWithLongValue(t *testing.T) {
	runTable(t, []struct{ name, in, want string }{
		{"long number", "key=1234567", "key=[redacted]"},
		{"mixed value", "key=Zx81Qw9Lm2", "key=[redacted]"},
		{"short word", "key=foo", "key=foo"},
		{"plain word", "key=namespace", "key=namespace"},
		{"short number", "key=12345", "key=12345"},
		{"prose colon", "key: 1234567", "key: 1234567"},
		{"dotted key", "tls.key=1234567", "tls.key=1234567"},
		{"inside word", "monkey=1234567", "monkey=1234567"},
	})
}
