package tenantapp

import (
	"context"
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

func TestTenantHandlerScopeCountAndCursorReplay(t *testing.T) {
	handler := seededHandler(t)

	first := request(t, handler, 1, url.Values{
		"filter": {"status eq 'paid'"},
		"sort":   {"issuedAt"},
		"limit":  {"1"},
		"count":  {"true"},
	})
	if first.Total == nil || *first.Total != 2 || ids(first.Items)[0] != 1 || first.Page.NextCursor == "" {
		t.Fatalf("first tenant page = %#v", first)
	}

	second := request(t, handler, 1, url.Values{
		"filter": {"status eq 'paid'"},
		"sort":   {"issuedAt"},
		"limit":  {"1"},
		"cursor": {first.Page.NextCursor},
	})
	if got := ids(second.Items); !reflect.DeepEqual(got, []uint{2}) || second.Page.HasMore {
		t.Fatalf("second tenant page IDs = %v, page = %#v", got, second.Page)
	}

	// A cursor is a position, not authority. Replaying tenant 1's token under
	// tenant 2 must retain tenant 2's base scope.
	replayed := request(t, handler, 2, url.Values{
		"filter": {"status eq 'paid'"},
		"sort":   {"issuedAt"},
		"limit":  {"10"},
		"cursor": {first.Page.NextCursor},
	})
	if got := ids(replayed.Items); !reflect.DeepEqual(got, []uint{3}) {
		t.Fatalf("cross-tenant cursor replay IDs = %v", got)
	}
}

func TestTenantHandlerRejectsMissingPrincipalAndInvalidQuery(t *testing.T) {
	handler := seededHandler(t)

	request := httptest.NewRequest(http.MethodGet, "/invoices", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("missing principal status = %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/invoices?filter=tenantId%20eq%202", nil)
	request = request.WithContext(WithTenant(request.Context(), 1))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("private field status = %d, body = %s", response.Code, response.Body.String())
	}
}

func seededHandler(t *testing.T) *Handler {
	t.Helper()
	dsn := fmt.Sprintf("file:tenant-%d?mode=memory&cache=shared", time.Now().UnixNano())
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
	if err := db.AutoMigrate(&Invoice{}); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	invoices := []Invoice{
		{ID: 1, TenantID: 1, Number: "T1-001", Status: "paid", Amount: 100, IssuedAt: base.Add(time.Hour)},
		{ID: 2, TenantID: 1, Number: "T1-002", Status: "paid", Amount: 200, IssuedAt: base.Add(2 * time.Hour)},
		{ID: 4, TenantID: 1, Number: "T1-003", Status: "draft", Amount: 300, IssuedAt: base.Add(3 * time.Hour)},
		{ID: 3, TenantID: 2, Number: "T2-001", Status: "paid", Amount: 999, IssuedAt: base.Add(4 * time.Hour)},
	}
	if err := db.Create(&invoices).Error; err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(db)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func request(t *testing.T, handler http.Handler, tenantID uint, values url.Values) query.Page[Invoice] {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/invoices?"+values.Encode(), nil)
	req = req.WithContext(WithTenant(context.Background(), tenantID))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var page query.Page[Invoice]
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	return page
}

func ids(invoices []Invoice) []uint {
	result := make([]uint, len(invoices))
	for index, invoice := range invoices {
		result[index] = invoice.ID
	}
	return result
}
