package transport

import (
	"errors"
	"io"
	"net/http"
)

// maxSDKResponseBytes bounds a response body read by a provider SDK, as
// Send bounds its own reads.
const maxSDKResponseBytes = 1 << 20

// errResponseTooLarge is returned by a capped body that went past the
// bound, so the SDK fails instead of decoding a cut-off answer.
var errResponseTooLarge = errors.New("provider response body too large")

// SDKClient returns a copy of c for a provider SDK that executes its own
// requests (such as slack-go): like Send it never follows a redirect and
// reads at most 1 MiB of a response body. A nil c stays nil, so the SDK
// keeps its own default.
func SDKClient(c *http.Client) *http.Client {
	if c == nil {
		return nil
	}
	copied := noRedirects(c)
	base := copied.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	copied.Transport = cappedTransport{base: base,
		limit: maxSDKResponseBytes}
	return copied
}

// cappedTransport caps every response body it returns.
type cappedTransport struct {
	base  http.RoundTripper
	limit int64
}

func (t cappedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(r)
	if err != nil || response == nil || response.Body == nil {
		return response, err
	}
	response.Body = &cappedBody{body: response.Body, left: t.limit}
	return response, nil
}

// cappedBody reads at most left bytes, then fails.
type cappedBody struct {
	body io.ReadCloser
	left int64
}

func (b *cappedBody) Read(p []byte) (int, error) {
	if b.left <= 0 {
		// One more byte tells a body that ends exactly at the bound
		// from one that goes on.
		var probe [1]byte
		if n, _ := b.body.Read(probe[:]); n > 0 {
			return 0, errResponseTooLarge
		}
		return 0, io.EOF
	}
	if int64(len(p)) > b.left {
		p = p[:b.left]
	}
	n, err := b.body.Read(p)
	b.left -= int64(n)
	return n, err
}

func (b *cappedBody) Close() error { return b.body.Close() }
