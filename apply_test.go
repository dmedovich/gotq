package query

import (
	"net/url"
	"reflect"
	"testing"
)

func TestParseAndApplyEndToEnd(t *testing.T) {
	t.Parallel()

	schema, err := testUserSchema()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseHTTP(url.Values{
		"filter": {"age gt 18 and name contains 'ann'"},
		"sort":   {"-createdAt"},
		"limit":  {"20"},
	})
	if err != nil {
		t.Fatal(err)
	}
	scoped, err := Apply(dryRunDB(t).Model(&schemaTestUser{}), schema, parsed)
	if err != nil {
		t.Fatal(err)
	}
	statement := scoped.Find(&[]schemaTestUser{}).Statement
	wantSQL := "SELECT * FROM `schema_test_users` WHERE `age` > ? AND `name` LIKE ? ESCAPE '!' ORDER BY CASE WHEN `created_at` IS NULL THEN 1 ELSE 0 END ASC,`created_at` DESC LIMIT 20"
	if got := statement.SQL.String(); got != wantSQL {
		t.Fatalf("SQL = %q, want %q", got, wantSQL)
	}
	if want := []any{int8(18), "%ann%"}; !reflect.DeepEqual(statement.Vars, want) {
		t.Fatalf("Vars = %#v, want %#v", statement.Vars, want)
	}
}

func TestApplyRejectsInvalidOperator(t *testing.T) {
	t.Parallel()

	schema, err := testUserSchema()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseHTTP(url.Values{"filter": {"age contains 'ann'"}})
	if err != nil {
		t.Fatalf("syntax parse error = %v", err)
	}
	scoped, err := Apply(dryRunDB(t).Model(&schemaTestUser{}), schema, parsed)
	if scoped != nil {
		t.Fatalf("Apply() DB = %#v, want nil", scoped)
	}
	queryErr, ok := err.(*Error)
	if !ok || queryErr.Code != CodeOperatorNotAllowed || queryErr.Position == nil || queryErr.Position.Offset != 4 {
		t.Fatalf("Apply() error = %#v", err)
	}
}

func TestCountDoesNotChangeScopes(t *testing.T) {
	t.Parallel()

	schema, err := testUserSchema()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseHTTP(url.Values{"count": {"true"}})
	if err != nil {
		t.Fatal(err)
	}
	scoped, err := Apply(dryRunDB(t).Model(&schemaTestUser{}), schema, parsed)
	if err != nil {
		t.Fatal(err)
	}
	statement := scoped.Find(&[]schemaTestUser{}).Statement
	if got, want := statement.SQL.String(), "SELECT * FROM `schema_test_users`"; got != want {
		t.Fatalf("SQL = %q, want %q", got, want)
	}
}

func TestApplyDefendsPaginationInManuallyConstructedQuery(t *testing.T) {
	t.Parallel()

	schema, err := testUserSchema()
	if err != nil {
		t.Fatal(err)
	}
	negative, tooLarge := -1, defaultQueryLimits.MaxLimit+1
	tests := []struct {
		name  string
		query Query
		code  ErrorCode
	}{
		{name: "negative limit", query: Query{Limit: &negative}, code: CodeInvalidParameter},
		{name: "limit over default", query: Query{Limit: &tooLarge}, code: CodeLimitExceeded},
		{name: "negative offset", query: Query{Offset: &negative}, code: CodeInvalidParameter},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			scoped, applyErr := Apply(dryRunDB(t).Model(&schemaTestUser{}), schema, tt.query)
			queryErr, ok := applyErr.(*Error)
			if scoped != nil || !ok || queryErr.Code != tt.code {
				t.Fatalf("Apply() = (%#v, %#v), want nil and %s", scoped, applyErr, tt.code)
			}
		})
	}
}

func TestApplyPreservesCustomParseLimit(t *testing.T) {
	t.Parallel()

	schema, err := testUserSchema()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseHTTP(url.Values{"limit": {"150"}}, WithLimits(Limits{
		MaxQueryBytes:      16 << 10,
		MaxFilterBytes:     8 << 10,
		MaxTokens:          256,
		MaxLiteralBytes:    4 << 10,
		MaxInValues:        100,
		MaxLimit:           200,
		MaxOffset:          100_000,
		MaxSortTerms:       5,
		MaxSearchBytes:     256,
		MaxExpressionDepth: 16,
		MaxNodes:           100,
		MaxPathDepth:       8,
		MaxQuantifierDepth: 4,
		MaxCursorBytes:     4 << 10,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(dryRunDB(t).Model(&schemaTestUser{}), schema, parsed); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
}
