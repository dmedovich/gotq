package query

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestParserPrecedenceAssociativityAndParentheses(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("testdata/parser.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var tests []struct {
		Name  string `json:"name"`
		Input string `json:"input"`
		Tree  string `json:"tree"`
	}
	if err := json.Unmarshal(data, &tests); err != nil {
		t.Fatal(err)
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			expr, err := parseFilter(tt.Input, defaultQueryLimits)
			if err != nil {
				t.Fatalf("parseFilter() error = %v", err)
			}
			if got := formatTestExpr(expr); got != tt.Tree {
				t.Fatalf("AST = %q, want golden %q", got, tt.Tree)
			}
		})
	}
}

func TestParserLiteralValuesAndSpans(t *testing.T) {
	t.Parallel()

	expr, err := parseFilter("name eq 'O''Brien'", defaultQueryLimits)
	if err != nil {
		t.Fatalf("parseFilter() error = %v", err)
	}
	comparison := expr.(*ComparisonExpr)
	if got, want := comparison.Span(), (Span{0, 18}); got != want {
		t.Errorf("comparison span = %#v, want %#v", got, want)
	}
	if got, want := comparison.FieldSource, (Span{0, 4}); got != want {
		t.Errorf("field span = %#v, want %#v", got, want)
	}
	if got, want := comparison.OpSource, (Span{5, 7}); got != want {
		t.Errorf("operator span = %#v, want %#v", got, want)
	}
	if got, want := comparison.Literal.Source, (Span{8, 18}); got != want {
		t.Errorf("literal span = %#v, want %#v", got, want)
	}
	if got, want := comparison.Literal.Value, any("O'Brien"); got != want {
		t.Errorf("literal value = %#v, want %#v", got, want)
	}
}

func TestParserScalarExtensions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{input: "age in (18, 21, 42)", want: "age in (18,21,42)"},
		{input: "age not in (18,21)", want: "age not in (18,21)"},
		{input: "createdAt is null", want: "createdAt is null"},
		{input: "createdAt is not null", want: "createdAt is not null"},
		{input: "name startswith 'Ann'", want: "name startswith 'Ann'"},
		{input: "name endswith 'son'", want: "name endswith 'son'"},
		{input: "not active eq true", want: "not active eq true"},
		{input: "not (age lt 18 or age gt 65)", want: "not (age lt 18 or age gt 65)"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			expr, err := parseFilter(tt.input, defaultQueryLimits)
			if err != nil {
				t.Fatal(err)
			}
			if got := formatTestExpr(expr); got != tt.want {
				t.Fatalf("AST = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParserRelationshipPathsAndQuantifiers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{input: "company/country/code eq 'US'", want: "company/country/code eq 'US'"},
		{input: "orders/any(o: o/total gt 100)", want: "orders/any(o: o/total gt 100)"},
		{input: "orders/all(o: o/status eq 'paid')", want: "orders/all(o: o/status eq 'paid')"},
		{input: "orders/any(o: o/items/any(i: i/price gte 10))", want: "orders/any(o: o/items/any(i: i/price gte 10))"},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			expr, err := parseFilter(test.input, defaultQueryLimits)
			if err != nil {
				t.Fatal(err)
			}
			if got := formatTestExpr(expr); got != test.want {
				t.Fatalf("AST = %q, want %q", got, test.want)
			}
		})
	}
}

func TestParserRejectsMalformedAndOversizedRelationshipSyntax(t *testing.T) {
	t.Parallel()
	for _, input := range []string{
		"company /name eq 'x'",
		"company/ name eq 'x'",
		"orders /any(o: o/id eq 1)",
		"orders/any (o: o/id eq 1)",
		"orders/any(: o/id eq 1)",
		"orders/any(o o/id eq 1)",
		"orders/any(o:o/id eq 1)",
		"orders/any(o: o/id eq 1",
	} {
		if _, err := parseFilter(input, defaultQueryLimits); err == nil {
			t.Errorf("parseFilter(%q) returned nil error", input)
		}
	}
	pathLimits := defaultQueryLimits
	pathLimits.MaxPathDepth = 2
	if _, err := parseFilter("company/country/code eq 'US'", pathLimits); err == nil || err.Code != CodeLimitExceeded {
		t.Fatalf("path limit error = %#v", err)
	}
	pathLimits.MaxPathDepth = 1
	if _, err := parseFilter("orders/any(o: o/total gt 1)", pathLimits); err != nil {
		t.Fatalf("scoped path at inclusive limit failed: %v", err)
	}
	if _, err := parseFilter("company/orders/any(o: o/total gt 1)", pathLimits); err == nil || err.Code != CodeLimitExceeded {
		t.Fatalf("quantifier path limit error = %#v", err)
	}
	quantifierLimits := defaultQueryLimits
	quantifierLimits.MaxQuantifierDepth = 1
	if _, err := parseFilter("orders/any(o: o/items/any(i: i/id eq 1))", quantifierLimits); err == nil || err.Code != CodeLimitExceeded {
		t.Fatalf("quantifier limit error = %#v", err)
	}
}

