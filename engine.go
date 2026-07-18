package query

import (
	"context"
	"fmt"
	"net/url"

	"gorm.io/gorm"
)

// SearchFunc applies an endpoint-owned search predicate. It is trusted
// application code and must bind all values derived from term.
type SearchFunc func(context.Context, *gorm.DB, string) (*gorm.DB, error)

// Config defines an immutable query engine for one model and endpoint policy.
type Config[T any] struct {
	Policy *SchemaBuilder[T]

	DefaultLimit int
	MaxLimit     int
	MaxOffset    int

	MaxSortTerms       int
	MaxSearchBytes     int
	MaxQueryBytes      int
	MaxFilterBytes     int
	MaxTokens          int
	MaxLiteralBytes    int
	MaxInValues        int
	MaxExpressionDepth int
	MaxNodes           int
	MaxPathDepth       int
	MaxQuantifierDepth int
	MaxCursorBytes     int

	AllowCount                bool
	AllowCompatibilityAliases bool
	Search                    SearchFunc
}

// Engine parses, validates, applies, and optionally executes list queries for
// T. It is immutable after New returns and safe for concurrent use.
type Engine[T any] struct {
	db     *gorm.DB
	schema *ModelSchema[T]
	config Config[T]
	limits Limits
}

// Session is a request-local engine view using a caller-owned base scope.
type Session[T any] struct {
	engine *Engine[T]
	base   *gorm.DB
}

// Page is the result of a list operation.
type Page[T any] struct {
	Items []T      `json:"items"`
	Page  PageInfo `json:"page"`
	Total *int64   `json:"total,omitempty"`
}

