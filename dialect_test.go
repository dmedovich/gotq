package query

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type dialectTestUser struct {
	ID       uint    `json:"id" gorm:"primaryKey"`
	TenantID uint    `json:"tenantId"`
	Name     string  `json:"name"`
	Age      int     `json:"age"`
	Nickname *string `json:"nickname"`
}

type dialectScalarModel struct {
	ID     uint         `json:"id" gorm:"column:id"`
	Date   DateValue    `json:"date" gorm:"column:event_date"`
	UUID   UUIDValue    `json:"uuid" gorm:"column:external_id"`
	Amount DecimalValue `json:"amount" gorm:"column:amount"`
}

func (dialectTestUser) TableName() string { return "gotq_dialect_users" }

func dialectPolicy() *SchemaBuilder[dialectTestUser] {
	return Schema[dialectTestUser]().
		Expose("id", Filterable(), Sortable()).
		Expose("tenantId", Filterable(Eq)).
		Expose("name", Filterable(), Sortable()).
		Expose("age", Filterable(), Sortable()).
		Expose("nickname", Filterable())
}

func TestDialectDryRunConformance(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		dialector   gorm.Dialector
		quotedName  string
		placeholder string
	}{
		{name: "sqlite", dialector: sqlite.Open(":memory:"), quotedName: "`name`", placeholder: "?"},
		{name: "postgres", dialector: postgres.New(postgres.Config{DSN: "host=localhost user=gotq dbname=gotq sslmode=disable", PreferSimpleProtocol: true}), quotedName: `"name"`, placeholder: "$1"},
		{name: "mysql", dialector: mysql.New(mysql.Config{DSN: "gotq:gotq@tcp(localhost:3306)/gotq?parseTime=true", SkipInitializeWithVersion: true}), quotedName: "`name`", placeholder: "?"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := gorm.Open(tt.dialector, &gorm.Config{DryRun: true, DisableAutomaticPing: true})
			if err != nil {
				t.Fatal(err)
			}
			engine, err := New(db, Config[dialectTestUser]{Policy: dialectPolicy(), DefaultLimit: 10, MaxLimit: 100, MaxOffset: 100})
			if err != nil {
				t.Fatal(err)
			}
			payload := "x' OR 1=1 --"
			q, err := engine.Parse(url.Values{"filter": {"name eq 'x'' OR 1=1 --'"}, "sort": {"-age"}, "limit": {"10"}})
			if err != nil {
				t.Fatal(err)
			}
			scoped, err := engine.Apply(context.Background(), db, q)
			if err != nil {
				t.Fatal(err)
			}
			statement := scoped.Find(&[]dialectTestUser{}).Statement
			sql := statement.SQL.String()
			if strings.Contains(sql, payload) {
				t.Fatalf("SQL contains client payload: %s", sql)
			}
			if !strings.Contains(sql, tt.quotedName) || !strings.Contains(sql, tt.placeholder) {
				t.Fatalf("SQL %q lacks dialect quoting/placeholder", sql)
			}
			if len(statement.Vars) < 1 || statement.Vars[0] != payload {
				t.Fatalf("Vars = %#v, want bound payload", statement.Vars)
			}
			if !strings.Contains(sql, "ORDER BY") || !strings.Contains(sql, "id") {
				t.Fatalf("SQL lacks stable primary-key ordering: %s", sql)
			}
			extended, err := engine.Parse(url.Values{"filter": {"id in (1,2) and nickname is null and name startswith 'A' and age not in (20)"}})
			if err != nil {
				t.Fatal(err)
			}
			extendedScope, err := engine.Apply(context.Background(), db, extended)
			if err != nil {
				t.Fatal(err)
			}
			if got := extendedScope.Find(&[]dialectTestUser{}).Statement.SQL.String(); !strings.Contains(got, " IN ") || !strings.Contains(got, "IS NULL") || !strings.Contains(got, "LIKE") {
				t.Fatalf("extended SQL = %q", got)
			}
			scalarSchema, err := Schema[dialectScalarModel]().
				Field("date", Date, Filterable()).
				Field("uuid", UUID, Filterable()).
				Field("amount", Decimal, Filterable()).
				Build()
			if err != nil {
				t.Fatal(err)
			}
			scalarQuery, err := ParseHTTP(url.Values{"filter": {"date eq '2026-07-16' and uuid eq '550e8400-e29b-41d4-a716-446655440000' and amount gt 12.34"}})
			if err != nil {
				t.Fatal(err)
			}
			scalarScope, err := Apply(db.Model(&dialectScalarModel{}), scalarSchema, scalarQuery)
			if err != nil {
				t.Fatal(err)
			}
			scalarStatement := scalarScope.Find(&[]dialectScalarModel{}).Statement
			if len(scalarStatement.Vars) != 3 || strings.Contains(scalarStatement.SQL.String(), "550e8400") {
				t.Fatalf("scalar statement SQL=%q Vars=%#v", scalarStatement.SQL.String(), scalarStatement.Vars)
			}
		})
	}
}

