---
name: Bug report
about: Create a report to help us improve

---

**Describe the bug**
A clear and concise description of what the bug is.

**To Reproduce**
Steps to reproduce the behavior:

**Expected behavior**
A clear and concise description of what you expected to happen.

**Actual behavior**
A clear and concise description of what really happens.

**Version/Commit**
- Kwatch version:
- Image tag or digest:
- Source commit, if known:

**Installation method**
Helm, raw manifest, kwatch manager, or another method.

**Reproducibility**
- Reproduces after restart: yes/no
- Reproduces on every attempt: yes/no
- First known good/bad version:

**Kubernetes environment**
- Kubernetes version:
- Cloud or distribution:
- Container runtime:

**Issue link**
If this is a regression, include the issue or pull request that introduced
the permanent scenario.

**Configuration and logs**
Paste the smallest redacted configuration and relevant logs. Never include
tokens, credentials, webhook URLs, Secret data, or complete sensitive payloads.

Include the smallest redacted Kwatch configuration and Kubernetes resources
needed to reproduce the issue. Maintainers may convert them into a reviewed,
committed Kind scenario.

**Reproduction**
Is this reproducible after restart or only during an outage? Include the
smallest safe reproduction and any health, readiness, or metric output.
Credentials, Secret data, private image references, and host access must never
be included. Issue content is never executed directly.