// PageInfo describes the effective page and optional forward continuation.
type PageInfo struct {
	Limit      int    `json:"limit"`
	Offset     int    `json:"offset"`
	HasMore    bool   `json:"hasMore"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// New constructs and validates an immutable engine using the active GORM
// naming strategy and model metadata.
func New[T any](db *gorm.DB, config Config[T]) (*Engine[T], error) {
	if db == nil {
		return nil, schemaError("gorm DB is nil", "")
	}
	if config.Policy == nil {
		return nil, schemaError("engine policy is nil", "")
	}
	if config.DefaultLimit <= 0 {
		return nil, schemaError("default limit must be positive", "")
	}
	if config.MaxLimit <= 0 {
		return nil, schemaError("maximum limit must be positive", "")
	}
	if config.MaxLimit == maxInt() {
		return nil, schemaError("maximum limit must leave room for pagination lookahead", "")
	}
	if config.DefaultLimit > config.MaxLimit {
		return nil, schemaError("default limit must not exceed maximum limit", "")
	}
	if config.MaxOffset <= 0 {
		return nil, schemaError("maximum offset must be positive", "")
	}
	if config.MaxSortTerms == 0 {
		config.MaxSortTerms = defaultQueryLimits.MaxSortTerms
	}
	if config.MaxSearchBytes == 0 {
		config.MaxSearchBytes = defaultQueryLimits.MaxSearchBytes
	}
	if config.MaxQueryBytes == 0 {
		config.MaxQueryBytes = defaultQueryLimits.MaxQueryBytes
	}
	if config.MaxFilterBytes == 0 {
		config.MaxFilterBytes = defaultQueryLimits.MaxFilterBytes
	}
	if config.MaxTokens == 0 {
		config.MaxTokens = defaultQueryLimits.MaxTokens
	}
	if config.MaxLiteralBytes == 0 {
		config.MaxLiteralBytes = defaultQueryLimits.MaxLiteralBytes
	}
	if config.MaxInValues == 0 {
		config.MaxInValues = defaultQueryLimits.MaxInValues
	}
	if config.MaxExpressionDepth == 0 {
		config.MaxExpressionDepth = defaultQueryLimits.MaxExpressionDepth
	}
	if config.MaxNodes == 0 {
		config.MaxNodes = defaultQueryLimits.MaxNodes
	}
	if config.MaxPathDepth == 0 {
		config.MaxPathDepth = defaultQueryLimits.MaxPathDepth
	}
	if config.MaxQuantifierDepth == 0 {
		config.MaxQuantifierDepth = defaultQueryLimits.MaxQuantifierDepth
	}
	if config.MaxCursorBytes == 0 {
		config.MaxCursorBytes = defaultQueryLimits.MaxCursorBytes
	}
	if config.MaxSortTerms < 0 || config.MaxSearchBytes < 0 || config.MaxQueryBytes < 0 || config.MaxFilterBytes < 0 || config.MaxTokens < 0 || config.MaxLiteralBytes < 0 || config.MaxInValues < 0 || config.MaxExpressionDepth < 0 || config.MaxNodes < 0 || config.MaxPathDepth < 0 || config.MaxQuantifierDepth < 0 || config.MaxCursorBytes < 0 {
		return nil, schemaError("query resource limits must be positive", "")
	}
	schema, err := config.Policy.Bind(db)
	if err != nil {
		return nil, err
	}
	if len(schema.primaryColumns) == 0 {
		return nil, schemaError("model must have at least one primary key", "")
	}
	limits := defaultQueryLimits
	limits.MaxLimit = config.MaxLimit
	limits.MaxOffset = config.MaxOffset
	limits.MaxSortTerms = config.MaxSortTerms
	limits.MaxSearchBytes = config.MaxSearchBytes
	limits.MaxQueryBytes = config.MaxQueryBytes
	limits.MaxFilterBytes = config.MaxFilterBytes
	limits.MaxTokens = config.MaxTokens
	limits.MaxLiteralBytes = config.MaxLiteralBytes
	limits.MaxInValues = config.MaxInValues
	limits.MaxExpressionDepth = config.MaxExpressionDepth
	limits.MaxNodes = config.MaxNodes
	limits.MaxPathDepth = config.MaxPathDepth
	limits.MaxQuantifierDepth = config.MaxQuantifierDepth
	limits.MaxCursorBytes = config.MaxCursorBytes
	return &Engine[T]{db: db, schema: schema, config: config, limits: limits}, nil
}

// From returns a session that preserves base for data and count queries.
func (e *Engine[T]) From(base *gorm.DB) Session[T] {
	return Session[T]{engine: e, base: base}
}

// Parse decodes an HTTP query using the engine's limits and compatibility
// policy. It does not access the database.
func (e *Engine[T]) Parse(values url.Values) (Query, error) {
	if e == nil {
		return Query{}, schemaError("engine is nil", "")
	}
	options := []ParseOption{WithLimits(e.limits)}
	if e.config.AllowCompatibilityAliases {
		options = append(options, WithCompatibilityAliases())
	}
	return ParseHTTP(values, options...)
}

// Apply validates q and returns a derived GORM scope without executing it.
func (e *Engine[T]) Apply(ctx context.Context, base *gorm.DB, q Query) (*gorm.DB, error) {
	if e == nil {
		return nil, schemaError("engine is nil", "")
	}
	validated, err := e.validate(q)
	if err != nil {
		return nil, err
	}
	return e.applyValidated(ctx, base, validated, true, true)
}

// Apply validates q and returns a derived session scope without executing it.
func (s Session[T]) Apply(ctx context.Context, q Query) (*gorm.DB, error) {
	if s.engine == nil {
		return nil, schemaError("engine session is nil", "")
	}
	return s.engine.Apply(ctx, s.base, q)
}

// List parses and executes a page using the engine's default database.
func (e *Engine[T]) List(ctx context.Context, values url.Values) (Page[T], error) {
	if e == nil {
		return Page[T]{}, schemaError("engine is nil", "")
	}
	return e.list(ctx, e.db, values)
}

// List parses and executes a page using the session base scope.
func (s Session[T]) List(ctx context.Context, values url.Values) (Page[T], error) {
	if s.engine == nil {
		return Page[T]{}, schemaError("engine session is nil", "")
	}
	return s.engine.list(ctx, s.base, values)
}

func (e *Engine[T]) list(ctx context.Context, base *gorm.DB, values url.Values) (Page[T], error) {
	q, err := e.Parse(values)
	if err != nil {
		return Page[T]{}, err
	}
	validated, err := e.validate(q)
	if err != nil {
		return Page[T]{}, err
	}
	limit := e.config.DefaultLimit
	if validated.limit != nil {
		limit = *validated.limit
	}
	offset := 0
	if validated.offset != nil {
		offset = *validated.offset
	}
	page := Page[T]{
		Items: make([]T, 0),
		Page:  PageInfo{Limit: limit, Offset: offset},
	}
	if validated.count != nil && *validated.count {
		countScope, scopeErr := e.applyValidated(ctx, base, validated, false, false)
		if scopeErr != nil {
			return Page[T]{}, scopeErr
		}
		var total int64
		if dbErr := countScope.Count(&total).Error; dbErr != nil {
			return Page[T]{}, executionError("failed to count query results", dbErr)
		}
		page.Total = &total
	}
	if limit == 0 {
		return page, nil
	}
	dataPlan := *validated
	fetchLimit := limit + 1
	dataPlan.limit = &fetchLimit
	dataPlan.offset = &offset
	dataScope, scopeErr := e.applyValidated(ctx, base, &dataPlan, true, true)
	if scopeErr != nil {
		return Page[T]{}, scopeErr
	}
	orders, orderErr := effectiveSort(e.schema.modelSchema, validated.sort)
	if orderErr != nil {
		return Page[T]{}, orderErr
	}
	items, cursorKeys, dbErr := findCursorRows[T](dataScope, e.schema.modelSchema, orders)
	if dbErr != nil {
		return Page[T]{}, executionError("failed to load query results", dbErr)
	}
	page.Items = items
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		page.Page.HasMore = true
		next, cursorErr := encodeCursor(e.schema.modelSchema, orders, cursorKeys[limit-1], e.limits.MaxCursorBytes)
		if cursorErr != nil {
			return Page[T]{}, executionError("failed to encode next cursor", cursorErr)
		}
		page.Page.NextCursor = next
	}
	return page, nil
}

func (e *Engine[T]) validate(q Query) (*validatedQuery, error) {
	validated, err := validateQuery(e.schema, q)
	if err != nil {
		return nil, err
	}
	if validated.count != nil && *validated.count && !e.config.AllowCount {
		return nil, queryValidationError(CodeInvalidParameter, "count", "true", 0, "count is not enabled for this endpoint", "")
	}
	if validated.search != nil && e.config.Search == nil {
		return nil, queryValidationError(CodeInvalidParameter, "search", *validated.search, 0, "search is not enabled for this endpoint", "")
	}
	if validated.rawCursor != nil {
		orders, orderErr := effectiveSort(e.schema.modelSchema, validated.sort)
		if orderErr != nil {
			return nil, orderErr
		}
		cursor, cursorErr := decodeCursor(*validated.rawCursor, e.limits.MaxCursorBytes, e.schema.modelSchema, orders)
		if cursorErr != nil {
			return nil, cursorErr
		}
		validated.cursor = cursor
	}
	return validated, nil
}

func (e *Engine[T]) applyValidated(ctx context.Context, base *gorm.DB, validated *validatedQuery, includeSort, includePage bool) (*gorm.DB, error) {
	if base == nil {
		return nil, schemaError("gorm DB is nil", "")
	}
	scoped := base.WithContext(ctx).Model(new(T))
	if validated.search != nil {
		var err error
		scoped, err = e.config.Search(ctx, scoped, *validated.search)
		if err != nil {
			return nil, executionError("search callback failed", err)
		}
		if scoped == nil {
			return nil, executionError("search callback returned a nil GORM scope", nil)
		}
	}
	plan := *validated
	if includeSort {
		orders, orderErr := effectiveSort(e.schema.modelSchema, plan.sort)
		if orderErr != nil {
			return nil, orderErr
		}
		plan.sort = orders
	} else {
		plan.sort = nil
	}
	if !includePage {
		plan.limit = nil
		plan.offset = nil
		plan.cursor = nil
	}
	compiled, err := compileGORM(scoped, &plan)
	if err != nil {
		return nil, err
	}
	return compiled, nil
}

func executionError(message string, cause error) *Error {
	return &Error{Code: CodeExecutionFailed, Message: fmt.Sprintf("gotq: %s", message), Cause: cause}
}