func TestRelationshipDialectDryRunConformance(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		dialector gorm.Dialector
	}{
		{name: "sqlite", dialector: sqlite.Open(":memory:")},
		{name: "postgres", dialector: postgres.New(postgres.Config{DSN: "host=localhost user=gotq dbname=gotq sslmode=disable", PreferSimpleProtocol: true})},
		{name: "mysql", dialector: mysql.New(mysql.Config{DSN: "gotq:gotq@tcp(localhost:3306)/gotq?parseTime=true", SkipInitializeWithVersion: true})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, err := gorm.Open(test.dialector, &gorm.Config{DryRun: true, DisableAutomaticPing: true})
			if err != nil {
				t.Fatal(err)
			}
			policy, _ := relationshipPolicies()
			engine, err := New(db, Config[relationshipTestUser]{Policy: policy, DefaultLimit: 10, MaxLimit: 20, MaxOffset: 100})
			if err != nil {
				t.Fatal(err)
			}
			payload := "x' OR 1=1 --"
			parsed, err := engine.Parse(url.Values{
				"filter": {"roles/any(r: r/name eq 'x'' OR 1=1 --') and orders/all(o: o/status eq 'paid')"},
				"sort":   {"company/name"},
			})
			if err != nil {
				t.Fatal(err)
			}
			scope, err := engine.Apply(context.Background(), db, parsed)
			if err != nil {
				t.Fatal(err)
			}
			statement := scope.Find(&[]relationshipTestUser{}).Statement
			sql := statement.SQL.String()
			if strings.Contains(sql, payload) || !strings.Contains(sql, "EXISTS") || !strings.Contains(sql, "NOT EXISTS") || !strings.Contains(sql, "relationship_user_roles") || !strings.Contains(sql, "JOIN") {
				t.Fatalf("relationship SQL = %q", sql)
			}
			if !containsInterface(statement.Vars, payload) {
				t.Fatalf("Vars = %#v", statement.Vars)
			}
		})
	}
}

