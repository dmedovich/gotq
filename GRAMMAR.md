# gotq v1 query grammar

This document defines the V1 language accepted by `ParseHTTP`. It is the source
of truth for lexer and parser behavior. Endpoint policy and schema behavior are
documented in the [model schema guide](website/docs/model-schema.md).

## Input model

`ParseHTTP` receives `url.Values`, so every parameter value has already been
percent-decoded and `+` in a raw URL has already become a space. All spans and
error positions refer to this decoded Go string.

The filter lexer accepts valid UTF-8 input. Identifiers and language keywords
are ASCII. Quoted strings may contain non-ASCII Unicode scalar values. The
first invalid UTF-8 byte is an `invalid_token` error.

V1 uses these ASCII whitespace bytes outside strings:

```text
SPACE (U+0020), TAB (U+0009), CR (U+000D), LF (U+000A)
```

Whitespace is skipped by the lexer, but token spans preserve enough
information for the parser to enforce required separation. LF advances the
one-based line number and resets the column to one. Every other byte,
including TAB and CR, advances the one-based byte column by one.

## Notation

The grammar uses ISO-style EBNF:

- `=` defines a production;
- `,` is concatenation;
- `|` separates alternatives;
- `[ x ]` means zero or one occurrence;
- `{ x }` means zero or more occurrences;
- quoted text is exact and case-sensitive;
- `-` means character-set subtraction;
- `EOF` is the end of the decoded parameter value.

`spacing` may be empty. `required-spacing` contains at least one whitespace
byte.

## Filter grammar

```ebnf
filter             = spacing, or-expression, spacing, EOF ;

or-expression      = and-expression,
                     { required-spacing, "or", required-spacing,
                       and-expression } ;

and-expression     = primary-expression,
                     { required-spacing, "and", required-spacing,
                       primary-expression } ;

primary-expression = comparison
                   | quantifier
                   | "not", required-spacing, primary-expression
                   | "(", spacing, or-expression, spacing, ")" ;

comparison         = path, required-spacing,
                     ( value-comparison, required-spacing, literal
                     | set-comparison, required-spacing, literal-list
                     | null-comparison ) ;

quantifier         = path, "/", quantifier-operator, "(", identifier,
                     ":", required-spacing, or-expression, spacing, ")" ;
quantifier-operator = "any" | "all" ;

path               = identifier, { "/", identifier } ;

value-comparison    = "eq" | "ne" | "gt" | "gte"
                    | "lt" | "lte" | "contains"
                    | "startswith" | "endswith" ;

set-comparison      = "in" | "not", required-spacing, "in" ;
null-comparison     = "is", required-spacing,
                      [ "not", required-spacing ], "null" ;

literal-list       = "(", spacing, literal,
                     { spacing, ",", spacing, literal }, spacing, ")" ;

literal            = string-literal | number-literal | boolean-literal ;
boolean-literal    = "true" | "false" ;

identifier         = identifier-start, { identifier-continue } ;
identifier-start   = ASCII-letter | "_" ;
identifier-continue = identifier-start | decimal-digit ;

ASCII-letter       = "A" | ... | "Z" | "a" | ... | "z" ;
decimal-digit      = "0" | ... | "9" ;

string-literal     = "'", { string-character | escaped-apostrophe }, "'" ;
escaped-apostrophe = "''" ;
string-character   = Unicode-scalar - ("'" | control-character) ;
control-character  = U+0000 | ... | U+001F | U+007F ;

number-literal     = [ "-" ], integer-part,
                     [ fraction-part ], [ exponent-part ] ;
integer-part       = "0" | non-zero-digit, { decimal-digit } ;
fraction-part      = ".", decimal-digit, { decimal-digit } ;
exponent-part      = ("e" | "E"), [ "+" | "-" ],
                     decimal-digit, { decimal-digit } ;
non-zero-digit     = "1" | ... | "9" ;

spacing            = { whitespace } ;
required-spacing   = whitespace, spacing ;
whitespace         = SPACE | TAB | CR | LF ;
```

### Precedence and associativity

From highest to lowest precedence:

1. parenthesized expressions;
2. comparisons, relationship quantifiers, and unary `not`;
3. `and`;
4. `or`.

