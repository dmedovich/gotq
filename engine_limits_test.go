package query

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	"gorm.io/gorm"
)

func TestEngineApplyEnforcesEndpointLimits(t *testing.T) {
	tests := []struct {
		name, parameter, accepted, rejected string
		configure                           func(*Config[engineTestUser])
		transportOnly                       bool
	}{
		{"limit", "limit", "3", "4", func(c *Config[engineTestUser]) { c.MaxLimit = 3 }, false},
		{"offset", "offset", "10", "11", func(c *Config[engineTestUser]) { c.MaxOffset = 10 }, false},
		{"sort", "sort", "id", "id,name", func(c *Config[engineTestUser]) { c.MaxSortTerms = 1 }, false},
		{"search", "search", "abc", "abcd", func(c *Config[engineTestUser]) { c.MaxSearchBytes = 3 }, false},
		{"literal", "filter", "name eq 'ab'", "name eq 'abc'", func(c *Config[engineTestUser]) { c.MaxLiteralBytes = 4 }, false},
		{"set", "filter", "age in (1,2)", "age in (1,2,3)", func(c *Config[engineTestUser]) { c.MaxInValues = 2 }, false},
		{"nodes", "filter", "age eq 1", "age eq 1 or age eq 2", func(c *Config[engineTestUser]) { c.MaxNodes = 1 }, false},
		{"depth", "filter", "not age eq 1", "not not age eq 1", func(c *Config[engineTestUser]) { c.MaxExpressionDepth = 2 }, false},
		{"filter bytes", "filter", "age eq 1", "age eq 10", func(c *Config[engineTestUser]) { c.MaxFilterBytes = 8 }, true},
		{"tokens", "filter", "age eq 1", "(age eq 1)", func(c *Config[engineTestUser]) { c.MaxTokens = 3 }, true},
		{"query bytes", "app", "1234567", "12345678", func(c *Config[engineTestUser]) { c.MaxQueryBytes = 10 }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newEngineTestDB(t)
			callbacks := 0
			config := Config[engineTestUser]{
				Policy:       Schema[engineTestUser]().Expose("id", Filterable(), Sortable()).Expose("name", Filterable(), Sortable()).Expose("age", Filterable(), Sortable()),
				DefaultLimit: 1, MaxLimit: 100, MaxOffset: 1000,
				Search: func(_ context.Context, scope *gorm.DB, _ string) (*gorm.DB, error) { callbacks++; return scope, nil },
			}
			tt.configure(&config)
			engine, err := New(db, config)
			if err != nil {
				t.Fatal(err)
			}
			for _, manual := range []bool{false, true} {
				if manual && tt.transportOnly {
					continue
				}
				for _, accepted := range []bool{true, false} {
					input := tt.rejected
					if accepted {
						input = tt.accepted
					}
					q, err := ParseHTTP(url.Values{tt.parameter: {input}})
					if err != nil {
						t.Fatal(err)
					}
					if manual {
						q.limits = Limits{}
						q.filterSource = ""
						q.sortSource = ""
						q.queryBytes = 0
					}
					before := callbacks
					_, err = engine.From(db).Apply(context.Background(), q)
					if accepted {
						if err != nil {
							t.Fatalf("accepted boundary (manual=%v): %v", manual, err)
						}
					} else {
						var qe *Error
						if !errors.As(err, &qe) || qe.Code != CodeLimitExceeded {
							t.Fatalf("manual=%v: got %v, want limit_exceeded", manual, err)
						}
						if callbacks != before {
							t.Fatal("rejected query invoked search")
						}
					}
				}
			}
		})
	}
}