func TestParserRejectsMalformedAndOversizedInLists(t *testing.T) {
	t.Parallel()
	for _, input := range []string{
		"age in ()", "age in (1,)", "age in (,1)", "age in (1 2)",
		"age in(1)", "age notin (1)", "age is", "age is not", "not(age eq 1)",
	} {
		if _, err := parseFilter(input, defaultQueryLimits); err == nil {
			t.Errorf("parseFilter(%q) returned nil error", input)
		}
	}
	limits := defaultQueryLimits
	limits.MaxInValues = 2
	_, err := parseFilter("age in (1,2,3)", limits)
	if err == nil || err.Code != CodeLimitExceeded || err.Position == nil || err.Position.Offset != 12 {
		t.Fatalf("in-list limit error = %#v", err)
	}
}

func TestParserConsumesCompleteInputAndRequiresWhitespace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input  string
		offset int
	}{
		{input: "   ", offset: 3},
		{input: "age", offset: 3},
		{input: "age gt", offset: 6},
		{input: "active eq TRUE", offset: 10},
		{input: "age gt 18 trailing", offset: 10},
		{input: "age gt 18 and", offset: 13},
		{input: "(age gt 18", offset: 10},
		{input: "age gt 18)", offset: 9},
		{input: "()", offset: 1},
		{input: "age gt(18)", offset: 6},
		{input: "a eq 1and b eq 2", offset: 6},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			_, err := parseFilter(tt.input, defaultQueryLimits)
			if err == nil {
				t.Fatalf("parseFilter(%q) returned nil error", tt.input)
			}
			if err.Code != CodeInvalidSyntax {
				t.Fatalf("error code = %q, want %q", err.Code, CodeInvalidSyntax)
			}
			if err.Position == nil || err.Position.Offset != tt.offset {
				t.Errorf("error position = %#v, want offset %d", err.Position, tt.offset)
			}
		})
	}
}

func TestParserNodeAndDepthLimits(t *testing.T) {
	t.Parallel()

	input := "a eq 1 and b eq 2 and c eq 3"
	tests := []struct {
		name   string
		limits Limits
		offset int
	}{
		{name: "nodes", limits: Limits{MaxLimit: 100, MaxExpressionDepth: 10, MaxNodes: 3}, offset: 18},
		{name: "depth", limits: Limits{MaxLimit: 100, MaxExpressionDepth: 2, MaxNodes: 10}, offset: 18},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseFilter(input, tt.limits)
			if err == nil {
				t.Fatal("parseFilter() returned nil error")
			}
			if err.Code != CodeLimitExceeded || err.Position == nil || err.Position.Offset != tt.offset {
				t.Fatalf("error = %#v, want limit error at %d", err, tt.offset)
			}
		})
	}
}

func TestParserParenthesesDoNotChangeASTLimits(t *testing.T) {
	t.Parallel()

	expr, err := parseFilter("((((age gt 18))))", Limits{MaxLimit: 100, MaxExpressionDepth: 1, MaxNodes: 1})
	if err != nil {
		t.Fatalf("parseFilter() error = %v", err)
	}
	if got, want := expr.Span(), (Span{4, 13}); got != want {
		t.Fatalf("span = %#v, want %#v", got, want)
	}
}

func TestParserBoundsParenthesisNestingIndependently(t *testing.T) {
	t.Parallel()

	input := strings.Repeat("(", maxParenthesisNesting+1) + "age gt 18" + strings.Repeat(")", maxParenthesisNesting+1)
	_, err := parseFilter(input, defaultQueryLimits)
	if err == nil {
		t.Fatal("parseFilter() returned nil error")
	}
	if err.Code != CodeLimitExceeded || err.Position == nil || err.Position.Offset != maxParenthesisNesting {
		t.Fatalf("error = %#v, want limit error at offset %d", err, maxParenthesisNesting)
	}
}

func formatTestExpr(expr Expr) string {
	switch expr := expr.(type) {
	case *ComparisonExpr:
		if expr.Operator == In || expr.Operator == NotIn {
			values := make([]string, len(expr.Literals))
			for i, literal := range expr.Literals {
				values[i] = literal.Raw
			}
			return fmt.Sprintf("%s %s (%s)", expr.Field, expr.Operator, strings.Join(values, ","))
		}
		if expr.Operator == IsNull || expr.Operator == IsNotNull {
			return fmt.Sprintf("%s %s", expr.Field, expr.Operator)
		}
		return fmt.Sprintf("%s %s %s", expr.Field, expr.Operator, expr.Literal.Raw)
	case *LogicalExpr:
		return fmt.Sprintf("(%s %s %s)", formatTestExpr(expr.Left), expr.Operator, formatTestExpr(expr.Right))
	case *NotExpr:
		inner := formatTestExpr(expr.Expr)
		return "not " + inner
	case *QuantifierExpr:
		return fmt.Sprintf("%s/%s(%s: %s)", expr.Relationship, expr.Operator, expr.Variable, formatTestExpr(expr.Predicate))
	default:
		panic(fmt.Sprintf("unexpected expression type %T", expr))
	}
}
