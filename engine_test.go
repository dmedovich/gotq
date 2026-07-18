package query

import (
	"context"
	"errors"
	"net/url"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type engineTestUser struct {
	ID       uint   `json:"id"`
	TenantID uint   `json:"tenantId"`
	Name     string `json:"name"`
	Age      int    `json:"age"`
}

func newEngineTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&engineTestUser{}); err != nil {
		t.Fatal(err)
	}
	users := []engineTestUser{
		{ID: 1, TenantID: 1, Name: "Alice", Age: 30},
		{ID: 2, TenantID: 1, Name: "Aaron", Age: 30},
		{ID: 3, TenantID: 1, Name: "Bob", Age: 20},
		{ID: 4, TenantID: 2, Name: "Mallory", Age: 40},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func newUserEngine(t *testing.T, db *gorm.DB) *Engine[engineTestUser] {
	t.Helper()
	policy := Schema[engineTestUser]().
		Expose("id", Sortable()).
		Expose("tenantId", Filterable(Eq)).
		Expose("name", Filterable(Eq, Contains), Sortable()).
		Expose("age", Filterable(Eq, Gte, Lte), Sortable())
	engine, err := New(db, Config[engineTestUser]{
		Policy:         policy,
		DefaultLimit:   2,
		MaxLimit:       3,
		MaxOffset:      10,
		AllowCount:     true,
		MaxSearchBytes: 20,
		Search: func(ctx context.Context, db *gorm.DB, term string) (*gorm.DB, error) {
			return db.WithContext(ctx).Where("name LIKE ?", "%"+term+"%"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return engine
}

func TestEngineListUsesDefaultsStableSortAndCount(t *testing.T) {
	db := newEngineTestDB(t)
	engine := newUserEngine(t, db)

	page, err := engine.List(context.Background(), url.Values{
		"filter": {"age gte 20"},
		"sort":   {"-age"},
		"count":  {"true"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.Page.Limit != 2 || page.Page.Offset != 0 || !page.Page.HasMore || page.Page.NextCursor == "" {
		t.Fatalf("PageInfo = %#v", page.Page)
	}
	if page.Total == nil || *page.Total != 4 {
		t.Fatalf("Total = %v, want 4", page.Total)
	}
	if got, want := []uint{page.Items[0].ID, page.Items[1].ID}, []uint{4, 1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("IDs = %v, want stable order %v", got, want)
	}
}

func TestEngineFromPreservesTenantScopeForDataAndCount(t *testing.T) {
	db := newEngineTestDB(t)
	engine := newUserEngine(t, db)

	page, err := engine.From(db.Where("tenant_id = ?", 1)).List(context.Background(), url.Values{
		"limit": {"3"},
		"count": {"true"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total == nil || *page.Total != 3 || len(page.Items) != 3 {
		t.Fatalf("page = %#v, want three tenant rows", page)
	}
	for _, user := range page.Items {
		if user.TenantID != 1 {
			t.Fatalf("tenant scope leaked row %#v", user)
		}
	}
}

func TestEngineSearchAndLimitZero(t *testing.T) {
	db := newEngineTestDB(t)
	engine := newUserEngine(t, db)

	page, err := engine.List(context.Background(), url.Values{
		"search": {"A"},
		"limit":  {"0"},
		"count":  {"true"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 0 || page.Total == nil || *page.Total != 3 {
		t.Fatalf("page = %#v, want empty items and search count 3", page)
	}
}

func TestEngineRejectsDisabledFeaturesBeforeExecution(t *testing.T) {
	db := newEngineTestDB(t)
	policy := Schema[engineTestUser]().Expose("id", Sortable())
	engine, err := New(db, Config[engineTestUser]{Policy: policy, DefaultLimit: 1, MaxLimit: 2, MaxOffset: 10})
	if err != nil {
		t.Fatal(err)
	}
	var queries atomic.Int64
	if err := db.Callback().Query().Before("gorm:query").Register("gotq:count_queries", func(*gorm.DB) {
		queries.Add(1)
	}); err != nil {
		t.Fatal(err)
	}
	for _, values := range []url.Values{
		{"count": {"true"}},
		{"search": {"alice"}},
		{"sort": {"missing"}},
		{"cursor": {"eyJ2IjoxfQ"}},
		{"cursor": {"abc"}, "offset": {"0"}},
	} {
		if _, listErr := engine.List(context.Background(), values); listErr == nil {
			t.Fatalf("List(%v) returned nil error", values)
		}
	}
	if got := queries.Load(); got != 0 {
		t.Fatalf("invalid requests executed %d queries", got)
	}
}

func TestEngineApplyAndSessionApplyDoNotExecute(t *testing.T) {
	db := newEngineTestDB(t)
	engine := newUserEngine(t, db)
	q, err := engine.Parse(url.Values{
		"filter": {"age gte 30"},
		"sort":   {"-age"},
		"limit":  {"2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var queries atomic.Int64
	if err := db.Callback().Query().Before("gorm:query").Register("gotq:apply_queries", func(*gorm.DB) {
		queries.Add(1)
	}); err != nil {
		t.Fatal(err)
	}
	base := db.Where("tenant_id = ?", 1)
	scoped, err := engine.Apply(context.Background(), base, q)
	if err != nil {
		t.Fatal(err)
	}
	if got := queries.Load(); got != 0 {
		t.Fatalf("Engine.Apply executed %d queries", got)
	}
	var users []engineTestUser
	if err := scoped.Find(&users).Error; err != nil {
		t.Fatal(err)
	}
	if got, want := len(users), 2; got != want {
		t.Fatalf("applied item count = %d, want %d", got, want)
	}
	if _, err := engine.From(base).Apply(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Apply(context.Background(), nil, q); err == nil {
		t.Fatal("Engine.Apply(nil base) returned nil error")
	}
}

func TestEngineWrapsSearchFailures(t *testing.T) {
	db := newEngineTestDB(t)
	sentinel := errors.New("search failed")
	policy := Schema[engineTestUser]().Expose("id", Sortable())
	for _, search := range []SearchFunc{
		func(context.Context, *gorm.DB, string) (*gorm.DB, error) { return nil, sentinel },
		func(context.Context, *gorm.DB, string) (*gorm.DB, error) { return nil, nil },
	} {
		engine, err := New(db, Config[engineTestUser]{
			Policy: policy, DefaultLimit: 1, MaxLimit: 2, MaxOffset: 10, Search: search,
		})
		if err != nil {
			t.Fatal(err)
		}
		_, listErr := engine.List(context.Background(), url.Values{"search": {"x"}})
		var queryErr *Error
		if !errors.As(listErr, &queryErr) || queryErr.Code != CodeExecutionFailed {
			t.Fatalf("List() error = %#v", listErr)
		}
		if queryErr.Cause != nil && !errors.Is(listErr, sentinel) {
			t.Fatalf("wrapped error does not expose cause: %v", listErr)
		}
	}
}

func TestEnginePropagatesContextCancellation(t *testing.T) {
	db := newEngineTestDB(t)
	engine := newUserEngine(t, db)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := engine.List(ctx, url.Values{"limit": {"1"}})
	var queryErr *Error
	if !errors.As(err, &queryErr) || queryErr.Code != CodeExecutionFailed || !errors.Is(err, context.Canceled) {
		t.Fatalf("List() error = %#v, want wrapped context cancellation", err)
	}
}

func TestEngineConcurrentReuse(t *testing.T) {
	db := newEngineTestDB(t)
	engine := newUserEngine(t, db)
	const workers = 12
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			page, err := engine.From(db.Where("tenant_id = ?", 1)).List(context.Background(), url.Values{"limit": {"1"}})
			if err == nil && len(page.Items) != 1 {
				err = &testEngineError{"unexpected item count"}
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

type testEngineError struct{ message string }

func (e *testEngineError) Error() string { return e.message }

func TestNewValidatesConfigurationAndInfersSchema(t *testing.T) {
	db := newEngineTestDB(t)
	if _, err := New[engineTestUser](nil, Config[engineTestUser]{}); err == nil {
		t.Fatal("New(nil DB) returned nil error")
	}
	tests := []struct {
		name   string
		config Config[engineTestUser]
	}{
		{name: "nil policy", config: Config[engineTestUser]{DefaultLimit: 1, MaxLimit: 2, MaxOffset: 3}},
		{name: "zero default", config: Config[engineTestUser]{Policy: Schema[engineTestUser](), MaxLimit: 2, MaxOffset: 3}},
		{name: "default over max", config: Config[engineTestUser]{Policy: Schema[engineTestUser](), DefaultLimit: 3, MaxLimit: 2, MaxOffset: 3}},
		{name: "maximum limit overflows lookahead", config: Config[engineTestUser]{Policy: Schema[engineTestUser](), DefaultLimit: 1, MaxLimit: maxInt(), MaxOffset: 3}},
		{name: "negative expression depth", config: Config[engineTestUser]{Policy: Schema[engineTestUser](), DefaultLimit: 1, MaxLimit: 2, MaxOffset: 3, MaxExpressionDepth: -1}},
		{name: "negative nodes", config: Config[engineTestUser]{Policy: Schema[engineTestUser](), DefaultLimit: 1, MaxLimit: 2, MaxOffset: 3, MaxNodes: -1}},
		{name: "negative path depth", config: Config[engineTestUser]{Policy: Schema[engineTestUser](), DefaultLimit: 1, MaxLimit: 2, MaxOffset: 3, MaxPathDepth: -1}},
		{name: "negative quantifier depth", config: Config[engineTestUser]{Policy: Schema[engineTestUser](), DefaultLimit: 1, MaxLimit: 2, MaxOffset: 3, MaxQuantifierDepth: -1}},
		{name: "negative cursor bytes", config: Config[engineTestUser]{Policy: Schema[engineTestUser](), DefaultLimit: 1, MaxLimit: 2, MaxOffset: 3, MaxCursorBytes: -1}},
		{name: "missing field", config: Config[engineTestUser]{Policy: Schema[engineTestUser]().Expose("missing"), DefaultLimit: 1, MaxLimit: 2, MaxOffset: 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := New(db, tt.config); err == nil {
				t.Fatal("New() returned nil error")
			}
		})
	}
}
