package transport

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type fixedTransport struct{ body string }

func (f fixedTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(f.body))}, nil
}

func TestSDKClientRefusesRedirects(t *testing.T) {
	c := SDKClient(&http.Client{})
	if c.CheckRedirect == nil || !errors.Is(
		c.CheckRedirect(nil, nil), http.ErrUseLastResponse,
	) {
		t.Fatal("redirects must not be followed")
	}
}

func TestSDKClientCapsResponseBody(t *testing.T) {
	big := strings.Repeat("x", maxSDKResponseBytes+1)
	c := SDKClient(&http.Client{Transport: fixedTransport{big}})
	response, err := c.Transport.RoundTrip(&http.Request{})
	if err != nil {
		t.Fatal(err)
	}
	read, err := io.ReadAll(response.Body)
	if !errors.Is(err, errResponseTooLarge) {
		t.Fatalf("err = %v, want too large", err)
	}
	if len(read) != maxSDKResponseBytes {
		t.Fatalf("read %d bytes", len(read))
	}
}

func TestSDKClientKeepsBodyAtTheBound(t *testing.T) {
	exact := strings.Repeat("x", maxSDKResponseBytes)
	c := SDKClient(&http.Client{Transport: fixedTransport{exact}})
	response, _ := c.Transport.RoundTrip(&http.Request{})
	read, err := io.ReadAll(response.Body)
	if err != nil || len(read) != maxSDKResponseBytes {
		t.Fatalf("read %d bytes, err %v", len(read), err)
	}
}

func TestSDKClientKeepsNil(t *testing.T) {
	if SDKClient(nil) != nil {
		t.Fatal("a nil client stays nil")
	}
}
