package ifttt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/delivery/transport"
)

func TestIftttBodyErrorIsPermanent(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"errors":[{"message":"bad key"}]}`))
		}))
	defer s.Close()
	c := NewIfttt(map[string]interface{}{"key": "abc"},
		testAppConfig(), testDeps)
	c.url = s.URL

	err := c.SendMessage(context.Background(), "test")
	assert.Error(t, err)
	assert.True(t, transport.IsPermanent(err))
}
