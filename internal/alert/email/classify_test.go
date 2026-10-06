package email

import (
	"errors"
	"fmt"
	"net/textproto"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/transport"
)

func TestSMTPFiveHundredRepliesArePermanent(t *testing.T) {
	for _, code := range []int{535, 550, 554} {
		err := classifySMTP(&textproto.Error{Code: code, Msg: "no"})
		assert.True(t, transport.IsPermanent(err), "code %d", code)
	}
}

func TestSMTPFourHundredRepliesAndDropsRetry(t *testing.T) {
	for _, err := range []error{
		&textproto.Error{Code: 421, Msg: "try later"},
		&textproto.Error{Code: 451, Msg: "local error"},
		&textproto.Error{Code: 552, Msg: "mailbox full"},
		errors.New("connection reset"),
		fmt.Errorf("wrapped: %w", &textproto.Error{Code: 452}),
	} {
		assert.False(t, transport.IsPermanent(classifySMTP(err)), "%v", err)
	}
}

func TestSMTPWrappedFiveHundredIsPermanent(t *testing.T) {
	err := fmt.Errorf("rcpt: %w", &textproto.Error{Code: 550})
	assert.True(t, transport.IsPermanent(classifySMTP(err)))
}
