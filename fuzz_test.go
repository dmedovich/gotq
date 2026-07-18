package query

import (
	"context"
	"net/url"
	"testing"
)

func FuzzFilterLexer(f *testing.F) {
	for _, seed := range []string{
		"age gt 18 and name contains 'ann'",
		"name eq 'O''Brien'",
		"city eq 'Волгоград'",
		"score gte -1.5e2",
		"id in (1,2,3) and deletedAt is null",
		"not name startswith 'A'",
		"orders/any(o: o/items/any(i: i/price gte 10))",
		"name eq 'unterminated",
		"user.name eq 'ann'",
		string([]byte{'a', 0xff, 'b'}),
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		tokens, err := lexFilter(input)
		if err != nil {
			if err.Position == nil || err.Position.Offset < 0 || err.Position.Offset > len(input) {
				t.Fatalf("invalid error position %#v for input length %d", err.Position, len(input))
			}
			return
		}
		if len(tokens) == 0 || tokens[len(tokens)-1].kind != tokenEOF {
			t.Fatalf("successful token stream has no EOF: %#v", tokens)
		}
		previousEnd := 0
		for i, token := range tokens {
			if token.span.Start < previousEnd || token.span.Start > token.span.End || token.span.End > len(input) {
				t.Fatalf("token[%d] has invalid/non-monotonic span %#v after %d", i, token.span, previousEnd)
			}
			if token.kind != tokenEOF && token.span.Start == token.span.End {
				t.Fatalf("non-EOF token[%d] did not advance", i)
			}
			previousEnd = token.span.End
		}
	})
}

func FuzzQueryPipeline(f *testing.F) {
	for _, seed := range [][2]string{
		{"age gt 18", "-age"},
		{"name eq 'O''Brien'", "name"},
		{"name contains 'x%_!'", "id"},
		{"age in (18,21) and not name endswith 'bot'", "id"},
		{"age gt 18 trailing", "missing"},
	} {
		f.Add(seed[0], seed[1])
	}
	db := dryRunDB(f)
	schema, err := Schema[schemaTestUser]().
		Expose("id", Sortable()).
		Expose("name", Filterable(), Sortable()).
		Expose("age", Filterable(), Sortable()).
		Bind(db)
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, filter, sort string) {
		q, parseErr := ParseHTTP(url.Values{"filter": {filter}, "sort": {sort}})
		if parseErr != nil {
			return
		}
		scoped, applyErr := Apply(db.Model(&schemaTestUser{}), schema, q)
		if applyErr != nil {
			return
		}
		statement := scoped.Find(&[]schemaTestUser{}).Statement
		if statement.Error != nil || statement.SQL.Len() == 0 {
			t.Fatalf("compiled statement = %#v", statement)
		}
	})
}

func FuzzFilterParser(f *testing.F) {
	for _, seed := range []string{
		"age gt 18",
		"a eq 1 or b eq 2 and c eq 3",
		"(a eq 1 or b eq 2) and c eq 3",
		"((((age gt 18))))",
		"age not in (18,21) and not active eq false",
		"orders/all(o: o/status eq 'paid')",
		"a eq 1and b eq 2",
		"age gt 18 trailing",
		"",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		expr, err := parseFilter(input, defaultQueryLimits)
		if err != nil {
			if err.Position == nil || err.Position.Offset < 0 || err.Position.Offset > len(input) {
				t.Fatalf("invalid error position %#v for input length %d", err.Position, len(input))
			}
			return
		}
		if expr == nil {
			t.Fatal("successful parse returned nil expression")
		}
		nodes, depth := assertFuzzAST(t, expr, len(input))
		if nodes > defaultQueryLimits.MaxNodes || depth > defaultQueryLimits.MaxExpressionDepth {
			t.Fatalf("successful AST exceeds limits: nodes=%d depth=%d", nodes, depth)
		}
	})
}

