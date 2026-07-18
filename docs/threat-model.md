# Threat model

Status: reviewed for stable v1.0
Date: 2026-07-16

## Assets and trust boundaries

gotq protects database confidentiality, tenant/authorization scope, query
correctness, and service resources while processing attacker-controlled decoded
HTTP query parameters.

Untrusted inputs are parameter names/values, filter syntax, public field paths,
literals, sort terms, pagination/cursors, count intent, and search text. Endpoint policy,
GORM model metadata, base scopes, and search callback code are trusted
application inputs. Database drivers and GORM are dependencies inside the server
trust boundary.

## Threats and controls

| Threat | Primary controls | Tests |
| --- | --- | --- |
| Value SQL injection | GORM bound variables; closed operators | injection vars and dialect conformance tests |
| Identifier/order injection | explicit policy; GORM metadata; clause identifiers | invalid field/sort tests and SQL goldens |
| Policy bypass | bind before compile; immutable scalar and nested relationship maps | unknown/non-capable field/relation tests |
| Relationship identifier injection | public paths resolve to GORM association/key/table metadata; internal aliases are generated and quoted | path injection, alias-collision, and dialect SQL tests |
| Tenant scope loss | every data/count/search query derives from caller base; tenant-relative associations use composite references | scalar and composite relationship tenant integration tests |
| Count/page corruption | to-many predicates use correlated subqueries; count excludes query sort/page; stable PK tie-break; `limit+1` | relationship list/count/stable-order tests |
| Parser exhaustion | query/filter/token/literal/node/depth/path/quantifier/parenthesis limits | inclusive boundaries, fuzzing, rejection benchmark |
| Pagination exhaustion | default/max limit and max offset | engine configuration and boundary tests |
| Cursor parsing/exhaustion | encoded byte bound; canonical base64url/JSON; exact key count and typed round-trip | boundary, malformed payload, fuzz, and rejection tests |
| Cursor sort confusion | versioned signature covers trusted model and complete effective sort | wrong-version, wrong-sort, and tamper tests |
| Cursor scope replay | cursor grants position only; current base/filter/search always reapply | tenant, relationship, filter, search, and count continuation tests |
| Information leakage | stable error codes; causes excluded from JSON | serialization and unwrap tests |
| Panic from malformed public AST | cycle/nil/enum/literal/path/scope validation | malformed scalar/quantifier AST and fuzz tests |
| Concurrency races | immutable engine/schema; request-local sessions | race detector and concurrent engine test |
| Dialect semantic drift | shared DryRun and real DB conformance | SQLite/PostgreSQL/MySQL matrix |
| Custom scalar SQL escape | codec API exposes literals/types only; compiler operators stay closed | codec configuration and bound-var tests |
| Cross-relation data leakage | every traversed relation is explicit; to-many scope variables are lexical; polymorphic metadata is rejected | undisclosed path, shadowing, composite key, and nested quantifier tests |

## Search callback boundary

Search callbacks are trusted application code and can create unsafe SQL if
implemented incorrectly. gotq bounds the client term, supplies context and the
current base scope, and rejects nil callback scopes. It cannot prove callback
SQL safety. Documentation and examples require bound arguments; a future
dialect search adapter may reduce application-owned SQL.

Custom scalar codecs are also trusted application code, but their authority is
smaller: they validate a Go target and convert a syntactic literal to a bound
value. They receive no GORM scope, SQL builder, table, or column identifier.

## Residual risks

- Exact count can remain expensive within accepted bounds; endpoints choose
  whether to enable it.
- Large offsets can remain expensive below `MaxOffset`; applications should use
  cursor pagination for deep traversal.
- Core cursors are unsigned, unencrypted, non-expiring, and not bound to a
  filter or principal. Applications wrap the opaque token when those properties
  are required.
- Base64url cursor keys are not confidential and include hidden primary-key
  tie-breaker values; applications encrypt the complete token when disclosure
  of those values is unacceptable.
- Database collation controls `contains` case behavior.
- Caller-provided base scopes and search callbacks are server-trusted and may
  themselves omit authorization.
- Standard `gorm.DeletedAt` is honored in relationship subqueries; custom
  soft-delete plugins require separate compatibility review.
- GORM has-one data that violates its application cardinality invariant can
  make a sort join ambiguous; database uniqueness constraints remain an
  application responsibility.
- CI covers declared database versions, not every version accepted by a driver.

Security findings follow `SECURITY.md`. SQL injection, scope loss, broad
incorrect results, and remotely exploitable exhaustion are release blockers.
