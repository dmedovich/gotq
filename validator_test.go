package query

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestValidatorConvertsLiteralsToFieldTypes(t *testing.T) {
	t.Parallel()

	schema, err := testUserSchema()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		filter string
		want   any
	}{
		{filter: "name eq 'ann'", want: "ann"},
		{filter: "active eq true", want: true},
		{filter: "age gt -18", want: int8(-18)},
		{filter: "quota eq 42", want: uint16(42)},
		{filter: "score gte -1.5e2", want: float32(-150)},
		{filter: "createdAt gt '2026-07-16T12:30:00Z'", want: time.Date(2026, 7, 16, 12, 30, 0, 0, time.UTC)},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.filter, func(t *testing.T) {
			t.Parallel()
			expr, parseErr := parseFilter(tt.filter, defaultQueryLimits)
			if parseErr != nil {
				t.Fatal(parseErr)
			}
			validated, validationErr := validateQuery(schema, Query{Filter: expr, filterSource: tt.filter})
			if validationErr != nil {
				t.Fatal(validationErr)
			}
			comparison := validated.filter.(*validatedComparison)
			if !reflect.DeepEqual(comparison.value, tt.want) {
				t.Fatalf("value = %#v (%T), want %#v (%T)", comparison.value, comparison.value, tt.want, tt.want)
			}
		})
	}
}

func TestValidatorConvertsToNamedGoType(t *testing.T) {
	t.Parallel()

	type model struct {
		Alias schemaTestAlias `json:"alias" gorm:"column:alias"`
	}
	schema, err := Schema[model]().Field("alias", String, Filterable()).Build()
	if err != nil {
		t.Fatal(err)
	}
	filter := "alias eq 'value'"
	expr, parseErr := parseFilter(filter, defaultQueryLimits)
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	validated, validationErr := validateQuery(schema, Query{Filter: expr, filterSource: filter})
	if validationErr != nil {
		t.Fatal(validationErr)
	}
	value := validated.filter.(*validatedComparison).value
	if got, ok := value.(schemaTestAlias); !ok || got != "value" {
		t.Fatalf("value = %#v (%T), want schemaTestAlias", value, value)
	}
}

func TestValidatorRejectsSchemaViolationsBeforeCompilation(t *testing.T) {
	t.Parallel()

	schema, err := testUserSchema()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		filter string
		code   ErrorCode
		offset int
	}{
		{filter: "missing eq 1", code: CodeUnknownField, offset: 0},
		{filter: "id eq 1", code: CodeNotFilterable, offset: 0},
		{filter: "age contains 'ann'", code: CodeOperatorNotAllowed, offset: 4},
		{filter: "age eq '18'", code: CodeInvalidLiteral, offset: 7},
		{filter: "name eq 18", code: CodeInvalidLiteral, offset: 8},
		{filter: "createdAt gt 'yesterday'", code: CodeInvalidLiteral, offset: 13},
		{filter: "name gt 'ann'", code: CodeOperatorNotAllowed, offset: 5},
		{filter: "age eq 128", code: CodeInvalidLiteral, offset: 7},
		{filter: "quota eq -1", code: CodeInvalidLiteral, offset: 9},
		{filter: "score eq 1e999", code: CodeInvalidLiteral, offset: 9},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.filter, func(t *testing.T) {
			t.Parallel()
			expr, parseErr := parseFilter(tt.filter, defaultQueryLimits)
			if parseErr != nil {
				t.Fatalf("syntax-valid vector failed parsing: %v", parseErr)
			}
			_, validationErr := validateQuery(schema, Query{Filter: expr, filterSource: tt.filter})
			if validationErr == nil {
				t.Fatal("validateQuery() returned nil error")
			}
			if validationErr.Code != tt.code || validationErr.Position == nil || validationErr.Position.Offset != tt.offset {
				t.Fatalf("error = %#v, want code %q at %d", validationErr, tt.code, tt.offset)
			}
		})
	}
}

func TestValidatorRejectsMalformedPublicQuantifierAST(t *testing.T) {
	db := relationshipDB(t)
	policy, _ := relationshipPolicies()
	schema, err := policy.Bind(db)
	if err != nil {
		t.Fatal(err)
	}
	cycle := &QuantifierExpr{
		Relationship: "orders",
		Operator:     Any,
		Variable:     "o",
		Source:       Span{Start: 0, End: 1},
	}
	cycle.Predicate = cycle
	tests := []Expr{
		cycle,
		&QuantifierExpr{Relationship: "orders", Operator: Any, Variable: "o", Source: Span{0, 1}},
		&QuantifierExpr{Relationship: "orders", Operator: QuantifierOperator(99), Variable: "o", Predicate: &ComparisonExpr{}, Source: Span{0, 1}},
		&QuantifierExpr{Relationship: "orders", Operator: Any, Variable: "any", Predicate: &ComparisonExpr{}, Source: Span{0, 1}},
	}
	for _, expression := range tests {
		if _, validationErr := validateQuery(schema, Query{Filter: expression}); validationErr == nil || validationErr.Code != CodeInvalidSyntax {
			t.Fatalf("malformed quantifier %#v error = %#v", expression, validationErr)
		}
	}
}