`and` and `or` associate to the left. Consequently:

```text
a eq 1 or b eq 2 and c eq 3
```

has the same tree as:

```text
(a eq 1) or ((b eq 2) and (c eq 3))
```

and:

```text
a eq 1 or b eq 2 or c eq 3
```

has the same tree as:

```text
((a eq 1) or (b eq 2)) or (c eq 3)
```

Parentheses affect grouping but do not produce AST nodes. Unary `not` and each
`any`/`all` quantifier produce a node and therefore count toward node and depth
limits.

### Relationship paths and quantifiers

Path segments use public policy names separated by `/`. Whitespace is not
allowed around `/`. A direct path may traverse only explicitly exposed to-one
relationships and must end at an exposed scalar field:

```text
company/country/code eq 'US'
```

A to-many relationship requires `any` or `all`. The variable after `(` creates
a lexical scope, and every path inside that predicate must start with that
variable:

```text
orders/any(o: o/total gt 100)
orders/all(o: o/status eq 'paid')
orders/any(o: o/items/any(i: i/price gte 10))
```

Variables cannot shadow an active variable. `any` is true when at least one
non-soft-deleted related row satisfies its predicate. `all` is true when no
related row violates the predicate, so it is true for an empty collection.
SQL `NULL` predicate results count as not satisfying `all`. The caller's GORM
`Unscoped` setting consistently includes soft-deleted root and related rows.

`MaxPathDepth` counts relationship plus final-field segments after removing a
quantifier variable. `MaxQuantifierDepth` counts simultaneously nested
quantifiers. Both are checked during parsing and again during public AST
validation.

### Keywords and identifiers

The following exact lowercase words are reserved and cannot be public field
names:

```text
and or not eq ne gt gte lt lte contains startswith endswith in is null any all
true false asc desc
```

Keyword matching is case-sensitive. `TRUE`, `Contains`, and `AND` are ordinary
identifiers, not keyword aliases. A schema containing a reserved public name
is rejected during `Build()` so it cannot be filterable in one context but
unaddressable in another.

Whitespace is required around comparison and logical operators and between the
words of composite operators. Inputs such
as `age gt(18)`, `age gt 18and name eq 'a'`, and
`age gt 18or(age lt 5)` are invalid.

### String literals

Strings use single quotes. An apostrophe is represented by two adjacent
apostrophes:

```text
'O''Brien'
```

Its decoded literal value is `O'Brien`. Backslash has no special meaning in
the query language, so `'a\b'` contains one backslash. Empty strings are valid.
ASCII control characters, including literal newlines and tabs, are not allowed
inside a string.

The lexer retains both the raw token text, including quotes, and the decoded
string value. Later schema validation decides whether a string is a `String`,
RFC 3339 `Time`, `Date`, or `UUID` value.

### Number literals

Valid examples include:

```text
0
-0
42
-17
0.5
-12.75
1e3
6.02E+23
1e-9
```

Leading zeroes, a leading plus, a missing integer part, a trailing decimal
point, and incomplete exponents are invalid. Therefore `01`, `+1`, `.5`,
`1.`, and `1e` are rejected.

The lexer only establishes number syntax. The validator later checks the
field kind, signedness, bit width, finite floating-point range, whether a
fraction or exponent is legal for an integer field, and exact `Decimal`
conversion without passing through binary floating point.

### AST and complexity invariants

A comparison produces one `ComparisonExpr`. Each `any` or `all` produces one
`QuantifierExpr`. Each `and` or `or` produces one `LogicalExpr`. There are no
nodes for identifiers, literals, operators, variables, or parentheses in the
V1 `Expr` tree.

For a comparison, node count and depth are both one. For a logical expression:

```text
nodes = 1 + nodes(left) + nodes(right)
depth = 1 + max(depth(left), depth(right))
```

For example, `a eq 1 and b eq 2 and c eq 3` contains five nodes and, because
operators associate to the left, has depth three. Redundant parentheses do not
change either value. The parser checks limits while constructing nodes and
must not first build an unbounded tree.

Before lexing, `MaxFilterBytes` bounds the decoded filter. `MaxTokens` bounds
non-EOF tokens and `MaxLiteralBytes` bounds string and number token byte length.
`MaxQueryBytes` bounds all decoded query parameter names and values before a
known parameter is parsed.

