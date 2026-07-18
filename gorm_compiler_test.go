package query

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGORMCompilerDryRunGolden(t *testing.T) {
	t.Parallel()

	db := dryRunDB(t)
	schema, err := testUserSchema()
	if err != nil {
		t.Fatal(err)
	}
	filter := "age gt 18 and name contains 'ann'"
	expr, parseErr := parseFilter(filter, defaultQueryLimits)
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	top, skip := 20, 5
	validated, validationErr := validateQuery(schema, Query{
		Filter:       expr,
		Sort:         []SortTerm{{Field: "createdAt", Desc: true}},
		Limit:        &top,
		Offset:       &skip,
		filterSource: filter,
		sortSource:   "-createdAt",
	})
	if validationErr != nil {
		t.Fatal(validationErr)
	}
	scoped, compileErr := compileGORM(db.Model(&schemaTestUser{}), validated)
	if compileErr != nil {
		t.Fatal(compileErr)
	}
	statement := scoped.Find(&[]schemaTestUser{}).Statement

	golden, err := os.ReadFile("testdata/gorm_sqlite.golden")
	if err != nil {
		t.Fatal(err)
	}
	wantSQL := strings.TrimSpace(string(golden))
	if got := statement.SQL.String(); got != wantSQL {
		t.Fatalf("SQL = %q\nwant  %q", got, wantSQL)
	}
	wantVars := []any{int8(18), "%ann%"}
	if !reflect.DeepEqual(statement.Vars, wantVars) {
		t.Fatalf("Vars = %#v, want %#v", statement.Vars, wantVars)
	}
}

func TestGORMCompilerKeepsInjectionTextInVars(t *testing.T) {
	t.Parallel()

	db := dryRunDB(t)
	schema, err := testUserSchema()
	if err != nil {
		t.Fatal(err)
	}
	filter := "name contains 'x'' OR 1=1 --_%\\'"
	expr, parseErr := parseFilter(filter, defaultQueryLimits)
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	validated, validationErr := validateQuery(schema, Query{Filter: expr, filterSource: filter})
	if validationErr != nil {
		t.Fatal(validationErr)
	}
	scoped, compileErr := compileGORM(db.Model(&schemaTestUser{}), validated)
	if compileErr != nil {
		t.Fatal(compileErr)
	}
	statement := scoped.Find(&[]schemaTestUser{}).Statement
	if strings.Contains(statement.SQL.String(), "OR 1=1") {
		t.Fatalf("SQL contains client text: %q", statement.SQL.String())
	}
	want := "%x' OR 1=1 --!_!%\\%"
	if len(statement.Vars) != 1 || statement.Vars[0] != want {
		t.Fatalf("Vars = %#v, want escaped pattern %q", statement.Vars, want)
	}
}

func TestGORMCompilerBooleanGrouping(t *testing.T) {
	t.Parallel()

	db := dryRunDB(t)
	schema, err := testUserSchema()
	if err != nil {
		t.Fatal(err)
	}
	filter := "active eq true or age lt 18 and name ne 'bot'"
	expr, parseErr := parseFilter(filter, defaultQueryLimits)
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	validated, validationErr := validateQuery(schema, Query{Filter: expr, filterSource: filter})
	if validationErr != nil {
		t.Fatal(validationErr)
	}
	scoped, compileErr := compileGORM(db.Model(&schemaTestUser{}), validated)
	if compileErr != nil {
		t.Fatal(compileErr)
	}
	statement := scoped.Find(&[]schemaTestUser{}).Statement
	want := "SELECT * FROM `schema_test_users` WHERE (`active` = ? OR (`age` < ? AND `name` <> ?))"
	if got := statement.SQL.String(); got != want {
		t.Fatalf("SQL = %q, want %q", got, want)
	}
}

func TestContainsEscapesWildcardCharactersLiterally(t *testing.T) {
	t.Parallel()

	if got, want := literalContainsPattern(`50%_off\today!`), `%50!%!_off\today!!%`; got != want {
		t.Fatalf("literalContainsPattern() = %q, want %q", got, want)
	}
}