func TestCursorDialectDryRunConformance(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		dialector gorm.Dialector
	}{
		{name: "sqlite", dialector: sqlite.Open(":memory:")},
		{name: "postgres", dialector: postgres.New(postgres.Config{DSN: "host=localhost user=gotq dbname=gotq sslmode=disable", PreferSimpleProtocol: true})},
		{name: "mysql", dialector: mysql.New(mysql.Config{DSN: "gotq:gotq@tcp(localhost:3306)/gotq?parseTime=true", SkipInitializeWithVersion: true})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, err := gorm.Open(test.dialector, &gorm.Config{DryRun: true, DisableAutomaticPing: true})
			if err != nil {
				t.Fatal(err)
			}
			engine, err := New(db, Config[dialectTestUser]{Policy: dialectPolicy(), DefaultLimit: 2, MaxLimit: 10, MaxOffset: 100})
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := engine.Parse(url.Values{"sort": {"-age"}})
			if err != nil {
				t.Fatal(err)
			}
			validated, err := engine.validate(parsed)
			if err != nil {
				t.Fatal(err)
			}
			orders, orderErr := effectiveSort(engine.schema.modelSchema, validated.sort)
			if orderErr != nil {
				t.Fatal(orderErr)
			}
			cursor, err := encodeCursor(engine.schema.modelSchema, orders, []any{30, uint(1)}, engine.limits.MaxCursorBytes)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err = engine.Parse(url.Values{"sort": {"-age"}, "cursor": {cursor}})
			if err != nil {
				t.Fatal(err)
			}
			scope, err := engine.Apply(context.Background(), db, parsed)
			if err != nil {
				t.Fatal(err)
			}
			statement := scope.Find(&[]dialectTestUser{}).Statement
			sql := statement.SQL.String()
			if strings.Contains(sql, cursor) || !strings.Contains(sql, "CASE WHEN") || !strings.Contains(sql, "IS NULL") || !strings.Contains(sql, " OR ") {
				t.Fatalf("cursor SQL = %q Vars=%#v", sql, statement.Vars)
			}
			if !containsInterface(statement.Vars, 30) || !containsInterface(statement.Vars, uint(1)) {
				t.Fatalf("cursor Vars = %#v", statement.Vars)
			}
		})
	}
}

func containsInterface(values []any, target any) bool {
	for _, value := range values {
		if reflect.DeepEqual(value, target) {
			return true
		}
	}
	return false
}

func TestExternalDialectIntegration(t *testing.T) {
	tests := []struct {
		name      string
		env       string
		version   string
		dialector func(string) gorm.Dialector
	}{
		{name: "postgres", env: "GOTQ_POSTGRES_DSN", version: "17.", dialector: func(dsn string) gorm.Dialector { return postgres.Open(dsn) }},
		{name: "mysql", env: "GOTQ_MYSQL_DSN", version: "8.4.", dialector: func(dsn string) gorm.Dialector { return mysql.Open(dsn) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dsn := os.Getenv(tt.env)
			if dsn == "" {
				t.Skipf("%s is not configured", tt.env)
			}
			assertExternalDatabaseVersion(t, tt.name, tt.version, tt.dialector(dsn))
			runDialectIntegration(t, tt.dialector(dsn))
		})
	}
}

func TestExternalRelationshipDialectIntegration(t *testing.T) {
	tests := []struct {
		name      string
		env       string
		version   string
		dialector func(string) gorm.Dialector
	}{
		{name: "postgres", env: "GOTQ_POSTGRES_DSN", version: "17.", dialector: func(dsn string) gorm.Dialector { return postgres.Open(dsn) }},
		{name: "mysql", env: "GOTQ_MYSQL_DSN", version: "8.4.", dialector: func(dsn string) gorm.Dialector { return mysql.Open(dsn) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dsn := os.Getenv(test.env)
			if dsn == "" {
				t.Skipf("%s is not configured", test.env)
			}
			assertExternalDatabaseVersion(t, test.name, test.version, test.dialector(dsn))
			runRelationshipDialectIntegration(t, test.dialector(dsn))
		})
	}
}

