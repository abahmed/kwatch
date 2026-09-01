# Security policy

Kwatch runs inside a Kubernetes cluster and normally receives read-only access to
cluster resources. We take vulnerabilities in the binary, Helm chart, installer,
release process, and RBAC manifests seriously.

## Reporting a vulnerability

Please use GitHub's **Report a vulnerability** button in the Security tab to send a
private report. Do not open a public issue or include credentials, tokens, customer
data, or an unpatched proof of concept in a public discussion.

If private reporting is unavailable, open a minimal issue asking for a private
contact channel without describing the vulnerability.

Please include, when safe to share:

- the affected kwatch version and installation method;
- Kubernetes and container-runtime versions;
- the affected resource, permission, endpoint, or release artifact;
- clear reproduction steps and the expected security impact; and
- whether the issue is already public or known to have been exploited.

## Supported versions

The latest stable release is the supported security target. The current release
candidate is handled on a best-effort basis. Older releases may receive a fix when
the impact is critical, but users should upgrade to the latest stable release.

## Response and disclosure

We aim to acknowledge reports within three business days and provide an initial
triage update within seven business days. These are targets, not a guaranteed SLA.

We will coordinate disclosure with the reporter, publish a security advisory when
appropriate, and credit the reporter if they want to be named. We will not publish
technical details while users are still exposed to an available fix.

## Safe testing

Only test clusters and accounts that you own or are explicitly authorized to test.
Do not access data belonging to other users, disrupt a production cluster, delete
resources, or attempt to bypass access controls beyond the minimum needed to prove
the issue.

Kwatch does not currently operate a paid bug-bounty program.

## Release handling

Confirmed issues are fixed in a private security branch, reviewed, and released
with a patch version when needed. The release workflow publishes checksums, image
digests, provenance, and a Cosign signature so consumers can verify the repaired
artifact before upgrading.

