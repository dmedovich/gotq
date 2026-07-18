// Package openapi generates an OpenAPI 3.1 operation from a gotq endpoint
// description. It has no dependency on a particular HTTP framework.
package openapi

import (
	"fmt"
	"sort"
	"strings"

	query "github.com/dmedovich/gotq"
)

// Document is a minimal OpenAPI 3.1 document for one list endpoint.
type Document struct {
	OpenAPI string              `json:"openapi" yaml:"openapi"`
	Info    Info                `json:"info" yaml:"info"`
	Paths   map[string]PathItem `json:"paths" yaml:"paths"`
}

// Info identifies the API represented by a generated document.
type Info struct {
	Title   string `json:"title" yaml:"title"`
	Version string `json:"version" yaml:"version"`
}

// PathItem contains the generated GET operation.
type PathItem struct {
	Get Operation `json:"get" yaml:"get"`
}

// Operation describes gotq parameters, responses, and public policy metadata.
type Operation struct {
	Summary            string                      `json:"summary,omitempty" yaml:"summary,omitempty"`
	Parameters         []Parameter                 `json:"parameters" yaml:"parameters"`
	Responses          map[string]Response         `json:"responses" yaml:"responses"`
	XGotQSyntaxVersion string                      `json:"x-gotq-syntax-version" yaml:"x-gotq-syntax-version"`
	XGotQSchema        query.SchemaDescription     `json:"x-gotq-schema" yaml:"x-gotq-schema"`
	XGotQLimits        query.Limits                `json:"x-gotq-limits" yaml:"x-gotq-limits"`
	XGotQPagination    query.PaginationDescription `json:"x-gotq-pagination" yaml:"x-gotq-pagination"`
}

// Response is a generated response entry.
type Response struct {
	Description string               `json:"description" yaml:"description"`
	Content     map[string]MediaType `json:"content,omitempty" yaml:"content,omitempty"`
}

// MediaType associates a response media type with its schema.
type MediaType struct {
	Schema ValueSchema `json:"schema" yaml:"schema"`
}

// Parameter is an OpenAPI query parameter with gotq vendor extensions.
type Parameter struct {
	Name          string        `json:"name" yaml:"name"`
	In            string        `json:"in" yaml:"in"`
	Description   string        `json:"description,omitempty" yaml:"description,omitempty"`
	Required      bool          `json:"required" yaml:"required"`
	Deprecated    bool          `json:"deprecated,omitempty" yaml:"deprecated,omitempty"`
	Schema        ValueSchema   `json:"schema" yaml:"schema"`
	XGotQFields   []FieldPolicy `json:"x-gotq-fields,omitempty" yaml:"x-gotq-fields,omitempty"`
	XGotQMaxBytes int           `json:"x-gotq-max-bytes,omitempty" yaml:"x-gotq-max-bytes,omitempty"`
	XGotQMaxTerms int           `json:"x-gotq-max-terms,omitempty" yaml:"x-gotq-max-terms,omitempty"`
}

// FieldPolicy describes the public subset usable by a generated parameter.
type FieldPolicy struct {
	Name      string   `json:"name" yaml:"name"`
	Kind      string   `json:"kind" yaml:"kind"`
	Nullable  bool     `json:"nullable,omitempty" yaml:"nullable,omitempty"`
	Operators []string `json:"operators,omitempty" yaml:"operators,omitempty"`
}