func assertExternalDatabaseVersion(t *testing.T, dialect, prefix string, dialector gorm.Dialector) {
	t.Helper()
	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	query := "SELECT VERSION()"
	if dialect == "postgres" {
		query = "SHOW server_version"
	}
	var version string
	if err := db.Raw(query).Scan(&version).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(version, prefix) {
		t.Fatalf("%s server version = %q, supported line starts with %q", dialect, version, prefix)
	}
	t.Logf("%s support line: %s", dialect, version)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteRelationshipDialectIntegration(t *testing.T) {
	runRelationshipDialectIntegration(t, sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"))
}

func runRelationshipDialectIntegration(t *testing.T, dialector gorm.Dialector) {
	t.Helper()
	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	dropRelationshipTables(t, db)
	t.Cleanup(func() { dropRelationshipTables(t, db) })
	_, engine := seedRelationshipEngine(t, db)
	values := url.Values{"sort": {"company/name"}, "limit": {"2"}, "count": {"true"}}
	var ids []uint
	for {
		page, err := engine.List(context.Background(), values)
		if err != nil {
			t.Fatal(err)
		}
		if page.Total == nil || *page.Total != 5 {
			t.Fatalf("relationship cursor total = %#v", page)
		}
		ids = append(ids, relationshipUserIDs(page.Items)...)
		if !page.Page.HasMore {
			break
		}
		values.Set("cursor", page.Page.NextCursor)
	}
	if want := []uint{1, 4, 2, 3, 5}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("relationship dialect cursor IDs = %v, want %v", ids, want)
	}
}

func dropRelationshipTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, table := range []any{
		"relationship_user_roles",
		&relationshipItem{},
		&relationshipOrder{},
		&relationshipTestUser{},
		&relationshipRole{},
		&relationshipCompany{},
		&relationshipCountry{},
	} {
		if err := db.Migrator().DropTable(table); err != nil {
			t.Errorf("drop relationship table %T: %v", table, err)
		}
	}
}

func TestSQLiteDialectIntegration(t *testing.T) {
	runDialectIntegration(t, sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"))
}

func runDialectIntegration(t *testing.T, dialector gorm.Dialector) {
	t.Helper()
	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrator().DropTable(&dialectTestUser{}); err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&dialectTestUser{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Migrator().DropTable(&dialectTestUser{}) })
	rows := []dialectTestUser{
		{ID: 1, TenantID: 1, Name: "Alice", Age: 30, Nickname: stringPointer("ali")},
		{ID: 2, TenantID: 1, Name: "Aaron", Age: 30, Nickname: stringPointer("aar")},
		{ID: 3, TenantID: 1, Name: "Bob", Age: 20},
		{ID: 4, TenantID: 2, Name: "Mallory", Age: 40},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	engine, err := New(db, Config[dialectTestUser]{Policy: dialectPolicy(), DefaultLimit: 2, MaxLimit: 3, MaxOffset: 10, AllowCount: true})
	if err != nil {
		t.Fatal(err)
	}
	page, err := engine.From(db.Where("tenant_id = ?", 1)).List(context.Background(), url.Values{
		"filter": {"age gte 20"}, "sort": {"-age"}, "count": {"true"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total == nil || *page.Total != 3 || !page.Page.HasMore {
		t.Fatalf("page metadata = %#v", page)
	}
	gotIDs := []uint{page.Items[0].ID, page.Items[1].ID}
	if want := []uint{1, 2}; !reflect.DeepEqual(gotIDs, want) {
		t.Fatalf("stable IDs = %v, want %v", gotIDs, want)
	}
	for _, row := range page.Items {
		if row.TenantID != 1 {
			t.Fatal(fmt.Errorf("tenant scope leaked row %#v", row))
		}
	}
	cursorValues := url.Values{"filter": {"age gte 20"}, "sort": {"-age"}, "count": {"true"}, "cursor": {page.Page.NextCursor}}
	next, err := engine.From(db.Where("tenant_id = ?", 1)).List(context.Background(), cursorValues)
	if err != nil {
		t.Fatal(err)
	}
	if next.Total == nil || *next.Total != 3 || len(next.Items) != 1 || next.Items[0].ID != 3 || next.Page.HasMore {
		t.Fatalf("cursor continuation = %#v", next)
	}
	extended, err := engine.From(db.Where("tenant_id = ?", 1)).List(context.Background(), url.Values{
		"filter": {"id in (1,2,3) and nickname is not null and name startswith 'A' and age not in (20)"},
		"limit":  {"3"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(extended.Items) != 2 {
		t.Fatalf("extended scalar items = %#v", extended.Items)
	}
	if got, want := []uint{extended.Items[0].ID, extended.Items[1].ID}, []uint{1, 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("extended scalar IDs = %v, want %v", got, want)
	}
}

func stringPointer(value string) *string { return &value }
