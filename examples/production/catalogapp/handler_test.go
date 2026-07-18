package catalogapp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	query "github.com/dmedovich/gotq"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRelationshipHandlerQuantifiersCountAndCursor(t *testing.T) {
	handler := seededHandler(t)
	filter := "reviews/any(r: r/rating gte 4) and tags/any(t: t/name eq 'sale')"
	first := request(t, handler, url.Values{
		"filter": {filter},
		"sort":   {"brand/name,-createdAt"},
		"limit":  {"1"},
		"count":  {"true"},
	})
	if first.Total == nil || *first.Total != 2 || !reflect.DeepEqual(productIDs(first.Items), []uint{1}) || first.Page.NextCursor == "" {
		t.Fatalf("first relationship page = %#v", first)
	}

	second := request(t, handler, url.Values{
		"filter": {filter},
		"sort":   {"brand/name,-createdAt"},
		"limit":  {"1"},
		"cursor": {first.Page.NextCursor},
	})
	if got := productIDs(second.Items); !reflect.DeepEqual(got, []uint{3}) || second.Page.HasMore {
		t.Fatalf("second relationship page IDs = %v, page = %#v", got, second.Page)
	}
}

func TestRelationshipHandlerCombinesToOneAndToManyPolicy(t *testing.T) {
	handler := seededHandler(t)
	page := request(t, handler, url.Values{
		"filter": {"brand/name eq 'Acme' and reviews/all(r: r/rating gte 4) and tags/any(t: t/name eq 'sale')"},
		"sort":   {"sku"},
	})
	if got := productIDs(page.Items); !reflect.DeepEqual(got, []uint{1}) {
		t.Fatalf("combined relationship IDs = %v", got)
	}

	request := httptest.NewRequest(http.MethodGet, "/products?filter=tags/name%20eq%20%27sale%27", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid collection path status = %d, body = %s", response.Code, response.Body.String())
	}
}

func seededHandler(t *testing.T) *Handler {
	t.Helper()
	dsn := fmt.Sprintf("file:catalog-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&Brand{}, &Product{}, &Review{}, &Tag{}); err != nil {
		t.Fatal(err)
	}

	brands := []Brand{{ID: 1, Name: "Acme"}, {ID: 2, Name: "Beta"}}
	tags := []Tag{{ID: 1, Name: "sale"}, {ID: 2, Name: "regular"}}
	base := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	products := []Product{
		{ID: 1, SKU: "A-1", Price: 100, BrandID: 1, CreatedAt: base.Add(time.Hour)},
		{ID: 2, SKU: "A-2", Price: 200, BrandID: 1, CreatedAt: base.Add(2 * time.Hour)},
		{ID: 3, SKU: "B-1", Price: 300, BrandID: 2, CreatedAt: base.Add(3 * time.Hour)},
	}
	reviews := []Review{
		{ID: 1, ProductID: 1, Rating: 5, Body: "excellent"},
		{ID: 2, ProductID: 2, Rating: 2, Body: "damaged"},
		{ID: 3, ProductID: 3, Rating: 4, Body: "good"},
	}
	for _, value := range []any{&brands, &tags, &products, &reviews} {
		if err := db.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Model(&products[0]).Association("Tags").Append(&tags[0]); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&products[1]).Association("Tags").Append(&tags[1]); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&products[2]).Association("Tags").Append(&tags[0]); err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(db)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func request(t *testing.T, handler http.Handler, values url.Values) query.Page[Product] {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/products?"+values.Encode(), nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var page query.Page[Product]
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	return page
}

func productIDs(products []Product) []uint {
	result := make([]uint, len(products))
	for index, product := range products {
		result[index] = product.ID
	}
	return result
}