Because parentheses do not count as AST nodes, V1 also applies a fixed safety
limit of 128 simultaneously open parentheses. Exceeding it is
`limit_exceeded` at the parenthesis that would cross the limit. This guard is
not configurable and does not alter expression-depth semantics.

## Sort grammar

`sort` is parsed as a simple HTTP parameter, not by the filter lexer.

```ebnf
sort               = spacing, sort-term,
                     { spacing, ",", spacing, sort-term },
                     spacing, EOF ;

sort-term          = [ "-" ], path ;
```

The identifier and whitespace productions are the same as for `filter`. A
leading `-` selects descending order; otherwise order is ascending. Empty
terms, a bare `-`, and trailing commas are invalid.

Every effective sort term uses `NULLS LAST` in both directions. `List` appends
missing primary-key columns in ascending order and owns the resulting complete
order even when the caller base scope already contains an order clause.

Sort paths may traverse only explicitly exposed to-one relationships. Sorting
through a to-many relationship is rejected. Repeating a public path in one
`sort` value is rejected as
`invalid_parameter` at the start of the repeated field, even if both
directions agree. Whether a field exists and is sortable is checked later
against the model schema.

With `WithCompatibilityAliases`, `orderby` accepts the pre-release grammar
`path [ ("asc" | "desc") ]` separated by commas. Alias and canonical
parameters cannot be used together.

## Scalar HTTP parameter grammar

```ebnf
limit              = unsigned-integer, EOF ;
offset             = unsigned-integer, EOF ;
cursor             = base64url-character, { base64url-character }, EOF ;
count              = ("true" | "false"), EOF ;
search             = utf8-text, EOF ;

unsigned-integer   = "0" | non-zero-digit, { decimal-digit } ;
utf8-text           = ? one or more valid UTF-8 code points ? ;
base64url-character = ASCII-letter | decimal-digit | "-" | "_" ;
```

These values do not accept surrounding whitespace, signs, leading zeroes, or
case variants. `limit` and `offset` are additionally checked against
`Limits.MaxLimit` and `Limits.MaxOffset`. Parsing requires each integer to fit
the public Go `int` representation; overflow is `invalid_parameter`. `search`
must be non-empty valid UTF-8 and no longer than `Limits.MaxSearchBytes` bytes.

`cursor` has no surrounding whitespace or `=` padding and is bounded by
`Limits.MaxCursorBytes` before decoding. `cursor` and `offset` cannot appear
together. `ParseHTTP` records cursor syntax and size; an engine later verifies
canonical base64url/JSON, version, typed keys, and the effective sort signature.
The versioned payload contract is normative in `docs/cursor-protocol.md`.

Each known HTTP parameter may have exactly one value. A missing key is
different from an empty value. An empty value and two or more values are
`invalid_parameter` errors. Unknown HTTP parameters are ignored.

## Source spans and diagnostics

Every token and public AST node has a half-open byte span `[Start, End)` in the
decoded parameter value. EOF has span `[len(input), len(input))`. The source
span of a parenthesized expression is the span of its inner expression; it
does not include parentheses because parentheses do not survive in the AST.
A logical node spans from the start of its left child to the end of its right
child.

`Position.Offset` is `Span.Start`. Line and column are derived from the rules
in the input model. Errors point to the earliest byte at which the input can no
longer satisfy the grammar:

- an illegal byte or invalid UTF-8 points at that byte;
- a forbidden control character in a string points at the control character;
- an unterminated string points at EOF, where the closing quote was expected;
- an unexpected token points at the start of that token;
- a missing token at EOF points at EOF;
- a complexity violation points at the logical operator whose node would
  exceed the configured limit;
- a duplicate `sort` field points at the repeated identifier;
- a schema error points at the field, operator, literal, or order term that
  failed validation.

Lexical failures use `invalid_token`. Parser failures use `invalid_syntax`.
Malformed scalar parameters and structurally invalid `sort` values use
`invalid_parameter`; malformed or mismatched cursor payloads use
`invalid_cursor`. Complexity and configured bound failures use
`limit_exceeded`.

