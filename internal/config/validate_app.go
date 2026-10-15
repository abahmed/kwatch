package config

import (
	"crypto/x509"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"unicode"
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

// validateLicense checks the license settings without echoing the key.
func validateLicense(license License) []error {
	var errs []error
	if strings.ContainsFunc(license.Key, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}) {
		errs = append(errs, errors.New(
			"license.key must not contain spaces or control characters"))
	}
	if license.Endpoint != "" {
		parsed, err := url.Parse(license.Endpoint)
		if err != nil || parsed.Host == "" ||
			(parsed.Scheme != "http" && parsed.Scheme != "https") {
			errs = append(errs, errors.New(
				"license.endpoint must be an http(s) URL"))
		}
	}
	return errs
}
