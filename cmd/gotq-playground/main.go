// Command gotq-playground runs a local, database-free query parser playground.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	query "github.com/dmedovich/gotq"
	"github.com/dmedovich/gotq/queryhttp"
)

func main() {
	address := flag.String("addr", "127.0.0.1:8080", "listen address")
	flag.Parse()
	server := &http.Server{
		Addr:              *address,
		Handler:           playgroundHandler(),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	log.Printf("gotq parser playground: http://%s", *address)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func playgroundHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, playgroundHTML)
	})
	mux.HandleFunc("GET /api/parse", func(w http.ResponseWriter, r *http.Request) {
		parsed, err := query.ParseHTTP(r.URL.Query())
		if err != nil {
			queryhttp.WriteError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(struct {
			OK    bool        `json:"ok"`
			Query query.Query `json:"query"`
		}{OK: true, Query: parsed})
	})
	return mux
}

const playgroundHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>gotq parser playground</title>
  <style>
    :root { color-scheme: light dark; font-family: ui-sans-serif, system-ui, sans-serif; }
    body { max-width: 880px; margin: 3rem auto; padding: 0 1rem; }
    label { display: block; margin: 1rem 0 .35rem; font-weight: 650; }
    input { box-sizing: border-box; width: 100%; padding: .7rem; font: inherit; }
    button { margin-top: 1rem; padding: .65rem 1rem; font: inherit; cursor: pointer; }
    pre { min-height: 12rem; overflow: auto; padding: 1rem; border: 1px solid #8886; border-radius: .5rem; }
    .row { display: grid; grid-template-columns: 2fr 1fr 1fr; gap: 1rem; }
  </style>
</head>
<body>
  <h1>gotq parser playground</h1>
  <p>Parse query syntax locally without a model or database. Schema validation happens later in a real endpoint.</p>
  <form id="query">
    <label for="filter">filter</label>
    <input id="filter" name="filter" value="age gte 18 and name contains 'ann'">
    <label for="sort">sort</label>
    <input id="sort" name="sort" value="-createdAt,name">
    <div class="row">
      <div><label for="search">search</label><input id="search" name="search"></div>
      <div><label for="limit">limit</label><input id="limit" name="limit" value="20"></div>
      <div><label for="offset">offset</label><input id="offset" name="offset" value="0"></div>
    </div>
    <button type="submit">Parse</button>
  </form>
  <h2>Result</h2>
  <pre id="result" aria-live="polite">Submit a query to inspect its syntax tree.</pre>
  <script>
    const form = document.querySelector('#query');
    const result = document.querySelector('#result');
    form.addEventListener('submit', async (event) => {
      event.preventDefault();
      const values = new URLSearchParams();
      for (const [name, value] of new FormData(form)) if (value !== '') values.set(name, value);
      const response = await fetch('/api/parse?' + values);
      result.textContent = JSON.stringify(await response.json(), null, 2);
    });
  </script>
</body>
</html>`
