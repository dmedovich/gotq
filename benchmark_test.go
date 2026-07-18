package query

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var benchmarkValues = url.Values{
	"filter": {"age gte 18 and name contains 'ann'"},
	"sort":   {"-age,name"},
	"limit":  {"25"},
}

func BenchmarkParseHTTP(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := ParseHTTP(benchmarkValues); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseMaximumValidFilter(b *testing.B) {
	values := url.Values{"filter": {balancedBenchmarkFilter(50)}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ParseHTTP(values); err != nil {
			b.Fatal(err)
		}
	}
}

func balancedBenchmarkFilter(leaves int) string {
	if leaves <= 1 {
		return "age gte 18"
	}
	left := leaves / 2
	return "(" + balancedBenchmarkFilter(left) + " and " + balancedBenchmarkFilter(leaves-left) + ")"
}

func BenchmarkRejectOversizedFilter(b *testing.B) {
	values := url.Values{"filter": {strings.Repeat("x", defaultQueryLimits.MaxFilterBytes+1)}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ParseHTTP(values); err == nil {
			b.Fatal("oversized filter was accepted")
		}
	}
}

func BenchmarkApply(b *testing.B) {
	db := dryRunDB(b)
	schema, err := Schema[schemaTestUser]().
		Expose("id", Sortable()).
		Expose("name", Filterable(), Sortable()).
		Expose("age", Filterable(), Sortable()).
		Bind(db)
	if err != nil {
		b.Fatal(err)
	}
	q, err := ParseHTTP(benchmarkValues)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Apply(db.Model(&schemaTestUser{}), schema, q); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEngineParseApply(b *testing.B) {
	db := dryRunDB(b)
	engine, err := New(db, Config[schemaTestUser]{
		Policy: Schema[schemaTestUser]().
			Expose("id", Sortable()).
			Expose("name", Filterable(), Sortable()).
			Expose("age", Filterable(), Sortable()),
		DefaultLimit: 25,
		MaxLimit:     100,
		MaxOffset:    100_000,
	})
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q, err := engine.Parse(benchmarkValues)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := engine.Apply(ctx, db, q); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEngineNewSchemaCacheHit(b *testing.B) {
	db := dryRunDB(b)
	policy := Schema[schemaTestUser]().
		Expose("id", Sortable()).
		Expose("name", Filterable(), Sortable()).
		Expose("age", Filterable(), Sortable())
	config := Config[schemaTestUser]{Policy: policy, DefaultLimit: 25, MaxLimit: 100, MaxOffset: 100_000}
	if _, err := New(db, config); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := New(db, config); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEngineListSQLite(b *testing.B) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		b.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		b.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&engineTestUser{}); err != nil {
		b.Fatal(err)
	}
	for i := 1; i <= 100; i++ {
		if err := db.Create(&engineTestUser{ID: uint(i), TenantID: 1, Name: "ann", Age: 18 + i%50}).Error; err != nil {
			b.Fatal(err)
		}
	}
	engine, err := New(db, Config[engineTestUser]{
		Policy: Schema[engineTestUser]().
			Expose("id", Sortable()).
			Expose("name", Filterable(), Sortable()).
			Expose("age", Filterable(), Sortable()),
		DefaultLimit: 25,
		MaxLimit:     100,
		MaxOffset:    100_000,
	})
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := engine.List(ctx, benchmarkValues); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRelationshipParseAndApply(b *testing.B) {
	db := dryRunDB(b)
	policy, _ := relationshipPolicies()
	schema, err := policy.Bind(db)
	if err != nil {
		b.Fatal(err)
	}
	values := url.Values{
		"filter": {"company/country/code eq 'US' and orders/any(o: o/items/any(i: i/price gte 10))"},
		"sort":   {"company/name"},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		parsed, parseErr := ParseHTTP(values)
		if parseErr != nil {
			b.Fatal(parseErr)
		}
		if _, applyErr := Apply(db.Model(&relationshipTestUser{}), schema, parsed); applyErr != nil {
			b.Fatal(applyErr)
		}
	}
}

func BenchmarkCursorParseAndApply(b *testing.B) {
	db := dryRunDB(b)
	engine, err := New(db, Config[dialectTestUser]{Policy: dialectPolicy(), DefaultLimit: 25, MaxLimit: 100, MaxOffset: 100_000})
	if err != nil {
		b.Fatal(err)
	}
	parsed, err := engine.Parse(url.Values{"sort": {"-age,name"}})
	if err != nil {
		b.Fatal(err)
	}
	validated, err := engine.validate(parsed)
	if err != nil {
		b.Fatal(err)
	}
	orders, orderErr := effectiveSort(engine.schema.modelSchema, validated.sort)
	if orderErr != nil {
		b.Fatal(orderErr)
	}
	cursor, err := encodeCursor(engine.schema.modelSchema, orders, []any{30, "Alice", uint(1)}, engine.limits.MaxCursorBytes)
	if err != nil {
		b.Fatal(err)
	}
	values := url.Values{"cursor": {cursor}, "sort": {"-age,name"}}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		query, parseErr := engine.Parse(values)
		if parseErr != nil {
			b.Fatal(parseErr)
		}
		if _, applyErr := engine.Apply(ctx, db, query); applyErr != nil {
			b.Fatal(applyErr)
		}
	}
}