## Accepted filter vectors

These cases are syntactically valid independently of any model schema:

| Input | Invariant exercised |
| --- | --- |
| `age gt 18` | integer comparison |
| `name contains 'ann'` | string comparison |
| `name eq ''` | empty string |
| `name eq 'O''Brien'` | apostrophe decoding |
| `city eq 'Волгоград'` | UTF-8 string |
| `active eq true` | boolean literal |
| `score gte -1.5e2` | signed decimal exponent |
| `createdAt gte '2026-07-16T12:30:00Z'` | syntactic string, later converted as time |
| `eventDate eq '2026-07-16'` | syntactic string, later converted as date |
| `id in (1, 2, 3)` | non-empty literal list |
| `id not in (4, 5)` | composite set operator |
| `deletedAt is null` | null predicate without a literal |
| `deletedAt is not null` | negated null predicate |
| `name startswith 'Ann'` | escaped prefix match |
| `name endswith 'son'` | escaped suffix match |
| `not active eq true` | unary logical negation |
| `(age gt 18)` | grouping without an extra AST node |
| `age gt 18 and name contains 'ann'` | logical conjunction |
| `age lt 18 or age gt 65` | logical disjunction |
| `a eq 1 or b eq 2 and c eq 3` | `and` precedence over `or` |
| `(a eq 1 or b eq 2) and c eq 3` | parentheses override precedence |
| `company/country/code eq 'US'` | explicit to-one path |
| `orders/any(o: o/total gt 100)` | collection existential predicate |
| `orders/all(o: o/status eq 'paid')` | collection universal predicate |
| `orders/any(o: o/items/any(i: i/price gte 10))` | nested lexical quantifiers |
| `  age\tgt\t18  ` | accepted leading, trailing, and required whitespace |

In the last row, the table notation `\t` represents an actual TAB byte in the
test input rather than the two characters backslash and `t`.

## Rejected filter vectors

Offsets below are zero-based byte offsets in the decoded value.

| Input | Code | Offset | Reason |
| --- | --- | ---: | --- |
| empty string | `invalid_parameter` | 0 | known parameter has an empty value |
| three spaces | `invalid_syntax` | 3 | expression expected at EOF |
| `age` | `invalid_syntax` | 3 | comparison operator expected at EOF |
| `age gt` | `invalid_syntax` | 6 | literal expected at EOF |
| `age > 18` | `invalid_token` | 4 | raw SQL operator is not language syntax |
| `age gt +1` | `invalid_token` | 7 | leading plus is forbidden |
| `age eq 01` | `invalid_token` | 8 | leading zero makes the numeric token invalid |
| `score eq 1.` | `invalid_token` | 11 (EOF) | fractional digits are required |
| `score eq 1e` | `invalid_token` | 11 (EOF) | exponent digits are required |
| `active eq TRUE` | `invalid_syntax` | 10 | keyword matching is case-sensitive |
| `age gt 18 trailing` | `invalid_syntax` | 10 | unexpected token after a complete expression |
| `age gt 18 and` | `invalid_syntax` | 13 | right operand expected at EOF |
| `(age gt 18` | `invalid_syntax` | 10 | closing parenthesis expected at EOF |
| `age gt 18)` | `invalid_syntax` | 9 | unexpected closing parenthesis |
| `()` | `invalid_syntax` | 1 | expression expected |
| `id in ()` | `invalid_syntax` | 7 | list must contain a literal |
| `id in (1,)` | `invalid_syntax` | 9 | trailing comma is forbidden |
| `id in(1)` | `invalid_syntax` | 5 | whitespace is required before the list |
| `not(active eq true)` | `invalid_syntax` | 3 | whitespace is required after unary `not` |
| `company/ name eq 'Acme'` | `invalid_syntax` | 7 | whitespace is forbidden around `/` |
| `orders/any(o:o/total gt 1)` | `invalid_syntax` | 13 | whitespace is required after `:` |
| `orders/any(o: total gt 1)` | `invalid_relationship` | 14 | predicate path is not rooted at `o` |
| `age gt(18)` | `invalid_syntax` | 6 | whitespace required before the literal |
| `a eq 1and b eq 2` | `invalid_syntax` | 6 | whitespace required before `and` |
| an unterminated quoted string | `invalid_token` | EOF | closing apostrophe expected |
| a string containing LF | `invalid_token` | LF | control characters are forbidden in strings |

