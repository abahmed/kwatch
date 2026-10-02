package matrix

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

var testDeps = transport.Dependencies{
	HTTPClient: http.DefaultClient,
	Clock:      clock.RealClock{},
}

func testAppConfig() string {
	return "dev"
}

func TestEmptyConfig(t *testing.T) {
	assert := assert.New(t)

	c := NewMatrix(map[string]interface{}{}, testAppConfig(), testDeps)
	assert.Nil(c)
}

func TestInvalidConfig(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"homeServer": "https://matrix-client.matrix.org",
	}
	c := NewMatrix(configMap, testAppConfig(), testDeps)
	assert.Nil(c)

	configMap = map[string]interface{}{
		"homeServer":  "https://matrix-client.matrix.org",
		"accessToken": "testToken",
	}
	c = NewMatrix(configMap, testAppConfig(), testDeps)
	assert.Nil(c)

	configMap = map[string]interface{}{
		"homeServer":     "https://matrix-client.matrix.org",
		"accessToken":    "testToken",
		"internalRoomId": "",
	}
	c = NewMatrix(configMap, testAppConfig(), testDeps)
	assert.Nil(c)

}

func TestMatrix(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"homeServer":     "https://matrix-client.matrix.org",
		"accessToken":    "testToken",
		"internalRoomId": "room1",
	}
	c := NewMatrix(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)

	assert.Equal(c.Name(), "Matrix")
}

func TestSendMessage(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"isOk": true}`))
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"homeServer":     s.URL,
		"accessToken":    "testToken",
		"internalRoomId": "room1",
	}
	c := NewMatrix(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)

	assert.Nil(c.SendMessage(context.Background(), "test"))
}

func TestSendMessageError(t *testing.T) {
	assert := assert.New(t)

	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))

	defer s.Close()

	configMap := map[string]interface{}{
		"homeServer":     s.URL,
		"accessToken":    "testToken",
		"internalRoomId": "room1",
	}
	c := NewMatrix(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)

	assert.NotNil(c.SendMessage(context.Background(), "test"))
}

func TestInvaildHttpRequest(t *testing.T) {
	assert := assert.New(t)

	configMap := map[string]interface{}{
		"homeServer":     "https://example.test/hook",
		"accessToken":    "testToken",
		"internalRoomId": "room1",
	}
	c := NewMatrix(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)
	c.homeServer = "h ttp://localhost"

	assert.NotNil(c.SendMessage(context.Background(), "test"))

	configMap = map[string]interface{}{
		"homeServer":     "http://localhost:132323",
		"accessToken":    "testToken",
		"internalRoomId": "room1",
	}
	c = NewMatrix(configMap, testAppConfig(), testDeps)
	assert.NotNil(c)

	assert.NotNil(c.SendMessage(context.Background(), "test"))
}

func captureTxnPaths(t *testing.T) (*Matrix, *[]string) {
	t.Helper()
	var paths []string
	s := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			paths = append(paths, r.URL.EscapedPath())
			_, _ = w.Write([]byte(`{"event_id": "$e"}`))
		}))
	t.Cleanup(s.Close)
	c := NewMatrix(map[string]interface{}{
		"homeServer":     s.URL,
		"accessToken":    "testToken",
		"internalRoomId": "room1",
	}, testAppConfig(), testDeps)
	return c, &paths
}

func TestMatrixTransactionIDStableAcrossRetries(t *testing.T) {
	c, paths := captureTxnPaths(t)
	ctx := context.Background()
	m := providertest.Announce()
	assert.NoError(t, c.SendIncident(ctx, m))
	assert.NoError(t, c.SendIncident(ctx, m))
	assert.NoError(t, c.SendMessage(ctx, "hello"))
	assert.NoError(t, c.SendMessage(ctx, "hello"))
	assert.Equal(t, (*paths)[0], (*paths)[1])
	assert.NotEqual(t, (*paths)[2], (*paths)[3])
	assert.NotEqual(t, (*paths)[0], (*paths)[2])
}

func TestMatrixTransactionIDDiffersPerRevision(t *testing.T) {
	c, paths := captureTxnPaths(t)
	ctx := context.Background()
	for _, tc := range providertest.Lifecycle() {
		assert.NoError(t, c.SendIncident(ctx, tc.Message))
	}
	assert.NotEqual(t, (*paths)[0], (*paths)[1])
	assert.NotEqual(t, (*paths)[1], (*paths)[2])
}

func TestMatrixPlainMessagesGetUniqueTransactionIDs(t *testing.T) {
	c, paths := captureTxnPaths(t)
	ctx := context.Background()
	assert.NoError(t, c.SendMessage(ctx, "digest"))
	assert.NoError(t, c.SendMessage(ctx, "digest"))
	assert.NotEqual(t, (*paths)[0], (*paths)[1])
}
