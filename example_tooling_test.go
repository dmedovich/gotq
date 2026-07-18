package query_test

import (
	"net/http/httptest"
	"testing"

	query "github.com/dmedovich/gotq"
	"github.com/dmedovich/gotq/openapi"
	"github.com/dmedovich/gotq/queryhttp"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAdoptionToolingDocumentationSnippet(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:tooling-docs?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	config := query.Config[User]{
		Policy: query.Schema[User]().
			Expose("id", query.Sortable()).
			Expose("name", query.Filterable(query.Eq, query.Contains), query.Sortable()),
		DefaultLimit: 25,
		MaxLimit:     100,
		MaxOffset:    100_000,
	}
	if err := query.ValidateConfig(db, config); err != nil {
		t.Fatal(err)
	}
	users, err := query.New(db, config)
	if err != nil {
		t.Fatal(err)
	}
	document, err := openapi.Generate("Users", "1.0.0", "/users", users.Describe())
	if err != nil || document.Paths["/users"].Get.Parameters == nil {
		t.Fatalf("OpenAPI document = %#v, err = %v", document, err)
	}
	recorder := httptest.NewRecorder()
	queryhttp.WriteError(recorder, &query.Error{Code: query.CodeInvalidParameter, Message: "bad parameter"})
	if recorder.Code != 400 {
		t.Fatalf("HTTP status = %d", recorder.Code)
	}
}