Malformed numbers are consumed as one attempted numeric token so the error is
reported at the first byte that violates the number production rather than as
an unrelated trailing-token error.

## Accepted and rejected non-filter vectors

| Parameter value | Accepted | Result or reason |
| --- | --- | --- |
| `sort=-createdAt,name` | yes | `createdAt` descending, `name` ascending |
| `sort=company/name` | yes | to-one relationship path, subject to policy |
| `sort=orders/total` | parser yes, schema no | sorting through to-many is forbidden |
| `sort= name , -createdAt ` | yes | surrounding spacing is ignored |
| `sort=name,` | no | sort term expected at EOF |
| `sort=name,,age` | no | empty sort term |
| `sort=name DESC` | no | legacy direction syntax is not canonical |
| `sort=name,name` | no | duplicate public field |
| `limit=0` | yes | limit zero |
| `limit=100` | yes with default limits | inclusive maximum |
| `limit=101` | no with default limits | `limit_exceeded` |
| `limit=01` | no | leading zero |
| `limit=-1` | no | sign is forbidden |
| `limit= 1` | no | whitespace is forbidden |
| `offset=0` | yes | offset zero |
| `offset=42` | yes | offset 42 |
| `offset=100001` | no with default limits | `limit_exceeded` |
| `count=true` | yes | `Query.Count` points to `true` |
| `count=false` | yes | `Query.Count` points to `false` |
| `count=True` | no | value is case-sensitive |
| `search=ann` | yes | `Query.Search` points to `ann` |
| empty `search` | no | known values must not be empty |
| two `filter` values | no | duplicate known parameter |
| unknown `cursor=abc` | yes | ignored by `ParseHTTP` |

## Schema-stage vectors

The following filters are syntactically valid. They must remain parser
successes and fail only during schema validation when applied to the example
Example `User` schema:

| Filter | Validation result |
| --- | --- |
| `missing eq 1` | `unknown_field` at `missing` |
| `id eq 1` | `field_not_filterable` at `id` |
| `age contains 'ann'` | `operator_not_allowed` at `contains` |
| `age eq '18'` | `invalid_literal` at `'18'` |
| `name eq 18` | `invalid_literal` at `18` |
| `createdAt gt 'yesterday'` | `invalid_literal` at `'yesterday'` |
| `name gt 'ann'` | `operator_not_allowed` at `gt` |
| `company/name eq 'Acme'` | succeeds only when `company` and nested `name` are exposed and filterable |
| `orders/total gt 1` | `invalid_relationship`; to-many requires `any` or `all` |
| `orders/any(o: total gt 1)` | `invalid_relationship`; paths must use variable `o` |
| `missing/name eq 'x'` | `invalid_relationship`; undisclosed relationships are indistinguishable from unknown ones |

Likewise, `sort=age` is syntactically valid and fails as
`field_not_sortable`, while `sort=missing` fails as `unknown_field`.

## Test obligations derived from the grammar

Tests should protect behavior, not implementation details or a coverage
percentage. At minimum, later stages must preserve these invariants:

- the lexer never panics, always advances or returns an error, and reports byte
  spans consistently for ASCII, UTF-8 strings, invalid UTF-8, and EOF;
- tokenization distinguishes exact keywords from identifier prefixes;
- string apostrophe decoding is reversible and control characters are
  rejected;
- numeric maximal munch reports malformed numbers at their first invalid byte;
- the parser consumes the complete input and cannot silently accept a valid
  prefix;
- precedence, associativity, and parentheses produce the documented tree;
- missing required whitespace is rejected even though trivia is not emitted;
- node and depth limits are enforced during construction;
- path and quantifier limits are enforced during construction and validation;
- quantifier lexical scope and variable shadowing are rejected deterministically;
- syntax-valid but schema-invalid cases cross the parser boundary unchanged;
- parameter values are never interpreted as SQL fragments.

The accepted and rejected tables are normative test vectors. Implementations
may add focused regression cases, but do not need combinatorial tests that add
no new invariant.
