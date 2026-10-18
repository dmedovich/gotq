package query

import (
	"context"
	"errors"
	"net/url"
	"reflect"
	"sync"
	"testing"
)

func TestDefaultSortIsValidatedAtStartup(t *testing.T) {
	db := newEngineTestDB(t)
	engine := newUserEngine(t, db)
	for _, sort := range []string{" ", "missing", "tenantId", "age,age", "age,", "orders/total", "-age;DROP TABLE users"} {
		config := engine.config
		config.DefaultSort = sort
		_, err := New(db, config)
		var qe *Error
		if !errors.As(err, &qe) || qe.Code != CodeInvalidSchema {
			t.Fatalf("sort %q: got %v", sort, err)
		}
	}
	config := engine.config
	config.DefaultSort = "age,name"
	config.MaxSortTerms = 1
	if err := ValidateConfig(db, config); err == nil {
		t.Fatal("default sort exceeded maximum terms")
	}
	db2, relations := seededRelationshipEngine(t)
	config2 := relations.config
	config2.DefaultSort = "company/country/code"
	config2.MaxPathDepth = 2
	if err := ValidateConfig(db2, config2); err == nil {
		t.Fatal("default sort exceeded maximum path depth")
	}
}

func TestDefaultSortListApplyAndCursorContract(t *testing.T) {
	db := newEngineTestDB(t)
	source := newUserEngine(t, db)
	config := source.config
	config.DefaultSort = "-age,name"
	engine, err := New(db, config)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	base := db.Where("tenant_id = ?", 1).Order("id DESC")
	page, err := engine.From(base).List(ctx, url.Values{"limit": {"1"}, "count": {"true"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Name != "Aaron" || page.Total == nil || *page.Total != 3 {
		t.Fatalf("page=%#v", page)
	}
	explicit, err := engine.From(base).List(ctx, url.Values{"limit": {"1"}, "sort": {"-age,name"}})
	if err != nil {
		t.Fatal(err)
	}
	if page.Page.NextCursor != explicit.Page.NextCursor {
		t.Fatal("default and explicit sort must produce the same cursor")
	}
	next, err := engine.From(base).List(ctx, url.Values{"limit": {"1"}, "cursor": {page.Page.NextCursor}})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Items) != 1 || next.Items[0].Name != "Alice" {
		t.Fatalf("continuation=%#v", next)
	}
	_, err = engine.From(base).List(ctx, url.Values{"cursor": {page.Page.NextCursor}, "sort": {"id"}})
	var qe *Error
	if !errors.As(err, &qe) || qe.Code != CodeInvalidCursor {
		t.Fatalf("wrong-sort cursor: %v", err)
	}
	overridden, err := engine.From(base).List(ctx, url.Values{"sort": {"id"}})
	if err != nil {
		t.Fatal(err)
	}
	if overridden.Items[0].Name != "Alice" {
		t.Fatal("explicit sort did not replace the default")
	}
	scope, err := engine.From(base).Apply(ctx, Query{})
	if err != nil {
		t.Fatal(err)
	}
	var users []engineTestUser
	if err = scope.Find(&users).Error; err != nil {
		t.Fatal(err)
	}
	if users[0].Name != "Aaron" {
		t.Fatal("Apply did not use default sort")
	}
	description := engine.Describe()
	if description.DefaultSort != "-age,name" {
		t.Fatal(description)
	}
	description.DefaultSort = "id"
	if engine.Describe().DefaultSort != "-age,name" {
		t.Fatal("description mutation changed engine")
	}
	// No default preserves v1's primary-key order.
	plain, err := source.From(base).List(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plain.Items[0].ID != 1 {
		t.Fatal("empty DefaultSort changed existing behavior")
	}
	// Compatibility aliases are explicit client sort and also replace defaults.
	config.AllowCompatibilityAliases = true
	legacy, err := New(db, config)
	if err != nil {
		t.Fatal(err)
	}
	plain, err = legacy.From(base).List(ctx, url.Values{"orderby": {"id desc"}})
	if err != nil {
		t.Fatal(err)
	}
	if plain.Items[0].ID != 3 {
		t.Fatal("legacy sort did not override default")
	}
}

func TestDefaultRelationshipSortKeepsNullOrderingAndStableTraversal(t *testing.T) {
	db, source := seededRelationshipEngine(t)
	config := source.config
	config.DefaultSort = "-company/name"
	engine, err := New(db, config)
	if err != nil {
		t.Fatal(err)
	}
	var ids []uint
	values := url.Values{"limit": {"1"}}
	for i := 0; i < 10; i++ {
		page, err := engine.List(context.Background(), values)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Items {
			ids = append(ids, item.ID)
		}
		if !page.Page.HasMore {
			break
		}
		values.Set("cursor", page.Page.NextCursor)
	}
	if !reflect.DeepEqual(ids, []uint{2, 1, 4, 3, 5}) {
		t.Fatalf("IDs=%v", ids)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, err := engine.List(context.Background(), url.Values{"limit": {"1"}})
			if err != nil {
				t.Error(err)
				return
			}
			if len(p.Items) != 1 || p.Items[0].ID != 2 {
				t.Errorf("page=%#v", p)
			}
		}()
	}
	wg.Wait()
}