// ValueSchema is the subset of JSON Schema needed by generated parameters and
// generic responses.
type ValueSchema struct {
	Type        string `json:"type,omitempty" yaml:"type,omitempty"`
	Format      string `json:"format,omitempty" yaml:"format,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Default     any    `json:"default,omitempty" yaml:"default,omitempty"`
	Minimum     *int   `json:"minimum,omitempty" yaml:"minimum,omitempty"`
	Maximum     *int   `json:"maximum,omitempty" yaml:"maximum,omitempty"`
}

// Generate builds a complete OpenAPI document for one GET list endpoint.
func Generate(title, version, path string, description query.EndpointDescription) (Document, error) {
	if strings.TrimSpace(title) == "" {
		return Document{}, fmt.Errorf("openapi: title must not be empty")
	}
	if strings.TrimSpace(version) == "" {
		return Document{}, fmt.Errorf("openapi: version must not be empty")
	}
	if !strings.HasPrefix(path, "/") {
		return Document{}, fmt.Errorf("openapi: path must start with '/'")
	}
	operation, err := NewOperation(description)
	if err != nil {
		return Document{}, err
	}
	operation.Summary = "List " + title
	return Document{
		OpenAPI: "3.1.0",
		Info:    Info{Title: title, Version: version},
		Paths:   map[string]PathItem{path: {Get: operation}},
	}, nil
}

// NewOperation builds a GET operation from a public endpoint description.
func NewOperation(description query.EndpointDescription) (Operation, error) {
	if description.SyntaxVersion != query.SyntaxVersion {
		return Operation{}, fmt.Errorf("openapi: unsupported gotq syntax version %q", description.SyntaxVersion)
	}
	if description.Pagination.DefaultLimit <= 0 || description.Pagination.MaxLimit <= 0 || description.Pagination.DefaultLimit > description.Pagination.MaxLimit || description.Pagination.MaxOffset <= 0 {
		return Operation{}, fmt.Errorf("openapi: invalid pagination description")
	}
	if description.Pagination.Cursor && description.Limits.MaxCursorBytes <= 0 {
		return Operation{}, fmt.Errorf("openapi: cursor pagination requires a positive cursor byte limit")
	}

	if err := validateSchemaDescription(description.Schema, ""); err != nil {
		return Operation{}, err
	}
	filterFields, sortFields := collectFieldPolicies(description.Schema, "")

	parameters := make([]Parameter, 0, 9)
	if len(filterFields) > 0 {
		parameters = append(parameters, stringParameter("filter", "Boolean filter expression in gotq "+query.SyntaxVersion+" syntax", description.Limits.MaxFilterBytes, filterFields))
	}
	if len(sortFields) > 0 {
		sortParameter := stringParameter("sort", "Comma-separated public fields; prefix a field with '-' for descending order", 0, sortFields)
		sortParameter.XGotQMaxTerms = description.Limits.MaxSortTerms
		parameters = append(parameters, sortParameter)
	}
	zero := 0
	parameters = append(parameters,
		Parameter{Name: "limit", In: "query", Description: "Maximum rows to return", Schema: ValueSchema{Type: "integer", Default: description.Pagination.DefaultLimit, Minimum: &zero, Maximum: intPointer(description.Pagination.MaxLimit)}},
		Parameter{Name: "offset", In: "query", Description: "Number of rows to skip", Schema: ValueSchema{Type: "integer", Default: 0, Minimum: &zero, Maximum: intPointer(description.Pagination.MaxOffset)}},
	)
	if description.Pagination.Cursor {
		parameters = append(parameters, stringParameter("cursor", "Opaque forward cursor; cannot be combined with offset", description.Limits.MaxCursorBytes, nil))
	}
	if description.Count {
		parameters = append(parameters, Parameter{Name: "count", In: "query", Description: "Request an exact total count", Schema: ValueSchema{Type: "boolean", Default: false}})
	}
	if description.Search {
		parameters = append(parameters, stringParameter("search", "Endpoint-defined search term", description.Limits.MaxSearchBytes, nil))
	}
	if description.CompatibilityAliases {
		if len(sortFields) > 0 {
			alias := stringParameter("orderby", "Deprecated compatibility alias for sort", 0, sortFields)
			alias.Deprecated = true
			alias.XGotQMaxTerms = description.Limits.MaxSortTerms
			parameters = append(parameters, alias)
		}
		parameters = append(parameters,
			deprecatedIntegerAlias("top", "limit", description.Pagination.MaxLimit),
			deprecatedIntegerAlias("skip", "offset", description.Pagination.MaxOffset),
		)
	}

	return Operation{
		Parameters:         parameters,
		XGotQSyntaxVersion: description.SyntaxVersion,
		XGotQSchema:        description.Schema,
		XGotQLimits:        description.Limits,
		XGotQPagination:    description.Pagination,
		Responses: map[string]Response{
			"200": {Description: "A gotq page", Content: map[string]MediaType{"application/json": {Schema: ValueSchema{Type: "object"}}}},
			"400": {Description: "Invalid query", Content: map[string]MediaType{"application/json": {Schema: ValueSchema{Type: "object"}}}},
			"500": {Description: "Internal server error"},
		},
	}, nil
}

func stringParameter(name, description string, maxBytes int, fields []FieldPolicy) Parameter {
	return Parameter{
		Name:          name,
		In:            "query",
		Description:   description,
		Schema:        ValueSchema{Type: "string"},
		XGotQFields:   fields,
		XGotQMaxBytes: maxBytes,
	}
}

func deprecatedIntegerAlias(name, canonical string, maximum int) Parameter {
	zero := 0
	return Parameter{
		Name:        name,
		In:          "query",
		Description: "Deprecated compatibility alias for " + canonical,
		Deprecated:  true,
		Schema:      ValueSchema{Type: "integer", Minimum: &zero, Maximum: intPointer(maximum)},
	}
}

func intPointer(value int) *int { return &value }

func validateSchemaDescription(description query.SchemaDescription, prefix string) error {
	seen := make(map[string]struct{}, len(description.Fields)+len(description.Relationships))
	for _, field := range description.Fields {
		if field.Name == "" {
			return fmt.Errorf("openapi: field name must not be empty")
		}
		if _, duplicate := seen[field.Name]; duplicate {
			return fmt.Errorf("openapi: duplicate public name %q", prefix+field.Name)
		}
		seen[field.Name] = struct{}{}
	}
	for _, relationship := range description.Relationships {
		if relationship.Name == "" {
			return fmt.Errorf("openapi: relationship name must not be empty")
		}
		if _, duplicate := seen[relationship.Name]; duplicate {
			return fmt.Errorf("openapi: duplicate public name %q", prefix+relationship.Name)
		}
		seen[relationship.Name] = struct{}{}
		if relationship.Cardinality != query.RelationshipOne && relationship.Cardinality != query.RelationshipMany {
			return fmt.Errorf("openapi: relationship %q has invalid cardinality %q", prefix+relationship.Name, relationship.Cardinality)
		}
		if err := validateSchemaDescription(relationship.Schema, prefix+relationship.Name+"/"); err != nil {
			return err
		}
	}
	return nil
}

func collectFieldPolicies(description query.SchemaDescription, prefix string) (filterFields, sortFields []FieldPolicy) {
	fields := append([]query.FieldDescription(nil), description.Fields...)
	sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })
	for _, field := range fields {
		name := prefix + field.Name
		if field.Filterable {
			operators := make([]string, len(field.Operators))
			for index, operator := range field.Operators {
				operators[index] = operator.String()
			}
			filterFields = append(filterFields, FieldPolicy{Name: name, Kind: field.Kind.String(), Nullable: field.Nullable, Operators: operators})
		}
		if field.Sortable {
			sortFields = append(sortFields, FieldPolicy{Name: name, Kind: field.Kind.String(), Nullable: field.Nullable})
		}
	}
	relationships := append([]query.RelationshipDescription(nil), description.Relationships...)
	sort.Slice(relationships, func(i, j int) bool { return relationships[i].Name < relationships[j].Name })
	for _, relationship := range relationships {
		if relationship.Cardinality != query.RelationshipOne {
			continue
		}
		nestedFilter, nestedSort := collectFieldPolicies(relationship.Schema, prefix+relationship.Name+"/")
		filterFields = append(filterFields, nestedFilter...)
		sortFields = append(sortFields, nestedSort...)
	}
	return filterFields, sortFields
}