func TestEngineApplyUsesReceivingPolicyAndRetainsStricterParserLimits(t *testing.T) {
	db := newEngineTestDB(t)
	small := newUserEngine(t, db)
	config := small.config
	config.MaxLimit = 1000
	large, err := New(db, config)
	if err != nil {
		t.Fatal(err)
	}
	q, err := large.Parse(url.Values{"limit": {"50"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = small.Apply(context.Background(), db, q); err == nil {
		t.Fatal("receiving engine accepted another engine's larger limit")
	}
	if _, err = large.Apply(context.Background(), db, q); err != nil {
		t.Fatal("validation mutated caller query:", err)
	}
	limits := defaultQueryLimits
	limits.MaxLimit = 2
	q, err = ParseHTTP(url.Values{"limit": {"2"}}, WithLimits(limits))
	if err != nil {
		t.Fatal(err)
	}
	larger := 3
	q.Limit = &larger
	if _, err = large.Apply(context.Background(), db, q); err == nil {
		t.Fatal("discarded stricter parser limit on edited query")
	}
	// Manually constructed queries use the endpoint policy, not global defaults.
	limit := 500
	if _, err = large.Apply(context.Background(), db, Query{Limit: &limit}); err != nil {
		t.Fatal(err)
	}
	// Standalone Apply continues to use the query/parser policy, not an engine.
	q, err = ParseHTTP(url.Values{"limit": {"50"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Apply(db, small.schema, q); err != nil {
		t.Fatal(err)
	}
}

func TestEngineApplyBoundsRelationshipsAndCursor(t *testing.T) {
	db, source := seededRelationshipEngine(t)
	for _, tc := range []struct {
		name, input string
		configure   func(*Config[relationshipTestUser])
	}{
		{"filter path", "company/country/code eq 'US'", func(c *Config[relationshipTestUser]) { c.MaxPathDepth = 2 }},
		{"quantifier", "orders/any(o: o/items/any(i: i/price gte 10))", func(c *Config[relationshipTestUser]) { c.MaxQuantifierDepth = 1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := source.config
			tc.configure(&config)
			receiver, err := New(db, config)
			if err != nil {
				t.Fatal(err)
			}
			q, err := source.Parse(url.Values{"filter": {tc.input}})
			if err != nil {
				t.Fatal(err)
			}
			for _, manual := range []bool{false, true} {
				if manual {
					q.limits = Limits{}
					q.filterSource = ""
					q.queryBytes = 0
				}
				_, err = receiver.Apply(context.Background(), db, q)
				var qe *Error
				if !errors.As(err, &qe) || qe.Code != CodeLimitExceeded {
					t.Fatalf("got %v", err)
				}
			}
		})
	}
	page, err := source.List(context.Background(), url.Values{"limit": {"1"}})
	if err != nil {
		t.Fatal(err)
	}
	cursor := page.Page.NextCursor
	if cursor == "" {
		t.Fatal("missing test cursor")
	}
	config := source.config
	config.MaxCursorBytes = len(cursor)
	receiver, err := New(db, config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = receiver.Apply(context.Background(), db, Query{Cursor: &cursor}); err != nil {
		t.Fatal(err)
	}
	config.MaxCursorBytes--
	receiver, err = New(db, config)
	if err != nil {
		t.Fatal(err)
	}
	_, err = receiver.Apply(context.Background(), db, Query{Cursor: &cursor})
	var qe *Error
	if !errors.As(err, &qe) || qe.Code != CodeLimitExceeded {
		t.Fatalf("got %v", err)
	}
}

func TestEngineApplyBoundsDecodedLiteralWithoutRawSyntax(t *testing.T) {
	db := newEngineTestDB(t)
	engine := newUserEngine(t, db)
	value := strings.Repeat("a", engine.limits.MaxLiteralBytes+1)
	for _, expr := range []*ComparisonExpr{
		{Field: "name", Operator: Eq, Literal: Literal{Kind: StringLiteral, Value: value}},
		{Field: "name", Operator: In, Literals: []Literal{{Kind: StringLiteral, Value: value}}},
	} {
		_, err := engine.Apply(context.Background(), db, Query{Filter: expr})
		var qe *Error
		if !errors.As(err, &qe) || qe.Code != CodeLimitExceeded {
			t.Fatalf("got %v", err)
		}
	}
}
