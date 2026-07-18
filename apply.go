package query

import "gorm.io/gorm"

// Apply validates q against schema and returns derived GORM scopes. It does
// not execute a database statement. On failure it returns nil and an error.
func Apply[T any](db *gorm.DB, schema *ModelSchema[T], q Query) (*gorm.DB, error) {
	if q.Search != nil {
		return nil, queryValidationError(CodeInvalidParameter, "search", *q.Search, 0, "search requires an engine callback", "")
	}
	if q.Cursor != nil {
		return nil, queryValidationError(CodeInvalidCursor, "cursor", *q.Cursor, 0, "cursor pagination requires an engine", "")
	}
	validated, err := validateQuery(schema, q)
	if err != nil {
		return nil, err
	}
	compiled, err := compileGORM(db, validated)
	if err != nil {
		return nil, err
	}
	return compiled, nil
}
