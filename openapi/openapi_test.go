package openapi

import (
	"encoding/json"
	"strings"
	"testing"

	query "github.com/dmedovich/gotq"
)

func TestGenerateUsesOnlyPublicPolicy(t *testing.T) {
	description := query.EndpointDescription{
		SyntaxVersion: query.SyntaxVersion,
		Schema: query.SchemaDescription{Fields: []query.FieldDescription{
			{Name: "name", Kind: query.String, Filterable: true, Sortable: true, Operators: []query.ComparisonOperator{query.Eq, query.Contains}},
			{Name: "id", Kind: query.Uint, Sortable: true},
			{Name: "privateLabel", Kind: query.String},
		}},
		Pagination:           query.PaginationDescription{DefaultLimit: 25, MaxLimit: 100, MaxOffset: 1000, Cursor: true},
		Limits:               query.Limits{MaxFilterBytes: 2048, MaxSearchBytes: 64, MaxSortTerms: 3, MaxNodes: 100, MaxCursorBytes: 4096},
		Count:                true,
		Search:               true,
		CompatibilityAliases: true,
	}

	document, err := Generate("Users", "1.0.0", "/users", description)
	if err != nil {
		t.Fatal(err)
	}
	operation := document.Paths["/users"].Get
	if got, want := parameterNames(operation.Parameters), []string{"filter", "sort", "limit", "offset", "cursor", "count", "search", "orderby", "top", "skip"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("parameters = %v, want %v", got, want)
	}
	filter := operation.Parameters[0]
	if filter.XGotQMaxBytes != 2048 || len(filter.XGotQFields) != 1 || filter.XGotQFields[0].Name != "name" {
		t.Fatalf("filter parameter = %#v", filter)
	}
	if got := strings.Join(filter.XGotQFields[0].Operators, ","); got != "eq,contains" {
		t.Fatalf("operators = %q", got)
	}
	if operation.Parameters[1].XGotQMaxTerms != 3 || operation.XGotQLimits.MaxNodes != 100 {
		t.Fatalf("generated limits = sort:%d operation:%#v", operation.Parameters[1].XGotQMaxTerms, operation.XGotQLimits)
	}
	if len(operation.XGotQSchema.Fields) != 3 {
		t.Fatalf("public schema fields = %#v", operation.XGotQSchema.Fields)
	}
	if operation.Parameters[4].XGotQMaxBytes != 4096 || !operation.Parameters[7].Deprecated || !operation.Parameters[8].Deprecated || !operation.Parameters[9].Deprecated {
		t.Fatal("compatibility aliases must be marked deprecated")
	}

	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"go_field", "db_column", "secret_column"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("OpenAPI leaked non-queryable detail %q: %s", private, encoded)
		}
	}
}

func TestNewOperationRejectsInvalidDescriptions(t *testing.T) {
	base := query.EndpointDescription{
		SyntaxVersion: query.SyntaxVersion,
		Pagination:    query.PaginationDescription{DefaultLimit: 1, MaxLimit: 10, MaxOffset: 10},
	}
	tests := []query.EndpointDescription{
		{SyntaxVersion: "v2", Pagination: base.Pagination},
		{SyntaxVersion: query.SyntaxVersion, Pagination: query.PaginationDescription{DefaultLimit: 11, MaxLimit: 10, MaxOffset: 10}},
		{SyntaxVersion: query.SyntaxVersion, Pagination: query.PaginationDescription{DefaultLimit: 1, MaxLimit: 10, MaxOffset: 10, Cursor: true}},
		{SyntaxVersion: query.SyntaxVersion, Pagination: base.Pagination, Schema: query.SchemaDescription{Fields: []query.FieldDescription{{Name: "id"}, {Name: "id"}}}},
		{SyntaxVersion: query.SyntaxVersion, Pagination: base.Pagination, Schema: query.SchemaDescription{Relationships: []query.RelationshipDescription{{Name: "orders", Cardinality: "unknown"}}}},
	}
	for _, description := range tests {
		if _, err := NewOperation(description); err == nil {
			t.Fatalf("accepted invalid description %#v", description)
		}
	}
	if _, err := Generate("", "1", "/users", base); err == nil {
		t.Fatal("accepted empty title")
	}
	if _, err := Generate("Users", "1", "users", base); err == nil {
		t.Fatal("accepted relative path")
	}
}

func TestNewOperationIncludesToOnePathsAndNestedRelationshipSchema(t *testing.T) {
	description := query.EndpointDescription{
		SyntaxVersion: query.SyntaxVersion,
		Pagination:    query.PaginationDescription{DefaultLimit: 10, MaxLimit: 50, MaxOffset: 100, Cursor: true},
		Limits:        query.Limits{MaxFilterBytes: 1000, MaxSortTerms: 4, MaxCursorBytes: 4096},
		Schema: query.SchemaDescription{
			Fields: []query.FieldDescription{{Name: "id", Kind: query.Uint, Sortable: true}},
			Relationships: []query.RelationshipDescription{
				{Name: "company", Cardinality: query.RelationshipOne, Filterable: true, Sortable: true, Schema: query.SchemaDescription{Fields: []query.FieldDescription{{Name: "name", Kind: query.String, Filterable: true, Sortable: true, Operators: []query.ComparisonOperator{query.Eq}}}}},
				{Name: "orders", Cardinality: query.RelationshipMany, Filterable: true, Schema: query.SchemaDescription{Fields: []query.FieldDescription{{Name: "total", Kind: query.Int, Filterable: true, Operators: []query.ComparisonOperator{query.Gt}}}}},
			},
		},
	}
	operation, err := NewOperation(description)
	if err != nil {
		t.Fatal(err)
	}
	if got := fieldPolicyNames(operation.Parameters[0].XGotQFields); strings.Join(got, ",") != "company/name" {
		t.Fatalf("filter paths = %v", got)
	}
	if got := fieldPolicyNames(operation.Parameters[1].XGotQFields); strings.Join(got, ",") != "id,company/name" {
		t.Fatalf("sort paths = %v", got)
	}
	if len(operation.XGotQSchema.Relationships) != 2 {
		t.Fatalf("relationship schema = %#v", operation.XGotQSchema.Relationships)
	}
}

func parameterNames(parameters []Parameter) []string {
	names := make([]string, len(parameters))
	for i, parameter := range parameters {
		names[i] = parameter.Name
	}
	return names
}

func fieldPolicyNames(fields []FieldPolicy) []string {
	names := make([]string, len(fields))
	for index, field := range fields {
		names[index] = field.Name
	}
	return names
}
