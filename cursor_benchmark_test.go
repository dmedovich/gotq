package query

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type cursorBenchmarkCompany struct {
	ID   uint    `json:"id"`
	Name *string `json:"name"`
}
type cursorBenchmarkUser struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	CompanyID *uint
	Company   cursorBenchmarkCompany `json:"company"`
}

// BenchmarkCursorPage exercises a continuation page, including row scanning,
// keyset predicates, duplicate/null sort keys, and the limit+1 lookahead.
func BenchmarkCursorPage(b *testing.B) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		b.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		b.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	b.Cleanup(func() { _ = sqlDB.Close() })
	if err = db.AutoMigrate(&cursorBenchmarkCompany{}, &cursorBenchmarkUser{}); err != nil {
		b.Fatal(err)
	}
	companies := make([]cursorBenchmarkCompany, 10)
	for i := range companies {
		name := fmt.Sprintf("company-%02d", i)
		companies[i] = cursorBenchmarkCompany{ID: uint(i + 1), Name: &name}
	}
	companies[9].Name = nil
	if err = db.Create(&companies).Error; err != nil {
		b.Fatal(err)
	}
	users := make([]cursorBenchmarkUser, 1000)
	for i := range users {
		id := uint(i%10 + 1)
		users[i] = cursorBenchmarkUser{ID: uint(i + 1), Name: fmt.Sprintf("name-%02d", i%20), CompanyID: &id}
		if i%11 == 0 {
			users[i].CompanyID = nil
		}
	}
	if err = db.CreateInBatches(&users, 100).Error; err != nil {
		b.Fatal(err)
	}
	policy := Schema[cursorBenchmarkUser]().Expose("id", Sortable()).Expose("name", Sortable()).Relation("company", Schema[cursorBenchmarkCompany]().Expose("name", Sortable()))
	engine, err := New(db, Config[cursorBenchmarkUser]{Policy: policy, DefaultLimit: 25, MaxLimit: 100, MaxOffset: 1000})
	if err != nil {
		b.Fatal(err)
	}
	for _, sort := range []string{"id", "name", "-company/name"} {
		for _, limit := range []int{1, 25, 100} {
			b.Run(fmt.Sprintf("%s/limit=%d", sort, limit), func(b *testing.B) {
				values := url.Values{"sort": {sort}, "limit": {strconv.Itoa(limit)}}
				first, err := engine.List(context.Background(), values)
				if err != nil {
					b.Fatal(err)
				}
				if first.Page.NextCursor == "" {
					b.Fatal("missing continuation cursor")
				}
				values.Set("cursor", first.Page.NextCursor)
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					if _, err := engine.List(context.Background(), values); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
