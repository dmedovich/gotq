package query

import (
	"context"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
)

func seededRelationshipEngine(t *testing.T) (*gorm.DB, *Engine[relationshipTestUser]) {
	t.Helper()
	db := relationshipDB(t)
	return seedRelationshipEngine(t, db)
}

func seedRelationshipEngine(t *testing.T, db *gorm.DB) (*gorm.DB, *Engine[relationshipTestUser]) {
	t.Helper()
	if err := db.AutoMigrate(
		&relationshipCountry{},
		&relationshipCompany{},
		&relationshipTestUser{},
		&relationshipOrder{},
		&relationshipItem{},
		&relationshipRole{},
	); err != nil {
		t.Fatal(err)
	}
	countries := []relationshipCountry{{ID: 1, Code: "US"}, {ID: 2, Code: "GB"}}
	companies := []relationshipCompany{{ID: 1, Name: "Acme", CountryID: 1}, {ID: 2, Name: "Beta", CountryID: 2}, {ID: 3, Name: "Ghost", CountryID: 1, DeletedAt: deletedAt(time.Now())}}
	users := []relationshipTestUser{
		{ID: 1, CompanyID: relationshipUint(1)},
		{ID: 2, CompanyID: relationshipUint(2)},
		{ID: 3},
		{ID: 4, CompanyID: relationshipUint(1)},
		{ID: 5, CompanyID: relationshipUint(3)},
	}
	orders := []relationshipOrder{
		{ID: 1, UserID: 1, Total: 50, Status: relationshipString("pending")},
		{ID: 2, UserID: 1, Total: 150, Status: relationshipString("paid")},
		{ID: 3, UserID: 2, Total: 200, Status: relationshipString("paid")},
		{ID: 4, UserID: 3, Total: 999, Status: relationshipString("failed"), DeletedAt: deletedAt(time.Now())},
		{ID: 5, UserID: 4, Total: 10},
	}
	items := []relationshipItem{
		{ID: 1, OrderID: 1, Price: 5},
		{ID: 2, OrderID: 2, Price: 20},
		{ID: 3, OrderID: 3, Price: 7},
		{ID: 4, OrderID: 3, Price: 100, DeletedAt: deletedAt(time.Now())},
	}
	roles := []relationshipRole{{ID: 1, Name: "admin"}, {ID: 2, Name: "member"}}
	for _, value := range []any{&countries, &companies, &users, &orders, &items, &roles} {
		if err := db.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Model(&users[0]).Association("Roles").Append(&roles[0], &roles[1]); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&users[1]).Association("Roles").Append(&roles[1]); err != nil {
		t.Fatal(err)
	}
	policy, _ := relationshipPolicies()
	engine, err := New(db, Config[relationshipTestUser]{
		Policy:             policy,
		DefaultLimit:       10,
		MaxLimit:           20,
		MaxOffset:          100,
		AllowCount:         true,
		MaxPathDepth:       6,
		MaxQuantifierDepth: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	return db, engine
}

func TestRelationshipFiltersQuantifiersAndSoftDelete(t *testing.T) {
	_, engine := seededRelationshipEngine(t)
	tests := []struct {
		filter string
		ids    []uint
	}{
		{filter: "company/name eq 'Acme'", ids: []uint{1, 4}},
		{filter: "company/country/code eq 'US'", ids: []uint{1, 4}},
		{filter: "orders/any(o: o/total gt 100)", ids: []uint{1, 2}},
		{filter: "orders/all(o: o/status eq 'paid')", ids: []uint{2, 3, 5}},
		{filter: "not orders/any(o: o/total gt 0)", ids: []uint{3, 5}},
		{filter: "roles/any(r: r/name eq 'admin')", ids: []uint{1}},
		{filter: "orders/any(o: o/items/any(i: i/price gte 10))", ids: []uint{1}},
	}
	for _, test := range tests {
		t.Run(test.filter, func(t *testing.T) {
			page, err := engine.List(context.Background(), url.Values{"filter": {test.filter}})
			if err != nil {
				t.Fatal(err)
			}
			if got := relationshipUserIDs(page.Items); !reflect.DeepEqual(got, test.ids) {
				t.Fatalf("IDs = %v, want %v", got, test.ids)
			}
		})
	}
}

func TestRelationshipCountPaginationAndToOneSort(t *testing.T) {
	_, engine := seededRelationshipEngine(t)
	page, err := engine.List(context.Background(), url.Values{
		"filter": {"orders/any(o: o/total gt 100)"},
		"sort":   {"company/name"},
		"limit":  {"1"},
		"count":  {"true"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total == nil || *page.Total != 2 || len(page.Items) != 1 || page.Items[0].ID != 1 || !page.Page.HasMore {
		t.Fatalf("page = %#v", page)
	}

	all, err := engine.List(context.Background(), url.Values{"sort": {"-company/name"}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := relationshipUserIDs(all.Items), []uint{2, 1, 4, 3, 5}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sorted IDs = %v, want %v", got, want)
	}
}

func TestRelationshipSoftDeleteFollowsCallerUnscopedPolicy(t *testing.T) {
	db, engine := seededRelationshipEngine(t)
	values := url.Values{"filter": {"orders/any(o: o/total gt 900)"}}
	page, err := engine.List(context.Background(), values)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 0 {
		t.Fatalf("default scope included soft-deleted relation: %#v", page.Items)
	}
	page, err = engine.From(db.Unscoped()).List(context.Background(), values)
	if err != nil {
		t.Fatal(err)
	}
	if got := relationshipUserIDs(page.Items); !reflect.DeepEqual(got, []uint{3}) {
		t.Fatalf("Unscoped relation IDs = %v", got)
	}
	companyValues := url.Values{"filter": {"company/name eq 'Ghost'"}}
	page, err = engine.List(context.Background(), companyValues)
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("default to-one soft-delete page = %#v, err = %v", page, err)
	}
	page, err = engine.From(db.Unscoped()).List(context.Background(), companyValues)
	if err != nil {
		t.Fatal(err)
	}
	if got := relationshipUserIDs(page.Items); !reflect.DeepEqual(got, []uint{5}) {
		t.Fatalf("Unscoped to-one IDs = %v", got)
	}
}

func TestRelationshipValidationRejectsUndisclosedPathsAndBadScopes(t *testing.T) {
	_, engine := seededRelationshipEngine(t)
	tests := []struct {
		parameter string
		value     string
		code      ErrorCode
	}{
		{parameter: "filter", value: "orders/total gt 1", code: CodeInvalidRelationship},
		{parameter: "filter", value: "missing/name eq 'x'", code: CodeInvalidRelationship},
		{parameter: "filter", value: "orders/any(o: total gt 1)", code: CodeInvalidRelationship},
		{parameter: "filter", value: "orders/any(o: o/items/any(o: o/price gte 1))", code: CodeInvalidRelationship},
		{parameter: "sort", value: "orders/total", code: CodeInvalidRelationship},
	}
	for _, test := range tests {
		_, err := engine.List(context.Background(), url.Values{test.parameter: {test.value}})
		queryErr, ok := err.(*Error)
		if !ok || queryErr.Code != test.code || queryErr.Parameter != test.parameter {
			t.Fatalf("%s=%q error = %#v", test.parameter, test.value, err)
		}
	}
}

func TestRelationshipSQLKeepsValuesBoundAndAliasesTrusted(t *testing.T) {
	db, engine := seededRelationshipEngine(t)
	payload := "x' OR 1=1 --"
	parsed, err := engine.Parse(url.Values{"filter": {"roles/any(r: r/name eq 'x'' OR 1=1 --')"}})
	if err != nil {
		t.Fatal(err)
	}
	scope, err := engine.Apply(context.Background(), db.Session(&gorm.Session{DryRun: true}), parsed)
	if err != nil {
		t.Fatal(err)
	}
	statement := scope.Find(&[]relationshipTestUser{}).Statement
	if strings.Contains(statement.SQL.String(), payload) || !strings.Contains(statement.SQL.String(), "gotq_rel_2") || len(statement.Vars) == 0 || statement.Vars[len(statement.Vars)-1] != payload {
		t.Fatalf("SQL=%q Vars=%#v", statement.SQL.String(), statement.Vars)
	}
}

func TestRelationshipSortDeduplicatesTrustedToOneJoins(t *testing.T) {
	db, engine := seededRelationshipEngine(t)
	parsed, err := engine.Parse(url.Values{"sort": {"company/name,company/country/code"}})
	if err != nil {
		t.Fatal(err)
	}
	scope, err := engine.Apply(context.Background(), db.Session(&gorm.Session{DryRun: true}), parsed)
	if err != nil {
		t.Fatal(err)
	}
	sql := scope.Find(&[]relationshipTestUser{}).Statement.SQL.String()
	if strings.Count(sql, "relationship_companies") != 1 || strings.Count(sql, "relationship_countries") != 1 {
		t.Fatalf("joins were not deduplicated: %s", sql)
	}
	if !strings.Contains(sql, "relationship_test_users") || !strings.Contains(sql, "ORDER BY") {
		t.Fatalf("stable root ordering is not qualified: %s", sql)
	}
}

func TestRelationshipEngineConcurrentReuse(t *testing.T) {
	_, engine := seededRelationshipEngine(t)
	const workers = 12
	errors := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			page, err := engine.List(context.Background(), url.Values{"filter": {"orders/any(o: o/total gt 100)"}, "sort": {"company/name"}})
			if err == nil && !reflect.DeepEqual(relationshipUserIDs(page.Items), []uint{1, 2}) {
				err = &testEngineError{"unexpected relationship result"}
			}
			errors <- err
		}()
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func relationshipUserIDs(users []relationshipTestUser) []uint {
	ids := make([]uint, len(users))
	for index, user := range users {
		ids[index] = user.ID
	}
	return ids
}

func relationshipUint(value uint) *uint { return &value }
