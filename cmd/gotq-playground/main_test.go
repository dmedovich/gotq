package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPlaygroundServesUIAndParsesWithoutDatabase(t *testing.T) {
	handler := playgroundHandler()

	ui := httptest.NewRecorder()
	handler.ServeHTTP(ui, httptest.NewRequest(http.MethodGet, "/", nil))
	if ui.Code != http.StatusOK || !strings.Contains(ui.Body.String(), "gotq parser playground") {
		t.Fatalf("UI response = %d %q", ui.Code, ui.Body.String())
	}

	parsed := httptest.NewRecorder()
	handler.ServeHTTP(parsed, httptest.NewRequest(http.MethodGet, "/api/parse?filter=age+gte+18&sort=-name&limit=5", nil))
	if parsed.Code != http.StatusOK {
		t.Fatalf("parse response = %d %q", parsed.Code, parsed.Body.String())
	}
	var payload struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(parsed.Body.Bytes(), &payload); err != nil || !payload.OK {
		t.Fatalf("parse payload = %q, err = %v", parsed.Body.String(), err)
	}

	rejected := httptest.NewRecorder()
	handler.ServeHTTP(rejected, httptest.NewRequest(http.MethodGet, "/api/parse?filter=age+wat+18", nil))
	if rejected.Code != http.StatusBadRequest || !strings.Contains(rejected.Body.String(), `"code":"invalid_syntax"`) {
		t.Fatalf("invalid response = %d %q", rejected.Code, rejected.Body.String())
	}
}
