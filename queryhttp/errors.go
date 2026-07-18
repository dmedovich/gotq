// Package queryhttp contains optional net/http helpers for gotq endpoints.
package queryhttp

import (
	"encoding/json"
	"errors"
	"net/http"

	query "github.com/dmedovich/gotq"
)

// InternalError is the deliberately small response used when an error is not
// safe to expose to a client.
type InternalError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Status maps public query errors to HTTP 400 and all configuration,
// execution, malformed, or unknown errors to HTTP 500.
func Status(err error) int {
	var queryErr *query.Error
	if !errors.As(err, &queryErr) || queryErr == nil {
		return http.StatusInternalServerError
	}
	switch queryErr.Code {
	case query.CodeInvalidParameter,
		query.CodeInvalidToken,
		query.CodeInvalidSyntax,
		query.CodeLimitExceeded,
		query.CodeUnknownField,
		query.CodeNotFilterable,
		query.CodeNotSortable,
		query.CodeOperatorNotAllowed,
		query.CodeInvalidLiteral,
		query.CodeInvalidRelationship,
		query.CodeInvalidCursor:
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// Response returns the status and safe JSON payload used by WriteError. It is
// useful in adapters such as Gin, Echo, and Fiber that own response encoding.
func Response(err error) (int, any) {
	status := Status(err)
	if status == http.StatusBadRequest {
		var queryErr *query.Error
		if errors.As(err, &queryErr) && queryErr != nil {
			return status, queryErr
		}
	}
	return status, InternalError{Code: "internal_error", Message: "internal server error"}
}

// WriteError writes a JSON response without exposing configuration,
// execution, wrapped, or unknown error details. It does not log the error;
// handlers retain control of observability policy.
func WriteError(w http.ResponseWriter, err error) {
	status, payload := Response(err)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
