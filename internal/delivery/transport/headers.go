package transport

import (
	"errors"
	"fmt"
)

// ValidateHeader rejects a header the HTTP client would refuse to send. The
// client refuses it on every attempt, so the error is permanent: retrying
// would only hold the job until the outbox expires. The error names the
// header but never shows its value, which may be a secret.
func ValidateHeader(name, value string) error {
	if name == "" {
		return errors.New("invalid header: the name is empty")
	}
	for i := 0; i < len(name); i++ {
		if !isTokenByte(name[i]) {
			return fmt.Errorf(
				"invalid header %q: the name has a character not "+
					"allowed in a header name", name)
		}
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if (c < ' ' && c != '\t') || c == 0x7f {
			return fmt.Errorf(
				"invalid header %q: the value has a control "+
					"character such as a newline", name)
		}
	}
	return nil
}

// isTokenByte reports whether c may appear in a header name (RFC 9110
// token characters).
func isTokenByte(c byte) bool {
	switch {
	case c >= '0' && c <= '9', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		return true
	}
	switch c {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_',
		'`', '|', '~':
		return true
	}
	return false
}
