package query_test

import (
	"encoding/json"
	"net/http"
	"time"

	query "github.com/dmedovich/gotq"
	"github.com/dmedovich/gotq/queryhttp"
	"gorm.io/gorm"
)

type User struct {
	ID        uint      `json:"id"`
	Name      string    `json:"name"`
	Age       int       `json:"age"`
	CreatedAt time.Time `json:"createdAt"`
}

func buildUserEngine(db *gorm.DB) (*query.Engine[User], error) {
	policy := query.Schema[User]().
		Expose("id", query.Sortable()).
		Expose("name", query.Filterable(), query.Sortable()).
		Expose("age", query.Filterable()).
		Expose("createdAt", query.Filterable(), query.Sortable())
	return query.New(db, query.Config[User]{
		Policy:       policy,
		DefaultLimit: 25,
		MaxLimit:     100,
		MaxOffset:    100_000,
		AllowCount:   true,
	})
}

func listUsers(engine *query.Engine[User]) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, err := engine.List(r.Context(), r.URL.Query())
		if err != nil {
			queryhttp.WriteError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(page)
	}
}
