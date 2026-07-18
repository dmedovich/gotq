package query

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

type schemaTestUser struct {
	ID        uint       `json:"id" gorm:"column:id"`
	Quota     uint16     `json:"quota" gorm:"column:quota"`
	Name      string     `json:"name" gorm:"column:name"`
	Age       int8       `json:"age" gorm:"column:age"`
	Score     float32    `json:"score" gorm:"column:score"`
	Active    bool       `json:"active" gorm:"column:active"`
	CreatedAt *time.Time `json:"createdAt" gorm:"column:created_at"`
}

type schemaTestAlias string

func TestSchemaBuildResolvesMappingsAndDefaults(t *testing.T) {
	t.Parallel()

	schema, err := testUserSchema()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	name := schema.fields["name"]
	if name.goName != "Name" || name.column != "name" || name.kind != String || !name.filterable || !name.sortable {
		t.Fatalf("name field = %#v", name)
	}
	if got, want := joinOperators(name.operators), "eq, ne, contains, startswith, endswith, in, not in"; got != want {
		t.Fatalf("name operators = %q, want %q", got, want)
	}
	age := schema.fields["age"]
	if age.bits != 8 || gotOperators(age) != "eq, ne, gt, gte, lt, lte, in, not in" {
		t.Fatalf("age field = %#v", age)
	}
}

func TestSchemaExplicitMappingAndOperatorSubset(t *testing.T) {
	t.Parallel()

	type model struct{ Display string }
	schema, err := Schema[model]().
		Field("name", String, GoField("Display"), Column("display_name"), Filterable(Eq)).
		Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	field := schema.fields["name"]
	if field.goName != "Display" || field.column != "display_name" || gotOperators(field) != "eq" {
		t.Fatalf("field = %#v", field)
	}
}

func TestSchemaPreservesNamedScalarType(t *testing.T) {
	t.Parallel()

	type model struct {
		Alias schemaTestAlias `json:"alias" gorm:"column:alias"`
	}
	schema, err := Schema[model]().Field("alias", String, Filterable()).Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got, want := schema.fields["alias"].scalarType, reflect.TypeOf(schemaTestAlias("")); got != want {
		t.Fatalf("scalar type = %v, want %v", got, want)
	}
}

func TestSchemaRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		build func() error
		part  string
	}{
		{name: "model not struct", build: func() error { _, err := Schema[*schemaTestUser]().Build(); return err }, part: "must be a struct"},
		{name: "invalid public name", build: func() error {
			_, err := Schema[schemaTestUser]().Field("user.name", String, GoField("Name"), Column("name")).Build()
			return err
		}, part: "not a valid identifier"},
		{name: "reserved public name", build: func() error {
			_, err := Schema[schemaTestUser]().Field("and", String, GoField("Name"), Column("name")).Build()
			return err
		}, part: "is reserved"},
		{name: "duplicate public name", build: func() error {
			_, err := Schema[schemaTestUser]().Field("name", String).Field("name", String).Build()
			return err
		}, part: "more than once"},
		{name: "missing Go field", build: func() error {
			_, err := Schema[schemaTestUser]().Field("missing", String, Column("missing")).Build()
			return err
		}, part: "does not match"},
		{name: "kind mismatch", build: func() error { _, err := Schema[schemaTestUser]().Field("age", String).Build(); return err }, part: "does not match"},
		{name: "missing column", build: func() error {
			type model struct {
				Name string `json:"name"`
			}
			_, err := Schema[model]().Field("name", String).Build()
			return err
		}, part: "no explicit database column"},
		{name: "unsafe column", build: func() error {
			_, err := Schema[schemaTestUser]().Field("name", String, Column("name DESC")).Build()
			return err
		}, part: "not a simple identifier"},
		{name: "incompatible operator", build: func() error {
			_, err := Schema[schemaTestUser]().Field("age", Int, Filterable(Contains)).Build()
			return err
		}, part: "incompatible"},
		{name: "null operator on non-nullable field", build: func() error {
			_, err := Schema[schemaTestUser]().Field("age", Int, Filterable(IsNull)).Build()
			return err
		}, part: "requires nullable"},
		{name: "duplicate operator", build: func() error {
			_, err := Schema[schemaTestUser]().Field("age", Int, Filterable(Eq, Eq)).Build()
			return err
		}, part: "repeated"},
		{name: "duplicate option", build: func() error {
			_, err := Schema[schemaTestUser]().Field("age", Int, Filterable(), Filterable(Eq)).Build()
			return err
		}, part: "specified more than once"},
		{name: "empty explicit column", build: func() error {
			_, err := Schema[schemaTestUser]().Field("age", Int, Column("")).Build()
			return err
		}, part: "empty Column"},
		{name: "uintptr is not an API integer", build: func() error {
			type model struct {
				Value uintptr `json:"value" gorm:"column:value"`
			}
			_, err := Schema[model]().Field("value", Uint).Build()
			return err
		}, part: "does not match"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.build()
			if err == nil || !strings.Contains(err.Error(), tt.part) {
				t.Fatalf("Build() error = %v, want containing %q", err, tt.part)
			}
			queryErr, ok := err.(*Error)
			if !ok || queryErr.Code != CodeInvalidSchema || queryErr.Position != nil {
				t.Fatalf("Build() error = %#v, want positionless invalid_schema", err)
			}
		})
	}
}

func testUserSchema() (*ModelSchema[schemaTestUser], error) {
	return Schema[schemaTestUser]().
		Field("id", Uint, Sortable()).
		Field("quota", Uint, Filterable()).
		Field("name", String, Filterable(), Sortable()).
		Field("age", Int, Filterable()).
		Field("score", Float, Filterable()).
		Field("active", Bool, Filterable()).
		Field("createdAt", Time, Filterable(), Sortable()).
		Build()
}

func gotOperators(field *modelField) string { return joinOperators(field.operators) }
