# 📄 Licensing

Kwatch is source available under the
[Elastic License 2.0 (ELv2)](../LICENSE), starting after `v1.0.0-rc.10`.

ELv2 lets you use kwatch for free, including commercially in your own
clusters, and read, modify, and redistribute the code. It has three
limitations:

1. You may not provide kwatch to others as a hosted or managed service.
2. You may not move, change, disable, or circumvent license key
   functionality.
3. You may not remove or obscure licensing, copyright, or other notices.

## Earlier releases

Releases up to and including `v1.0.0-rc.10` were published under the MIT
License, and those releases remain available under MIT. The MIT copyright
notice for that code is:

```text
Copyright (c) 2021 Abdelrahman Ahmed

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

## Contributions

By submitting a contribution you agree that it is licensed under the
project's current license, and that the maintainer may relicense it as part
of kwatch.

Third-party dependencies keep their own licenses and are listed in
[`third-party-notices.md`](./third-party-notices.md).

## Pro license keys

A Pro license key is a signed token that kwatch verifies with signing keys
compiled into the binary, each named by a key id (`kid`). A key signed under
a key id the binary does not know is reported as invalid; a key rotation ships
the new key id in a release while keys signed under the old one keep
verifying. kwatch activates the key against the license server over HTTPS
(through `app.proxyURL` and `app.caBundlePath`) and always verifies that
connection's TLS certificate: `app.insecureSkipTLSVerify` never applies to it.

- Spaces and line breaks inside the key are removed when the configuration
  loads, so a wrapped key still works. A key that is still unusable is
  reported by the license manager, never as a configuration error, and never
  stops kwatch from starting.
- A key that cannot run Pro (invalid, expired, revoked, or in use on another
  cluster) logs one warning per distinct reason, shows a `degraded` `license`
  component on `/health`, and sends a "Pro is disabled" notice with the
  reason, including at the first start when Pro never ran. kwatch runs the
  Community edition meanwhile.
- The lease and the local clock mark are kept in the state file. If the clock
  stepped forward and is corrected, the next renewal's signed server time pulls
  the mark back and Pro recovers.
- kwatch keeps an instance key in the Secret `kwatch-license-instance` in its
  own namespace. The chart renders that Secret and the Role that reads it
  only when a key is configured (`license.key`, `license.existingSecret` or
  `license.instanceSecret`). Keep the key out of ConfigMaps: use the
  `KWATCH_LICENSE_KEY` variable, which the chart fills from a Secret.
