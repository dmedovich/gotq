# Public API and protocol stability policy

Status: stable v1.0 contract
Date: 2026-07-16

## Stable surface

The v1 contract covers every exported identifier in the root `query`,
`openapi`, and `queryhttp` packages, including function and method signatures,
method sets, constants, generic constraints, exported struct fields and JSON
tags, and documented behavior. `cmd` packages and the independent modules below
`examples/` and `website/` are not library API.

The release gate uses the pinned official `apidiff` tool to compare the entire
module with the previous release tag. The v1 gate rejects an unreviewed change
to the exported surface.

For stable releases:

- patch releases preserve source and behavioral compatibility and contain
  fixes, security hardening, documentation, and non-observable performance
  work;
- minor releases may add backward-compatible API after API, security, and
  compatibility review;
- removal, rename, signature change, narrowing of accepted input, or a semantic
  reinterpretation requires the next major version unless it fixes a security
  vulnerability that cannot safely be mitigated otherwise.

Sealed interfaces intentionally contain unexported methods. Applications may
use the values returned by gotq but cannot provide arbitrary implementations.
Changing which external exported types satisfy an interface is an API change.

## Query grammar and HTTP parameters

`SyntaxVersion == "v1"`, `GRAMMAR.md`, and `conformance/v1/queries.json` define
one protocol. All inputs accepted by a stable v1 release keep the same parse,
precedence, binding, and diagnostic classification throughout the v1 line.
Changing an existing meaning, reserving a previously valid public name, or
making a valid expression invalid requires a new syntax version and normally a
new module major version.

An additive parameter or operator is considered only in a minor release after
it is proven not to reinterpret existing requests. Unknown HTTP parameters
remain application-owned. The deprecated `orderby`, `top`, and `skip` aliases
remain opt-in through the v1 line; canonical clients use `sort`, `limit`, and
`offset`.

## Serialized contracts

The following are public wire formats:

- `Error`, `Position`, `Page`, and `PageInfo` JSON field names;
- `EndpointDescription`, nested `SchemaDescription`, scalar/operator text, and
  all effective-limit fields;
- OpenAPI vendor-extension names generated from the endpoint description;
- cursor protocol v1 as specified in `docs/cursor-protocol.md`.

Consumers should ignore unknown JSON object fields so a future minor release
can add optional metadata. Gotq will not remove, rename, change the JSON type of,
or reinterpret an existing field during v1. Schema descriptions are sorted and
detached; storage identifiers remain forbidden.

Cursor payload v1 is immutable for the v1 release line. Patch releases cannot
change canonical encoding, key typing, null ordering, or the meaning of its
sort signature. A new incompatible encoding requires a new cursor version;
decoders reject versions they do not understand.

## Deprecation

A stable API can be deprecated only with a documented replacement and upgrade
example. Except for an urgent security removal, a deprecated symbol or protocol
feature remains functional for at least two minor releases and six months,
whichever is longer, and is removed only in a major release.

The pre-v1 compatibility aliases entered v1 already deprecated. They remain
explicit opt-ins for the entire v1 line, but are omitted from new examples and
may be removed in v2. Deprecation never silently enables an alias or weakens an
endpoint policy.
