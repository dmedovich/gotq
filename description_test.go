package query

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestEngineDescribeIsDeterministicDetachedAndStorageIndependent(t *testing.T) {
	db := newEngineTestDB(t)
	engine := newUserEngine(t, db)

	description := engine.Describe()
	if description.SyntaxVersion != "v1" {
		t.Fatalf("SyntaxVersion = %q", description.SyntaxVersion)
	}
	if got, want := description.Pagination, (PaginationDescription{DefaultLimit: 2, MaxLimit: 3, MaxOffset: 10, Cursor: true}); got != want {
		t.Fatalf("Pagination = %#v, want %#v", got, want)
	}
	if !description.Count || !description.Search || description.CompatibilityAliases {
		t.Fatalf("capabilities = count:%v search:%v aliases:%v", description.Count, description.Search, description.CompatibilityAliases)
	}
	if description.Limits.MaxExpressionDepth != defaultQueryLimits.MaxExpressionDepth || description.Limits.MaxNodes != defaultQueryLimits.MaxNodes {
		t.Fatalf("AST limits = depth:%d nodes:%d", description.Limits.MaxExpressionDepth, description.Limits.MaxNodes)
	}
	if description.Limits.MaxPathDepth != defaultQueryLimits.MaxPathDepth || description.Limits.MaxQuantifierDepth != defaultQueryLimits.MaxQuantifierDepth {
		t.Fatalf("relationship limits = path:%d quantifier:%d", description.Limits.MaxPathDepth, description.Limits.MaxQuantifierDepth)
	}
	if description.Limits.MaxCursorBytes != defaultQueryLimits.MaxCursorBytes {
		t.Fatalf("cursor limit = %d", description.Limits.MaxCursorBytes)
	}
	if got, want := fieldNames(description.Schema.Fields), []string{"age", "id", "name", "tenantId"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("field names = %v, want %v", got, want)
	}
	if description.Schema.Relationships == nil {
		t.Fatal("relationships must encode as an empty array, not null")
	}

	encoded, err := json.Marshal(description)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"TenantID", "tenant_id", "engineTestUser"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("description leaked storage detail %q: %s", private, encoded)
		}
	}

	description.Schema.Fields[0].Name = "changed"
	description.Schema.Fields[0].Operators[0] = IsNull
	again := engine.Describe()
	if again.Schema.Fields[0].Name != "age" || again.Schema.Fields[0].Operators[0] != Eq {
		t.Fatalf("description mutation changed engine policy: %#v", again.Schema.Fields[0])
	}
}

func TestNilDescribeAndValidateConfig(t *testing.T) {
	var schema *ModelSchema[engineTestUser]
	if description := schema.Describe(); description.Fields == nil || description.Relationships == nil {
		t.Fatalf("nil schema description = %#v", description)
	}
	var engine *Engine[engineTestUser]
	if description := engine.Describe(); description.SyntaxVersion != SyntaxVersion || description.Schema.Fields == nil {
		t.Fatalf("nil engine description = %#v", description)
	}

	db := newEngineTestDB(t)
	valid := Config[engineTestUser]{
		Policy:       Schema[engineTestUser]().Expose("id", Sortable()),
		DefaultLimit: 1,
		MaxLimit:     10,
		MaxOffset:    100,
	}
	if err := ValidateConfig(db, valid); err != nil {
		t.Fatalf("ValidateConfig(valid) = %v", err)
	}
	valid.MaxLimit = 0
	if err := ValidateConfig(db, valid); err == nil {
		t.Fatal("ValidateConfig accepted an invalid limit")
	}
}

func fieldNames(fields []FieldDescription) []string {
	names := make([]string, len(fields))
	for i, field := range fields {
		names[i] = field.Name
	}
	return names
}