func TestValidatorOperatorErrorIncludesDeterministicAllowedSet(t *testing.T) {
	t.Parallel()

	schema, err := testUserSchema()
	if err != nil {
		t.Fatal(err)
	}
	filter := "age contains 'ann'"
	expr, parseErr := parseFilter(filter, defaultQueryLimits)
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	_, validationErr := validateQuery(schema, Query{Filter: expr, filterSource: filter})
	if validationErr == nil {
		t.Fatal("validateQuery() returned nil error")
	}
	if got, want := joinOperators(validationErr.AllowedOperators), "eq, ne, gt, gte, lt, lte, in, not in"; got != want {
		t.Fatalf("allowed operators = %q, want %q", got, want)
	}
	for _, part := range []string{
		`operator "contains" cannot be used with field "age" of type int`,
		`allowed operators: eq, ne, gt, gte, lt, lte, in, not in`,
	} {
		if !strings.Contains(validationErr.Error(), part) {
			t.Errorf("error %q does not contain %q", validationErr, part)
		}
	}
}

func TestValidatorChecksOrderFields(t *testing.T) {
	t.Parallel()

	schema, err := testUserSchema()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		field string
		code  ErrorCode
	}{
		{field: "missing", code: CodeUnknownField},
		{field: "age", code: CodeNotSortable},
	}
	for _, tt := range tests {
		_, validationErr := validateQuery(schema, Query{
			Sort:       []SortTerm{{Field: tt.field, Source: Span{0, len(tt.field)}}},
			sortSource: tt.field,
		})
		if validationErr == nil || validationErr.Code != tt.code {
			t.Errorf("field %q error = %#v, want %q", tt.field, validationErr, tt.code)
		}
	}
}

func TestValidatorRejectsMalformedPublicASTWithoutPanic(t *testing.T) {
	t.Parallel()

	schema, err := testUserSchema()
	if err != nil {
		t.Fatal(err)
	}
	tests := []Query{
		{Filter: (*ComparisonExpr)(nil)},
		{Filter: (*NotExpr)(nil)},
		{Filter: &NotExpr{Expr: nil, Source: Span{0, 3}}},
		{Filter: &LogicalExpr{Operator: LogicalOperator(99), Source: Span{0, 1}}},
		{Filter: &LogicalExpr{Operator: And, Left: nil, Right: nil, Source: Span{0, 1}}},
		{Filter: &ComparisonExpr{
			Field:       "name",
			Operator:    Eq,
			Literal:     Literal{Kind: StringLiteral, Value: 42, Source: Span{8, 10}},
			Source:      Span{0, 10},
			FieldSource: Span{0, 4},
			OpSource:    Span{5, 7},
		}},
	}
	for i, query := range tests {
		if _, validationErr := validateQuery(schema, query); validationErr == nil {
			t.Errorf("case %d returned nil error", i)
		}
	}
}

func TestValidatorEnforcesLimitsAndCyclesOnPublicAST(t *testing.T) {
	t.Parallel()

	schema, err := testUserSchema()
	if err != nil {
		t.Fatal(err)
	}
	comparison := func() Expr {
		return &ComparisonExpr{
			Field:       "active",
			Operator:    Eq,
			Literal:     Literal{Kind: BoolLiteral, Raw: "true", Value: true, Source: Span{10, 14}},
			Source:      Span{0, 14},
			FieldSource: Span{0, 6},
			OpSource:    Span{7, 9},
		}
	}

	deep := comparison()
	for range defaultQueryLimits.MaxExpressionDepth {
		deep = &LogicalExpr{Operator: And, Left: deep, Right: comparison(), Source: Span{0, 14}}
	}
	if _, validationErr := validateQuery(schema, Query{Filter: deep}); validationErr == nil || validationErr.Code != CodeLimitExceeded {
		t.Fatalf("deep AST error = %#v, want limit_exceeded", validationErr)
	}

	cycle := &LogicalExpr{Operator: And, Right: comparison(), Source: Span{0, 14}}
	cycle.Left = cycle
	if _, validationErr := validateQuery(schema, Query{Filter: cycle}); validationErr == nil || validationErr.Code != CodeInvalidSyntax {
		t.Fatalf("cyclic AST error = %#v, want invalid_syntax", validationErr)
	}
	notCycle := &NotExpr{Source: Span{0, 3}}
	notCycle.Expr = notCycle
	if _, validationErr := validateQuery(schema, Query{Filter: notCycle}); validationErr == nil || validationErr.Code != CodeInvalidSyntax {
		t.Fatalf("cyclic not AST error = %#v, want invalid_syntax", validationErr)
	}
}
