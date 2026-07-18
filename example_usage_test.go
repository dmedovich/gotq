package query_test

import (
	"context"
	"fmt"
	"net/url"

	query "github.com/dmedovich/gotq"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func ExampleParseHTTP() {
	values := url.Values{
		"filter": {"age gte 18 and name contains 'ann'"},
		"sort":   {"-createdAt,name"},
		"limit":  {"20"},
	}

	parsed, err := query.ParseHTTP(values)
	if err != nil {
		panic(err)
	}

	fmt.Println(parsed.Filter != nil, len(parsed.Sort), *parsed.Limit)
	// Output: true 2 20
}

func ExampleEngine_List() {
	db, err := gorm.Open(sqlite.Open("file:example-list?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		panic(err)
	}
	if err := db.AutoMigrate(&User{}); err != nil {
		panic(err)
	}
	if err := db.Create(&[]User{
		{Name: "Alice", Age: 31},
		{Name: "Bob", Age: 16},
		{Name: "Cara", Age: 24},
		{Name: "Daria", Age: 28},
	}).Error; err != nil {
		panic(err)
	}

	users, err := buildUserEngine(db)
	if err != nil {
		panic(err)
	}
	page, err := users.List(
		context.Background(),
		url.Values{
			"filter": {"age gte 18"},
			"sort":   {"name"},
			"limit":  {"2"},
		},
	)
	if err != nil {
		panic(err)
	}

	fmt.Println(page.Items[0].Name, page.Items[1].Name, page.Page.HasMore)
	// Output: Alice Cara true
}

type exampleTenantUser struct {
	ID       uint   `json:"id"`
	TenantID uint   `json:"-"`
	Name     string `json:"name"`
}

func ExampleEngine_From() {
	db, err := gorm.Open(sqlite.Open("file:example-tenant?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		panic(err)
	}
	if err := db.AutoMigrate(&exampleTenantUser{}); err != nil {
		panic(err)
	}
	if err := db.Create(&[]exampleTenantUser{
		{TenantID: 1, Name: "Alice"},
		{TenantID: 1, Name: "Cara"},
		{TenantID: 2, Name: "Mallory"},
	}).Error; err != nil {
		panic(err)
	}

	users, err := query.New(db, query.Config[exampleTenantUser]{
		Policy: query.Schema[exampleTenantUser]().
			Expose("id", query.Sortable()).
			Expose("name", query.Filterable(query.Eq), query.Sortable()),
		DefaultLimit: 20,
		MaxLimit:     100,
		MaxOffset:    1_000,
		AllowCount:   true,
	})
	if err != nil {
		panic(err)
	}

	base := db.Where("tenant_id = ?", 1)
	page, err := users.From(base).List(
		context.Background(),
		url.Values{
			"sort":  {"name"},
			"count": {"true"},
		},
	)
	if err != nil {
		panic(err)
	}

	fmt.Println(page.Items[0].Name, page.Items[1].Name, *page.Total)
	// Output: Alice Cara 2
}
