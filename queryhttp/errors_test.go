package queryhttp

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	query "github.com/dmedovich/gotq"
)

func TestWriteErrorPreservesSafeClientDetails(t *testing.T) {
	err := &query.Error{
		Code:      query.CodeUnknownField,
		Parameter: "filter",
		Field:     "missing",
		Message:   "unknown public field",
	}
	recorder := httptest.NewRecorder()
	WriteError(recorder, err)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	for _, want := range []string{`"code":"unknown_field"`, `"field":"missing"`} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Fatalf("body %q does not contain %q", recorder.Body.String(), want)
		}
	}
}

func TestWriteErrorSanitizesInternalAndUnknownErrors(t *testing.T) {
	tests := []error{
		&query.Error{Code: query.CodeInvalidSchema, Message: "secret_column is invalid"},
		&query.Error{Code: query.CodeExecutionFailed, Message: "failed", Cause: errors.New("password=secret")},
		errors.New("token=secret"),
		nil,
	}
	for _, err := range tests {
		recorder := httptest.NewRecorder()
		WriteError(recorder, err)
		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("Status(%v) = %d", err, recorder.Code)
		}
		if got, want := strings.TrimSpace(recorder.Body.String()), `{"code":"internal_error","message":"internal server error"}`; got != want {
			t.Fatalf("body = %q, want %q", got, want)
		}
	}
}

func TestStatusTreatsUnknownQueryCodeAsInternal(t *testing.T) {
	if got := Status(&query.Error{Code: query.ErrorCode("future")}); got != http.StatusInternalServerError {
		t.Fatalf("status = %d", got)
	}
	if got := Status(&query.Error{Code: query.CodeInvalidLiteral}); got != http.StatusBadRequest {
		t.Fatalf("status = %d", got)
	}
	if got := Status(&query.Error{Code: query.CodeInvalidRelationship}); got != http.StatusBadRequest {
		t.Fatalf("relationship status = %d", got)
	}
	if got := Status(&query.Error{Code: query.CodeInvalidCursor}); got != http.StatusBadRequest {
		t.Fatalf("cursor status = %d", got)
	}
}

func TestResponseSupportsFrameworkAdapters(t *testing.T) {
	status, payload := Response(&query.Error{Code: query.CodeInvalidSyntax, Message: "bad filter"})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d", status)
	}
	if _, ok := payload.(*query.Error); !ok {
		t.Fatalf("payload type = %T", payload)
	}
	status, payload = Response(errors.New("secret"))
	if status != http.StatusInternalServerError {
		t.Fatalf("status = %d", status)
	}
	if _, ok := payload.(InternalError); !ok {
		t.Fatalf("payload type = %T", payload)
	}
}
