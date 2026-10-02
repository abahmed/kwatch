package slack

import (
	"context"
	"errors"
	"testing"

	slackClient "github.com/slack-go/slack"
	"github.com/stretchr/testify/require"
)

func TestPostWithThreadFallbackReturnsThreadTSOnSuccess(t *testing.T) {
	blocks := &slackClient.Blocks{}
	post := func(
		_ context.Context, _ *slackClient.Blocks, _ string,
	) (string, error) {
		return "999.888", nil
	}
	ts, err := postWithThreadFallback(context.Background(), post,
		blocks, "123.456")
	require.NoError(t, err)
	require.Equal(t, "123.456", ts)
}

func TestPostWithThreadFallbackReturnsErrorOnNonStaleThreadError(t *testing.T) {
	blocks := &slackClient.Blocks{}
	expectedErr := errors.New("some other error")
	post := func(
		_ context.Context, _ *slackClient.Blocks, _ string,
	) (string, error) {
		return "", expectedErr
	}
	ts, err := postWithThreadFallback(context.Background(), post,
		blocks, "123.456")
	require.Equal(t, expectedErr, err)
	require.Equal(t, "123.456", ts)
}

func TestPostWithThreadFallbackRetriesToTopLevelOnStaleThread(t *testing.T) {
	blocks := &slackClient.Blocks{}
	calls := 0
	post := func(
		_ context.Context, _ *slackClient.Blocks, threadTS string,
	) (string, error) {
		calls++
		if calls == 1 {
			return "", errors.New("error: invalid_thread_ts")
		}
		if threadTS == "" {
			return "new-ts", nil
		}
		return "", errors.New("unexpected threadTS in fallback")
	}
	ts, err := postWithThreadFallback(context.Background(), post,
		blocks, "old-ts")
	require.NoError(t, err)
	require.Equal(t, "new-ts", ts)
	require.Equal(t, 2, calls)
}

func TestPostWithThreadFallbackDoesNotRetryWhenThreadTSEmpty(t *testing.T) {
	blocks := &slackClient.Blocks{}
	calls := 0
	post := func(
		_ context.Context, _ *slackClient.Blocks, threadTS string,
	) (string, error) {
		calls++
		return "", errors.New("error: invalid_thread_ts")
	}
	ts, err := postWithThreadFallback(context.Background(), post,
		blocks, "")
	require.Error(t, err)
	require.Equal(t, "", ts)
	require.Equal(t, 1, calls)
}

func TestPostWithThreadFallbackFallbackError(t *testing.T) {
	blocks := &slackClient.Blocks{}
	calls := 0
	post := func(
		_ context.Context, _ *slackClient.Blocks, threadTS string,
	) (string, error) {
		calls++
		if calls == 1 {
			return "", errors.New("error: thread_not_found")
		}
		return "", errors.New("fallback also failed")
	}
	ts, err := postWithThreadFallback(context.Background(), post,
		blocks, "old-ts")
	require.Error(t, err)
	require.Equal(t, "old-ts", ts)
	require.Equal(t, 2, calls)
}
