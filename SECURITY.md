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

Gotq requires Go 1.25 or newer. The oldest CI lane checks source compatibility.
Production binaries must use a security-supported, fully patched Go toolchain.
The v1.1 release preparation tests Go 1.25.14, 1.26.9, and 1.27.2 and runs
`govulncheck` with Go 1.27.2.

## Documentation dependency exception

Reviewed on 2026-10-18; expires **2026-11-18 at 00:00 UTC**. The maintainer
accepted the following temporary build-time risk for the static documentation
site. This does not declare the dependency fixed or dismiss other advisories.

- Advisory: [GHSA-vfj7-8cjw-p6xm](https://github.com/advisories/GHSA-vfj7-8cjw-p6xm).
- Scope: exactly `braces` 3.0.3 in `website/package-lock.json`, including npm's
  propagated findings in dependent packages. Its installed direct callers are
  `micromatch` 4.0.8 and `chokidar` 3.6.0.
- Failure: deeply nested patterns can exhaust the Node stack. Both `compile`
  and `expand` reproduce the failure on Node 22 with 4,500 nested braces. There
  is no published fix at the review date; the maintainer
  [closed the release request](https://github.com/micromatch/braces/issues/84).
- Exposure: CI builds repository-controlled documentation using configured glob
  patterns such as `**/*.{md,mdx}`. GitHub Pages serves only `website/build`;
  there is no public Docusaurus Node server or visitor-supplied glob input.
  The Go library has no npm runtime dependency. Repository changes can still
  break a build; pull-request builds have read-only repository permissions,
  and Pages deployment runs only for pushes to `main`.
- Enforcement: `npm run audit:ci` runs a fresh npm audit, accepts only this
  advisory, and keeps the moderate severity threshold. A second advisory in
  the same package or dependency chain still fails. Invalid reports, npm
  errors, an expired exception, an unreviewed installed version, or an available
  fix also fail the gate. Accepted findings remain visible in CI output.
- Removal: update to a patched dependency as soon as available and remove the
  exception. Reassess before expiry; do not automatically extend it. Reassess
  immediately if documentation hosting changes or untrusted patterns become
  inputs to these tools. Plain `npm audit` continues to report the advisory.

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
