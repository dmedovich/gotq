package query

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"testing"
)

func TestV1ErrorCodeSetIsFrozen(t *testing.T) {
	got := []ErrorCode{
		CodeInvalidParameter,
		CodeInvalidToken,
		CodeInvalidSyntax,
		CodeLimitExceeded,
		CodeUnknownField,
		CodeNotFilterable,
		CodeNotSortable,
		CodeOperatorNotAllowed,
		CodeInvalidLiteral,
		CodeInvalidSchema,
		CodeExecutionFailed,
		CodeInvalidRelationship,
		CodeInvalidCursor,
	}
	want := []ErrorCode{
		"invalid_parameter",
		"invalid_token",
		"invalid_syntax",
		"limit_exceeded",
		"unknown_field",
		"field_not_filterable",
		"field_not_sortable",
		"operator_not_allowed",
		"invalid_literal",
		"invalid_schema",
		"execution_failed",
		"invalid_relationship",
		"invalid_cursor",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("V1 error codes = %q, want %q", got, want)
	}
}

func TestV1SchemaDescriptionWireShapeIsFrozen(t *testing.T) {
	description := EndpointDescription{
		SyntaxVersion: SyntaxVersion,
		Schema: SchemaDescription{
			Fields: []FieldDescription{{
				Name: "name", Kind: String, Nullable: true,
				Filterable: true, Sortable: true,
				Operators: []ComparisonOperator{Eq, Contains}, Codec: "slug",
			}},
			Relationships: []RelationshipDescription{{
				Name: "orders", Cardinality: RelationshipMany,
				Filterable: true, Sortable: false,
				Schema: SchemaDescription{Fields: []FieldDescription{}, Relationships: []RelationshipDescription{}},
			}},
		},
		Pagination: PaginationDescription{DefaultLimit: 20, MaxLimit: 100, MaxOffset: 1000, Cursor: true},
		Limits: Limits{
			MaxQueryBytes: 1, MaxFilterBytes: 2, MaxTokens: 3,
			MaxLiteralBytes: 4, MaxInValues: 5, MaxLimit: 6,
			MaxOffset: 7, MaxSortTerms: 8, MaxSearchBytes: 9,
			MaxExpressionDepth: 10, MaxNodes: 11, MaxPathDepth: 12,
			MaxQuantifierDepth: 13, MaxCursorBytes: 14,
		},
		Count: true, Search: true, CompatibilityAliases: false,
	}
	got, err := json.Marshal(description)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"syntaxVersion":"v1","schema":{"fields":[{"name":"name","kind":"string","nullable":true,"filterable":true,"sortable":true,"operators":["eq","contains"],"codec":"slug"}],"relationships":[{"name":"orders","cardinality":"many","filterable":true,"sortable":false,"schema":{"fields":[],"relationships":[]}}]},"pagination":{"defaultLimit":20,"maxLimit":100,"maxOffset":1000,"cursor":true},"limits":{"maxQueryBytes":1,"maxFilterBytes":2,"maxTokens":3,"maxLiteralBytes":4,"maxInValues":5,"maxLimit":6,"maxOffset":7,"maxSortTerms":8,"maxSearchBytes":9,"maxExpressionDepth":10,"maxNodes":11,"maxPathDepth":12,"maxQuantifierDepth":13,"maxCursorBytes":14},"count":true,"search":true,"compatibilityAliases":false}`
	if string(got) != want {
		t.Fatalf("V1 endpoint description JSON:\n%s\nwant:\n%s", got, want)
	}
}

func TestV1CursorEnvelopeEncodingIsFrozen(t *testing.T) {
	raw, err := json.Marshal(cursorEnvelope{
		Version:   cursorVersion,
		Signature: "0123",
		Values:    []json.RawMessage{json.RawMessage(`"x"`), json.RawMessage(`7`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(raw), `{"v":1,"s":"0123","k":["x",7]}`; got != want {
		t.Fatalf("cursor envelope = %s, want %s", got, want)
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	if encoded == "" || encoded[len(encoded)-1] == '=' {
		t.Fatalf("cursor encoding is not unpadded base64url: %q", encoded)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || string(decoded) != string(raw) {
		t.Fatalf("cursor round trip = %q, %v", decoded, err)
	}
}