func TestGORMCompilerContainsAcceptsNamedStringType(t *testing.T) {
	t.Parallel()

	type model struct {
		Alias schemaTestAlias `json:"alias" gorm:"column:alias"`
	}
	schema, err := Schema[model]().Field("alias", String, Filterable()).Build()
	if err != nil {
		t.Fatal(err)
	}
	expr, parseErr := parseFilter("alias contains '50%_off'", defaultQueryLimits)
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	validated, validationErr := validateQuery(schema, Query{Filter: expr})
	if validationErr != nil {
		t.Fatal(validationErr)
	}
	scoped, compileErr := compileGORM(dryRunDB(t).Model(&model{}), validated)
	if compileErr != nil {
		t.Fatal(compileErr)
	}
	statement := scoped.Find(&[]model{}).Statement
	if got, want := statement.Vars, []any{"%50!%!_off%"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Vars = %#v, want %#v", got, want)
	}
}

func TestGORMCompilerScalarExtensions(t *testing.T) {
	t.Parallel()
	schema, err := testUserSchema()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		filter   string
		sqlParts []string
		vars     []any
	}{
		{filter: "age in (18,21)", sqlParts: []string{"`age` IN (?,?)"}, vars: []any{int8(18), int8(21)}},
		{filter: "age not in (18,21)", sqlParts: []string{"`age` NOT IN (?,?)"}, vars: []any{int8(18), int8(21)}},
		{filter: "createdAt is null", sqlParts: []string{"`created_at` IS NULL"}},
		{filter: "createdAt is not null", sqlParts: []string{"`created_at` IS NOT NULL"}},
		{filter: "not active eq true", sqlParts: []string{"`active` <> ?"}, vars: []any{true}},
		{filter: "name startswith '50%_'", sqlParts: []string{"LIKE", "ESCAPE '!'"}, vars: []any{"50!%!_%"}},
		{filter: "name endswith '50%_'", sqlParts: []string{"LIKE", "ESCAPE '!'"}, vars: []any{"%50!%!_"}},
	}
	for _, tt := range tests {
		t.Run(tt.filter, func(t *testing.T) {
			expr, parseErr := parseFilter(tt.filter, defaultQueryLimits)
			if parseErr != nil {
				t.Fatal(parseErr)
			}
			validated, validationErr := validateQuery(schema, Query{Filter: expr, filterSource: tt.filter})
			if validationErr != nil {
				t.Fatal(validationErr)
			}
			scoped, compileErr := compileGORM(dryRunDB(t).Model(&schemaTestUser{}), validated)
			if compileErr != nil {
				t.Fatal(compileErr)
			}
			statement := scoped.Find(&[]schemaTestUser{}).Statement
			for _, part := range tt.sqlParts {
				if !strings.Contains(statement.SQL.String(), part) {
					t.Fatalf("SQL = %q, want containing %q", statement.SQL.String(), part)
				}
			}
			if len(statement.Vars) != len(tt.vars) || len(tt.vars) > 0 && !reflect.DeepEqual(statement.Vars, tt.vars) {
				t.Fatalf("Vars = %#v, want %#v", statement.Vars, tt.vars)
			}
		})
	}
}

func TestSetAndPatternOperatorsKeepInjectionTextInVars(t *testing.T) {
	t.Parallel()
	schema, err := testUserSchema()
	if err != nil {
		t.Fatal(err)
	}
	filter := "name in ('x'' OR 1=1 --','safe') or name startswith 'admin%_'"
	expr, parseErr := parseFilter(filter, defaultQueryLimits)
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	validated, validationErr := validateQuery(schema, Query{Filter: expr, filterSource: filter})
	if validationErr != nil {
		t.Fatal(validationErr)
	}
	scoped, compileErr := compileGORM(dryRunDB(t).Model(&schemaTestUser{}), validated)
	if compileErr != nil {
		t.Fatal(compileErr)
	}
	statement := scoped.Find(&[]schemaTestUser{}).Statement
	if strings.Contains(statement.SQL.String(), "OR 1=1") || strings.Contains(statement.SQL.String(), "admin") {
		t.Fatalf("SQL contains client text: %q", statement.SQL.String())
	}
	want := []any{"x' OR 1=1 --", "safe", "admin!%!_%"}
	if !reflect.DeepEqual(statement.Vars, want) {
		t.Fatalf("Vars = %#v, want %#v", statement.Vars, want)
	}
}

func dryRunDB(t testing.TB) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		DryRun:               true,
		DisableAutomaticPing: true,
	})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	return db
}
