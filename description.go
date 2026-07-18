package query

import (
	"sort"

	"gorm.io/gorm"
)

// SyntaxVersion is the stable query-language version implemented by this
// release. Tooling can use it to reject incompatible descriptions.
const SyntaxVersion = "v1"

// FieldDescription is the public, storage-independent view of one field.
// It intentionally contains neither Go field names nor database identifiers.
type FieldDescription struct {
	Name       string               `json:"name"`
	Kind       Kind                 `json:"kind"`
	Nullable   bool                 `json:"nullable"`
	Filterable bool                 `json:"filterable"`
	Sortable   bool                 `json:"sortable"`
	Operators  []ComparisonOperator `json:"operators"`
	Codec      string               `json:"codec,omitempty"`
}

// RelationshipCardinality describes whether a relationship resolves to one
// resource or a collection. Relationship declarations are introduced in M5;
// the type is present now so consumers can rely on a stable description shape.
type RelationshipCardinality string

const (
	RelationshipOne  RelationshipCardinality = "one"
	RelationshipMany RelationshipCardinality = "many"
)

// RelationshipDescription is the public view of a relationship policy.
type RelationshipDescription struct {
	Name        string                  `json:"name"`
	Cardinality RelationshipCardinality `json:"cardinality"`
	Filterable  bool                    `json:"filterable"`
	Sortable    bool                    `json:"sortable"`
	Schema      SchemaDescription       `json:"schema"`
}

// SchemaDescription is a deterministic, read-only projection of a model
// policy. Fields and relationships are sorted by public name.
type SchemaDescription struct {
	Fields        []FieldDescription        `json:"fields"`
	Relationships []RelationshipDescription `json:"relationships"`
}

// PaginationDescription documents the effective offset and cursor policy.
type PaginationDescription struct {
	DefaultLimit int  `json:"defaultLimit"`
	MaxLimit     int  `json:"maxLimit"`
	MaxOffset    int  `json:"maxOffset"`
	Cursor       bool `json:"cursor"`
}

// EndpointDescription is safe to publish as endpoint documentation. It only
// describes public query names and effective limits.
type EndpointDescription struct {
	SyntaxVersion        string                `json:"syntaxVersion"`
	Schema               SchemaDescription     `json:"schema"`
	Pagination           PaginationDescription `json:"pagination"`
	Limits               Limits                `json:"limits"`
	Count                bool                  `json:"count"`
	Search               bool                  `json:"search"`
	CompatibilityAliases bool                  `json:"compatibilityAliases"`
}

// Describe returns a detached public description. Mutating its slices cannot
// change the immutable schema.
func (s *ModelSchema[T]) Describe() SchemaDescription {
	if s == nil {
		return emptySchemaDescription()
	}
	return describeModelSchema(s.modelSchema)
}

func emptySchemaDescription() SchemaDescription {
	return SchemaDescription{
		Fields:        make([]FieldDescription, 0),
		Relationships: make([]RelationshipDescription, 0),
	}
}

func describeModelSchema(s *modelSchema) SchemaDescription {
	description := SchemaDescription{
		Fields:        make([]FieldDescription, 0),
		Relationships: make([]RelationshipDescription, 0),
	}
	if s == nil {
		return description
	}
	description.Fields = make([]FieldDescription, 0, len(s.fields))
	for _, field := range s.fields {
		description.Fields = append(description.Fields, FieldDescription{
			Name:       field.publicName,
			Kind:       field.kind,
			Nullable:   field.nullable,
			Filterable: field.filterable,
			Sortable:   field.sortable,
			Operators:  append([]ComparisonOperator(nil), field.operators...),
			Codec:      field.codecName,
		})
	}
	sort.Slice(description.Fields, func(i, j int) bool {
		return description.Fields[i].Name < description.Fields[j].Name
	})
	description.Relationships = make([]RelationshipDescription, 0, len(s.relationships))
	for _, relationship := range s.relationships {
		target := describeModelSchema(relationship.target)
		description.Relationships = append(description.Relationships, RelationshipDescription{
			Name:        relationship.publicName,
			Cardinality: relationship.cardinality,
			Filterable:  schemaHasFilter(target),
			Sortable:    relationship.cardinality == RelationshipOne && schemaHasSort(target),
			Schema:      target,
		})
	}
	sort.Slice(description.Relationships, func(i, j int) bool {
		return description.Relationships[i].Name < description.Relationships[j].Name
	})
	return description
}

func schemaHasFilter(description SchemaDescription) bool {
	for _, field := range description.Fields {
		if field.Filterable {
			return true
		}
	}
	for _, relationship := range description.Relationships {
		if relationship.Filterable {
			return true
		}
	}
	return false
}

func schemaHasSort(description SchemaDescription) bool {
	for _, field := range description.Fields {
		if field.Sortable {
			return true
		}
	}
	for _, relationship := range description.Relationships {
		if relationship.Sortable {
			return true
		}
	}
	return false
}

// Describe returns the endpoint's detached, effective public policy.
func (e *Engine[T]) Describe() EndpointDescription {
	if e == nil {
		return EndpointDescription{
			SyntaxVersion: SyntaxVersion,
			Schema: SchemaDescription{
				Fields:        make([]FieldDescription, 0),
				Relationships: make([]RelationshipDescription, 0),
			},
		}
	}
	return EndpointDescription{
		SyntaxVersion: SyntaxVersion,
		Schema:        e.schema.Describe(),
		Pagination: PaginationDescription{
			DefaultLimit: e.config.DefaultLimit,
			MaxLimit:     e.config.MaxLimit,
			MaxOffset:    e.config.MaxOffset,
			Cursor:       true,
		},
		Limits:               e.limits,
		Count:                e.config.AllowCount,
		Search:               e.config.Search != nil,
		CompatibilityAliases: e.config.AllowCompatibilityAliases,
	}
}

// ValidateConfig checks a complete endpoint policy against the active GORM
// model metadata. It is intended for startup checks and CI policy tests.
func ValidateConfig[T any](db *gorm.DB, config Config[T]) error {
	_, err := New(db, config)
	return err
}
