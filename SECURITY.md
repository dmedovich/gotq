# Security policy

## Reporting a vulnerability

Please report suspected vulnerabilities through GitHub's private
**Security → Report a vulnerability** flow for this repository. Do not open a
public issue with exploit details.

Include the affected version, a minimal reproduction, expected impact, and any
known mitigations. Reports concerning SQL identifier injection, bound-value
bypasses, parser resource exhaustion, or schema validation bypasses are
especially useful.

The current trust boundaries, mitigations, and residual risks are documented in
[`docs/threat-model.md`](docs/threat-model.md).

The project will acknowledge a complete report, investigate it privately, and
coordinate disclosure with the reporter after a fix is available.

## Supported versions

| Release line | Supported |
| --- | --- |
| latest `v1.0.x` patch | Yes |
| latest commit on `main` | Development security fixes |

Only the newest patch of the listed stable minor line receives fixes. When a
new v1 minor becomes supported, this table will state any overlap before the
older minor reaches end of support. A newer major line will likewise publish
its support overlap before superseding v1.

Gotq's Go 1.23 CI lane is source compatibility only. Production binaries must
use a security-supported, fully patched Go toolchain; the v1.0 release gate
tests Go 1.25.12 and Go 1.26.5 and runs `govulncheck` with Go 1.26.5.

## Disclosure expectations

The project aims to acknowledge a complete report within three business days
and provide an initial severity assessment within seven business days. These
are response targets, not a guaranteed remediation SLA. The maintainer and
reporter coordinate a disclosure date after a supported fix or mitigation is
available. Please allow reasonable remediation time before public disclosure
unless active exploitation or immediate user protection requires otherwise.

SQL injection, tenant/authorization scope loss, data corruption, remotely
exploitable exhaustion, and broadly incorrect results are release-blocking
severity-1 issues. Incorrect page/count boundaries, policy bypass, frequent
panic, and major cross-dialect breaks are severity 2.
