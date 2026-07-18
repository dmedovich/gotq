package frameworkexamples

import (
	"net/http"
	"testing"

	query "github.com/dmedovich/gotq"
)

func TestAdaptersCompile(t *testing.T) {
	var engine *query.Engine[User]
	if handler := NetHTTP(engine); handler == nil {
		t.Fatal("net/http adapter is nil")
	}
	if handler := Gin(engine); handler == nil {
		t.Fatal("Gin adapter is nil")
	}
	if handler := Echo(engine); handler == nil {
		t.Fatal("Echo adapter is nil")
	}
	if handler := Fiber(engine); handler == nil {
		t.Fatal("Fiber adapter is nil")
	}
	_ = http.StatusOK
}
