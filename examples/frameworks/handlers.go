// Package frameworkexamples contains compile-tested gotq adapters. It is a
// separate module so applications using gotq do not inherit web frameworks.
package frameworkexamples

import (
	"net/http"
	"net/url"
	"time"

	query "github.com/dmedovich/gotq"
	"github.com/dmedovich/gotq/queryhttp"
	"github.com/gin-gonic/gin"
	"github.com/gofiber/fiber/v2"
	"github.com/labstack/echo/v4"
)

// User is the response model used by the examples.
type User struct {
	ID        uint      `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

// NetHTTP returns a standard-library list handler.
func NetHTTP(engine *query.Engine[User]) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, err := engine.List(r.Context(), r.URL.Query())
		if err != nil {
			queryhttp.WriteError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = writeJSON(w, page)
	}
}

// Gin returns a Gin list handler.
func Gin(engine *query.Engine[User]) gin.HandlerFunc {
	return func(c *gin.Context) {
		page, err := engine.List(c.Request.Context(), c.Request.URL.Query())
		if err != nil {
			status, payload := queryhttp.Response(err)
			c.JSON(status, payload)
			return
		}
		c.JSON(http.StatusOK, page)
	}
}

// Echo returns an Echo list handler.
func Echo(engine *query.Engine[User]) echo.HandlerFunc {
	return func(c echo.Context) error {
		request := c.Request()
		page, err := engine.List(request.Context(), request.URL.Query())
		if err != nil {
			status, payload := queryhttp.Response(err)
			return c.JSON(status, payload)
		}
		return c.JSON(http.StatusOK, page)
	}
}

// Fiber returns a Fiber list handler.
func Fiber(engine *query.Engine[User]) fiber.Handler {
	return func(c *fiber.Ctx) error {
		values := make(url.Values)
		c.Context().QueryArgs().VisitAll(func(key, value []byte) {
			values.Add(string(key), string(value))
		})
		page, err := engine.List(c.UserContext(), values)
		if err != nil {
			status, payload := queryhttp.Response(err)
			return c.Status(status).JSON(payload)
		}
		return c.Status(http.StatusOK).JSON(page)
	}
}
