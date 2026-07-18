// Package catalogapp demonstrates relationship queries with gotq.
package catalogapp

import (
	"encoding/json"
	"net/http"
	"time"

	query "github.com/dmedovich/gotq"
	"github.com/dmedovich/gotq/queryhttp"
	"gorm.io/gorm"
)

// Brand is a product's belongs-to relationship.
type Brand struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

// Review is a product's has-many relationship.
type Review struct {
	ID        uint   `json:"id"`
	ProductID uint   `json:"productId"`
	Rating    int    `json:"rating"`
	Body      string `json:"body"`
}

// Tag is attached through a many-to-many join table.
type Tag struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

// Product is the application-owned root response model.
type Product struct {
	ID        uint      `json:"id"`
	SKU       string    `json:"sku"`
	Price     int64     `json:"price"`
	CreatedAt time.Time `json:"createdAt"`
	BrandID   uint      `json:"brandId"`
	Brand     Brand     `json:"brand"`
	Reviews   []Review  `json:"reviews,omitempty" gorm:"foreignKey:ProductID"`
	Tags      []Tag     `json:"tags,omitempty" gorm:"many2many:product_tags"`
}

// Handler owns the immutable relationship policy.
type Handler struct {
	engine *query.Engine[Product]
}

// NewHandler validates belongs-to, has-many, and many-to-many metadata at
// startup.
func NewHandler(db *gorm.DB) (*Handler, error) {
	brands := query.Schema[Brand]().
		Expose("name", query.Filterable(query.Eq, query.Contains), query.Sortable())
	reviews := query.Schema[Review]().
		Expose("rating", query.Filterable(query.Eq, query.Gte, query.Lte)).
		Expose("body", query.Filterable(query.Contains))
	tags := query.Schema[Tag]().
		Expose("name", query.Filterable(query.Eq, query.In))
	products := query.Schema[Product]().
		Expose("id", query.Sortable()).
		Expose("sku", query.Filterable(query.Eq, query.In), query.Sortable()).
		Expose("price", query.Filterable(query.Eq, query.Gte, query.Lte), query.Sortable()).
		Expose("createdAt", query.Filterable(query.Gte, query.Lte), query.Sortable()).
		Relation("brand", brands).
		Relation("reviews", reviews).
		Relation("tags", tags)
	engine, err := query.New(db, query.Config[Product]{
		Policy:             products,
		DefaultLimit:       20,
		MaxLimit:           100,
		MaxOffset:          1_000,
		MaxPathDepth:       5,
		MaxQuantifierDepth: 3,
		AllowCount:         true,
	})
	if err != nil {
		return nil, err
	}
	return &Handler{engine: engine}, nil
}

// ServeHTTP executes the relationship query without exposing storage names.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	page, err := h.engine.List(r.Context(), r.URL.Query())
	if err != nil {
		queryhttp.WriteError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(page)
}