func FuzzRelationshipPipeline(f *testing.F) {
	for _, seed := range [][2]string{
		{"company/country/code eq 'US'", "company/name"},
		{"orders/any(o: o/total gt 100)", "id"},
		{"orders/all(o: o/status eq 'paid')", "-company/name"},
		{"roles/any(r: r/name eq 'admin')", "company/country/code"},
		{"orders/any(o: o/items/any(i: i/price gte 10))", "id"},
	} {
		f.Add(seed[0], seed[1])
	}
	db := dryRunDB(f)
	policy, _ := relationshipPolicies()
	schema, err := policy.Bind(db)
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, filter, sort string) {
		parsed, parseErr := ParseHTTP(url.Values{"filter": {filter}, "sort": {sort}})
		if parseErr != nil {
			return
		}
		scope, applyErr := Apply(db.Model(&relationshipTestUser{}), schema, parsed)
		if applyErr != nil {
			return
		}
		statement := scope.Find(&[]relationshipTestUser{}).Statement
		if statement.Error != nil || statement.SQL.Len() == 0 {
			t.Fatalf("compiled relationship statement = %#v", statement)
		}
	})
}

func FuzzCursorPipeline(f *testing.F) {
	db := dryRunDB(f)
	engine, err := New(db, Config[dialectTestUser]{Policy: dialectPolicy(), DefaultLimit: 10, MaxLimit: 100, MaxOffset: 100})
	if err != nil {
		f.Fatal(err)
	}
	parsed, err := engine.Parse(url.Values{"sort": {"-age"}})
	if err != nil {
		f.Fatal(err)
	}
	validated, err := engine.validate(parsed)
	if err != nil {
		f.Fatal(err)
	}
	orders, orderErr := effectiveSort(engine.schema.modelSchema, validated.sort)
	if orderErr != nil {
		f.Fatal(orderErr)
	}
	valid, err := encodeCursor(engine.schema.modelSchema, orders, []any{30, uint(1)}, engine.limits.MaxCursorBytes)
	if err != nil {
		f.Fatal(err)
	}
	for _, seed := range [][2]string{{valid, "-age"}, {"not+base64", "age"}, {"e30", "name"}, {valid[:len(valid)-1], "-age"}} {
		f.Add(seed[0], seed[1])
	}
	f.Fuzz(func(t *testing.T, cursor, sort string) {
		query, parseErr := engine.Parse(url.Values{"cursor": {cursor}, "sort": {sort}})
		if parseErr != nil {
			return
		}
		scope, applyErr := engine.Apply(context.Background(), db, query)
		if applyErr != nil {
			if queryErr, ok := applyErr.(*Error); ok && queryErr.Position != nil {
				inputLength := len(cursor)
				if queryErr.Parameter == "sort" {
					inputLength = len(sort)
				}
				if queryErr.Position.Offset < 0 || queryErr.Position.Offset > inputLength {
					t.Fatalf("invalid %s error position %#v for input length %d", queryErr.Parameter, queryErr.Position, inputLength)
				}
			}
			return
		}
		statement := scope.Find(&[]dialectTestUser{}).Statement
		if statement.Error != nil || statement.SQL.Len() == 0 {
			t.Fatalf("compiled cursor statement = %#v", statement)
		}
	})
}

func assertFuzzAST(t *testing.T, expr Expr, inputLength int) (nodes, depth int) {
	t.Helper()
	span := expr.Span()
	if span.Start < 0 || span.Start >= span.End || span.End > inputLength {
		t.Fatalf("AST node %T has invalid span %#v for input length %d", expr, span, inputLength)
	}
	switch expr := expr.(type) {
	case *ComparisonExpr:
		return 1, 1
	case *LogicalExpr:
		leftNodes, leftDepth := assertFuzzAST(t, expr.Left, inputLength)
		rightNodes, rightDepth := assertFuzzAST(t, expr.Right, inputLength)
		return 1 + leftNodes + rightNodes, 1 + max(leftDepth, rightDepth)
	case *NotExpr:
		nodes, depth := assertFuzzAST(t, expr.Expr, inputLength)
		return nodes + 1, depth + 1
	case *QuantifierExpr:
		nodes, depth := assertFuzzAST(t, expr.Predicate, inputLength)
		return nodes + 1, depth + 1
	default:
		t.Fatalf("unexpected AST node %T", expr)
		return 0, 0
	}
}
