// Package tenantapp demonstrates a tenant-scoped gotq endpoint.
package tenantapp

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	query "github.com/dmedovich/gotq"
	"github.com/dmedovich/gotq/queryhttp"
	"gorm.io/gorm"
)

// Invoice is the application-owned response model.
type Invoice struct {
	ID       uint      `json:"id"`
	TenantID uint      `json:"-" gorm:"index"`
	Number   string    `json:"number"`
	Status   string    `json:"status"`
	Amount   int64     `json:"amount"`
	IssuedAt time.Time `json:"issuedAt"`
}

type tenantContextKey struct{}

// WithTenant represents the authentication middleware boundary. Real
// applications should call it only after authenticating and authorizing the
// principal, never from an unvalidated query parameter.
func WithTenant(ctx context.Context, tenantID uint) context.Context {
	return context.WithValue(ctx, tenantContextKey{}, tenantID)
}

// Handler owns one immutable engine and applies a request-local tenant scope.
type Handler struct {
	db     *gorm.DB
	engine *query.Engine[Invoice]
}

// NewHandler validates the endpoint policy during application startup.
func NewHandler(db *gorm.DB) (*Handler, error) {
	policy := query.Schema[Invoice]().
		Expose("id", query.Sortable()).
		Expose("number", query.Filterable(query.Eq, query.Contains), query.Sortable()).
		Expose("status", query.Filterable(query.Eq, query.In)).
		Expose("amount", query.Filterable(query.Eq, query.Gt, query.Gte, query.Lt, query.Lte), query.Sortable()).
		Expose("issuedAt", query.Filterable(query.Gt, query.Gte, query.Lt, query.Lte), query.Sortable())
	engine, err := query.New(db, query.Config[Invoice]{
		Policy:       policy,
		DefaultLimit: 20,
		MaxLimit:     100,
		MaxOffset:    1_000,
		AllowCount:   true,
	})
	if err != nil {
		return nil, err
	}
	return &Handler{db: db, engine: engine}, nil
}

// ServeHTTP lists invoices within the authenticated tenant boundary.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := r.Context().Value(tenantContextKey{}).(uint)
	if !ok || tenantID == 0 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"code":    "unauthorized",
			"message": "authentication required",
		})
		return
	}

	base := h.db.Where("tenant_id = ?", tenantID)
	page, err := h.engine.From(base).List(r.Context(), r.URL.Query())
	if err != nil {
		queryhttp.WriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
