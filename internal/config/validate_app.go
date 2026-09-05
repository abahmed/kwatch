package config

import (
	"crypto/x509"
	"errors"
	"fmt"
	"net/url"
	"os"
)

func validateApp(app App) []error {
	var errs []error
	if app.ProxyURL != "" {
		parsed, err := url.Parse(app.ProxyURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			errs = append(errs, errors.New("app.proxyURL must be a valid URL"))
		} else if parsed.Scheme != "http" && parsed.Scheme != "https" {
			errs = append(errs, fmt.Errorf(
				"app.proxyURL has unsupported scheme %q", parsed.Scheme,
			))
		}
	}
	if app.CABundlePath == "" {
		return errs
	}
	contents, err := os.ReadFile(app.CABundlePath)
	if err != nil {
		return append(errs, fmt.Errorf(
			"app.caBundlePath cannot be read: %w", err,
		))
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(contents) {
		errs = append(errs, errors.New(
			"app.caBundlePath does not contain a valid PEM certificate",
		))
	}
	return errs
}
