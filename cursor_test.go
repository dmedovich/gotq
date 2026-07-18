package query

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"math"
	"net/url"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
)

type cursorKindsModel struct {
	ID         uint         `json:"id" gorm:"primaryKey"`
	Text       string       `json:"text"`
	Optional   *string      `json:"optional"`
	Flag       bool         `json:"flag"`
	Signed     int          `json:"signed"`
	Unsigned   uint         `json:"unsigned"`
	Ratio      float64      `json:"ratio"`
	Moment     time.Time    `json:"moment"`
	Day        DateValue    `json:"day"`
	Identifier UUIDValue    `json:"identifier"`
	Amount     DecimalValue `json:"amount"`
	Code       customCode   `json:"code"`
}

type cursorAliasCollisionModel struct {
	ID    uint   `json:"id" gorm:"primaryKey"`
	Value string `json:"value" gorm:"column:__gotq_cursor_0"`
}

type cursorAfterFindModel struct {
	ID     uint `json:"id" gorm:"primaryKey"`
	Loaded bool `json:"loaded" gorm:"-"`
}

func (model *cursorAfterFindModel) AfterFind(*gorm.DB) error {
	model.Loaded = true
	return nil
}

func TestCursorTraversesDuplicateSortValuesWithoutGaps(t *testing.T) {
	db := newEngineTestDB(t)
	engine := newUserEngine(t, db)
	values := url.Values{"sort": {"age"}, "limit": {"1"}, "count": {"true"}}
	var ids []uint
	for {
		page, err := engine.List(context.Background(), values)
		if err != nil {
			t.Fatal(err)
		}
		if page.Total == nil || *page.Total != 4 || len(page.Items) != 1 {
			t.Fatalf("page = %#v", page)
		}
		ids = append(ids, page.Items[0].ID)
		if !page.Page.HasMore {
			if page.Page.NextCursor != "" {
				t.Fatalf("terminal cursor = %q", page.Page.NextCursor)
			}
			break
		}
		if page.Page.NextCursor == "" {
			t.Fatal("non-terminal page has no cursor")
		}
		values.Set("cursor", page.Page.NextCursor)
	}
	if want := []uint{3, 1, 2, 4}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("IDs = %v, want %v", ids, want)
	}
}

func TestCursorPreservesTenantFilterSearchAndMixedSort(t *testing.T) {
	db := newEngineTestDB(t)
	engine := newUserEngine(t, db)
	values := url.Values{
		"filter": {"age gte 20"},
		"search": {"A"},
		"sort":   {"-age,id"},
		"limit":  {"1"},
	}
	session := engine.From(db.Where("tenant_id = ?", 1))
	first, err := session.List(context.Background(), values)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 1 || first.Items[0].ID != 1 || first.Page.NextCursor == "" {
		t.Fatalf("first = %#v", first)
	}
	values.Set("cursor", first.Page.NextCursor)
	second, err := session.List(context.Background(), values)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.Items[0].ID != 2 || second.Page.HasMore || second.Page.NextCursor != "" {
		t.Fatalf("second = %#v", second)
	}
}

func TestCursorTraversesNullableRelationshipSort(t *testing.T) {
	_, engine := seededRelationshipEngine(t)
	values := url.Values{"sort": {"company/name"}, "limit": {"1"}}
	var ids []uint
	for {
		page, err := engine.List(context.Background(), values)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Items) != 1 {
			t.Fatalf("page = %#v", page)
		}
		ids = append(ids, page.Items[0].ID)
		if page.Items[0].ID == 1 && page.Items[0].Employer.Name != "Acme" {
			t.Fatalf("trusted join projection was lost: %#v", page.Items[0].Employer)
		}
		if !page.Page.HasMore {
			break
		}
		values.Set("cursor", page.Page.NextCursor)
	}
	if want := []uint{1, 4, 2, 3, 5}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("IDs = %v, want %v", ids, want)
	}
}

func TestCursorTraversesNestedToOneSort(t *testing.T) {
	_, engine := seededRelationshipEngine(t)
	values := url.Values{"sort": {"company/country/code"}, "limit": {"1"}}
	var ids []uint
	for {
		page, err := engine.List(context.Background(), values)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, page.Items[0].ID)
		if !page.Page.HasMore {
			break
		}
		values.Set("cursor", page.Page.NextCursor)
	}
	if want := []uint{2, 1, 4, 3, 5}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("IDs = %v, want %v", ids, want)
	}
}

func TestCursorDescendingKeepsNullsLast(t *testing.T) {
	db := relationshipDB(t)
	if err := db.AutoMigrate(&cursorKindsModel{}); err != nil {
		t.Fatal(err)
	}
	a, b := "a", "b"
	rows := []cursorKindsModel{{ID: 1, Optional: &a}, {ID: 2}, {ID: 3, Optional: &b}}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	engine, err := New(db, Config[cursorKindsModel]{Policy: Schema[cursorKindsModel]().Expose("optional", Sortable()), DefaultLimit: 1, MaxLimit: 2, MaxOffset: 10})
	if err != nil {
		t.Fatal(err)
	}
	values := url.Values{"sort": {"-optional"}, "limit": {"1"}}
	var ids []uint
	for {
		page, listErr := engine.List(context.Background(), values)
		if listErr != nil {
			t.Fatal(listErr)
		}
		ids = append(ids, page.Items[0].ID)
		if !page.Page.HasMore {
			break
		}
		values.Set("cursor", page.Page.NextCursor)
	}
	if want := []uint{3, 1, 2}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("IDs = %v, want %v", ids, want)
	}
}

func TestCursorRejectsMalformedWrongVersionAndWrongSort(t *testing.T) {
	db := newEngineTestDB(t)
	engine := newUserEngine(t, db)
	first, err := engine.List(context.Background(), url.Values{"sort": {"age"}, "limit": {"1"}})
	if err != nil {
		t.Fatal(err)
	}
	valid := first.Page.NextCursor
	wrongVersion, err := json.Marshal(cursorEnvelope{Version: 99, Signature: "x", Values: []json.RawMessage{json.RawMessage("1")}})
	if err != nil {
		t.Fatal(err)
	}
	tests := []url.Values{
		{"sort": {"age"}, "cursor": {"not+base64"}},
		{"sort": {"age"}, "cursor": {valid[:len(valid)-1]}},
		{"sort": {"age"}, "cursor": {base64.RawURLEncoding.EncodeToString(wrongVersion)}},
		{"sort": {"name"}, "cursor": {valid}},
	}
	for _, values := range tests {
		_, listErr := engine.List(context.Background(), values)
		queryErr, ok := listErr.(*Error)
		if !ok || queryErr.Code != CodeInvalidCursor || queryErr.Parameter != "cursor" {
			t.Fatalf("List(%v) error = %#v", values, listErr)
		}
	}
}

func TestCursorPayloadIsCanonicalBoundedAndStorageIndependent(t *testing.T) {
	db := newEngineTestDB(t)
	engine := newUserEngine(t, db)
	first, err := engine.List(context.Background(), url.Values{"sort": {"age"}, "limit": {"1"}})
	if err != nil {
		t.Fatal(err)
	}
	valid := first.Page.NextCursor
	raw, err := base64.RawURLEncoding.DecodeString(valid)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range [][]byte{[]byte("age"), []byte("id"), []byte("engine_test_users"), []byte("Age"), []byte("ID")} {
		if bytes.Contains(raw, private) {
			t.Fatalf("cursor payload leaked %q: %s", private, raw)
		}
	}
	var envelope cursorEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	mutations := [][]byte{
		append([]byte(" "), raw...),
		[]byte(`{"v":1,"s":"` + envelope.Signature + `","k":[null,1]}`),
		[]byte(`{"v":1,"s":"` + envelope.Signature + `","k":["thirty",1]}`),
		[]byte(`{"v":1,"s":"` + envelope.Signature + `","k":[30]}`),
		[]byte(`{"v":1,"s":"` + envelope.Signature + `","k":[30,1],"extra":true}`),
	}
	for _, mutation := range mutations {
		encoded := base64.RawURLEncoding.EncodeToString(mutation)
		_, listErr := engine.List(context.Background(), url.Values{"sort": {"age"}, "cursor": {encoded}})
		queryErr, ok := listErr.(*Error)
		if !ok || queryErr.Code != CodeInvalidCursor {
			t.Fatalf("payload %q error = %#v", mutation, listErr)
		}
	}
	_, err = engine.List(context.Background(), url.Values{"sort": {"age"}, "cursor": {valid + "="}})
	if queryErr, ok := err.(*Error); !ok || queryErr.Code != CodeInvalidCursor {
		t.Fatalf("padded cursor error = %#v", err)
	}
}

func TestCursorEncoderRejectsUnrepresentableOrOversizedKeys(t *testing.T) {
	db := newEngineTestDB(t)
	engine := newUserEngine(t, db)
	parsed, err := engine.Parse(url.Values{"sort": {"age"}})
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
	if _, err := encodeCursor(engine.schema.modelSchema, orders, []any{30}, 4096); err == nil {
		t.Fatal("mismatched key count was encoded")
	}
	if _, err := encodeCursor(engine.schema.modelSchema, orders, []any{math.NaN(), uint(1)}, 4096); err == nil {
		t.Fatal("non-finite float was encoded")
	}
	if _, err := encodeCursor(engine.schema.modelSchema, orders, []any{30, uint(1)}, 1); err == nil {
		t.Fatal("oversized generated cursor was encoded")
	}
	if _, err := decodeCursorValue(json.RawMessage("1"), nil); err == nil {
		t.Fatal("cursor key without a target type was decoded")
	}
}

func TestLowLevelApplyRejectsCursor(t *testing.T) {
	schema, err := testUserSchema()
	if err != nil {
		t.Fatal(err)
	}
	cursor := "opaque"
	_, err = Apply(dryRunDB(t).Model(&schemaTestUser{}), schema, Query{Cursor: &cursor})
	queryErr, ok := err.(*Error)
	if !ok || queryErr.Code != CodeInvalidCursor {
		t.Fatalf("Apply error = %#v", err)
	}
}

func TestEngineRejectsManuallyConstructedCursorOffsetConflict(t *testing.T) {
	db := newEngineTestDB(t)
	engine := newUserEngine(t, db)
	cursor, offset := "opaque", 0
	_, err := engine.Apply(context.Background(), db, Query{Cursor: &cursor, Offset: &offset})
	queryErr, ok := err.(*Error)
	if !ok || queryErr.Code != CodeInvalidParameter || queryErr.Parameter != "cursor" {
		t.Fatalf("Apply error = %#v", err)
	}
}

func TestCursorEngineIsSafeForConcurrentReuse(t *testing.T) {
	db := newEngineTestDB(t)
	engine := newUserEngine(t, db)
	first, err := engine.List(context.Background(), url.Values{"sort": {"age"}, "limit": {"1"}})
	if err != nil {
		t.Fatal(err)
	}
	values := url.Values{"sort": {"age"}, "limit": {"1"}, "cursor": {first.Page.NextCursor}}
	const workers = 32
	start := make(chan struct{})
	errors := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			page, listErr := engine.List(context.Background(), values)
			if listErr != nil {
				errors <- listErr
				return
			}
			if len(page.Items) != 1 || page.Items[0].ID != 1 {
				errors <- &Error{Code: CodeExecutionFailed, Message: "unexpected concurrent cursor page"}
			}
		}()
	}
	close(start)
	wait.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
}

func TestEngineOwnsDeterministicOrderOverCallerBaseOrder(t *testing.T) {
	db := newEngineTestDB(t)
	engine := newUserEngine(t, db)
	page, err := engine.From(db.Order("name DESC")).List(context.Background(), url.Values{"limit": {"3"}})
	if err != nil {
		t.Fatal(err)
	}
	ids := []uint{page.Items[0].ID, page.Items[1].ID, page.Items[2].ID}
	if want := []uint{1, 2, 3}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("IDs = %v, want deterministic engine order %v", ids, want)
	}
}

func TestCursorPreservesCallerProjection(t *testing.T) {
	db := newEngineTestDB(t)
	engine := newUserEngine(t, db)
	values := url.Values{"sort": {"age"}, "limit": {"1"}}
	first, err := engine.From(db.Select("id")).List(context.Background(), values)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 1 || first.Items[0].ID != 3 || first.Items[0].Age != 0 || first.Items[0].Name != "" || first.Page.NextCursor == "" {
		t.Fatalf("selected page = %#v", first)
	}
	values.Set("cursor", first.Page.NextCursor)
	next, err := engine.From(db.Select("id")).List(context.Background(), values)
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Items) != 1 || next.Items[0].ID != 1 {
		t.Fatalf("selected continuation = %#v", next)
	}
	omitted, err := engine.From(db.Omit("name").Distinct()).List(context.Background(), url.Values{"limit": {"1"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(omitted.Items) != 1 || omitted.Items[0].ID != 1 || omitted.Items[0].Name != "" || omitted.Page.NextCursor == "" {
		t.Fatalf("omitted page = %#v", omitted)
	}
}

func TestCursorListPreservesCallerPreload(t *testing.T) {
	db, engine := seededRelationshipEngine(t)
	page, err := engine.From(db.Preload("Orders")).List(context.Background(), url.Values{"limit": {"1"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != 1 || len(page.Items[0].Orders) != 2 {
		t.Fatalf("page = %#v", page)
	}
}

func TestCursorListUsesOneDataQueryAndOptionalCount(t *testing.T) {
	db := newEngineTestDB(t)
	engine := newUserEngine(t, db)
	var queries atomic.Int64
	if err := db.Callback().Query().Before("gorm:query").Register("gotq:cursor_query_count", func(*gorm.DB) {
		queries.Add(1)
	}); err != nil {
		t.Fatal(err)
	}
	first, err := engine.List(context.Background(), url.Values{"sort": {"age"}, "limit": {"1"}})
	if err != nil {
		t.Fatal(err)
	}
	if queries.Load() != 1 {
		t.Fatalf("first page queries = %d", queries.Load())
	}
	_, err = engine.List(context.Background(), url.Values{"sort": {"age"}, "limit": {"1"}, "cursor": {first.Page.NextCursor}})
	if err != nil {
		t.Fatal(err)
	}
	if queries.Load() != 2 {
		t.Fatalf("continuation queries = %d", queries.Load())
	}
	_, err = engine.List(context.Background(), url.Values{"sort": {"age"}, "limit": {"1"}, "cursor": {first.Page.NextCursor}, "count": {"true"}})
	if err != nil {
		t.Fatal(err)
	}
	if queries.Load() != 4 {
		t.Fatalf("counted continuation queries = %d", queries.Load())
	}
}

func TestCursorTraversesEverySortableScalarKindAndNull(t *testing.T) {
	db := relationshipDB(t)
	if err := db.AutoMigrate(&cursorKindsModel{}); err != nil {
		t.Fatal(err)
	}
	a, b := "a", "b"
	rows := []cursorKindsModel{
		{ID: 1, Text: "a", Optional: &a, Flag: false, Signed: -2, Unsigned: 1, Ratio: 1.25, Moment: time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC), Day: "2026-01-01", Identifier: "00000000-0000-0000-0000-000000000001", Amount: "1.10", Code: "AA"},
		{ID: 2, Text: "b", Flag: false, Signed: 0, Unsigned: 2, Ratio: 2.5, Moment: time.Date(2026, 1, 2, 1, 0, 0, 0, time.UTC), Day: "2026-01-02", Identifier: "00000000-0000-0000-0000-000000000002", Amount: "2.20", Code: "BB"},
		{ID: 3, Text: "c", Optional: &b, Flag: true, Signed: 4, Unsigned: 3, Ratio: 3.75, Moment: time.Date(2026, 1, 3, 1, 0, 0, 0, time.UTC), Day: "2026-01-03", Identifier: "00000000-0000-0000-0000-000000000003", Amount: "3.30", Code: "CC"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	policy := Schema[cursorKindsModel]().
		Expose("text", Sortable()).
		Expose("optional", Sortable()).
		Expose("flag", Sortable()).
		Expose("signed", Sortable()).
		Expose("unsigned", Sortable()).
		Expose("ratio", Sortable()).
		Expose("moment", Sortable()).
		Expose("day", Sortable()).
		Expose("identifier", Sortable()).
		Expose("amount", Sortable()).
		Expose("code", WithCodec(upperCodeCodec{}), Sortable())
	engine, err := New(db, Config[cursorKindsModel]{Policy: policy, DefaultLimit: 1, MaxLimit: 3, MaxOffset: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"text", "flag", "signed", "unsigned", "ratio", "moment", "day", "identifier", "amount", "code"} {
		t.Run(field, func(t *testing.T) {
			if got := traverseCursorKindIDs(t, engine, field); !reflect.DeepEqual(got, []uint{1, 2, 3}) {
				t.Fatalf("IDs = %v", got)
			}
		})
	}
	if got := traverseCursorKindIDs(t, engine, "optional"); !reflect.DeepEqual(got, []uint{1, 3, 2}) {
		t.Fatalf("nullable IDs = %v", got)
	}
}

func traverseCursorKindIDs(t *testing.T, engine *Engine[cursorKindsModel], field string) []uint {
	t.Helper()
	values := url.Values{"sort": {field}, "limit": {"1"}}
	var ids []uint
	for {
		page, err := engine.List(context.Background(), values)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, page.Items[0].ID)
		if !page.Page.HasMore {
			return ids
		}
		values.Set("cursor", page.Page.NextCursor)
	}
}

func TestCursorTraversesCompositePrimaryKey(t *testing.T) {
	db := relationshipDB(t)
	if err := db.AutoMigrate(&compositeAccount{}); err != nil {
		t.Fatal(err)
	}
	accounts := []compositeAccount{{TenantID: 2, Code: "b"}, {TenantID: 1, Code: "b"}, {TenantID: 1, Code: "a"}}
	if err := db.Create(&accounts).Error; err != nil {
		t.Fatal(err)
	}
	engine, err := New(db, Config[compositeAccount]{Policy: Schema[compositeAccount](), DefaultLimit: 1, MaxLimit: 2, MaxOffset: 10})
	if err != nil {
		t.Fatal(err)
	}
	values := url.Values{"limit": {"1"}}
	var keys []string
	for {
		page, listErr := engine.List(context.Background(), values)
		if listErr != nil {
			t.Fatal(listErr)
		}
		keys = append(keys, string(rune('0'+page.Items[0].TenantID))+page.Items[0].Code)
		if !page.Page.HasMore {
			break
		}
		values.Set("cursor", page.Page.NextCursor)
	}
	if want := []string{"1a", "1b", "2b"}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("keys = %v, want %v", keys, want)
	}
}

func TestCursorSelectAliasCannotCollideWithModelColumn(t *testing.T) {
	db := relationshipDB(t)
	if err := db.AutoMigrate(&cursorAliasCollisionModel{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]cursorAliasCollisionModel{{ID: 1, Value: "b"}, {ID: 2, Value: "a"}}).Error; err != nil {
		t.Fatal(err)
	}
	engine, err := New(db, Config[cursorAliasCollisionModel]{Policy: Schema[cursorAliasCollisionModel]().Expose("value", Sortable()), DefaultLimit: 1, MaxLimit: 2, MaxOffset: 10})
	if err != nil {
		t.Fatal(err)
	}
	first, err := engine.List(context.Background(), url.Values{"sort": {"value"}, "limit": {"1"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 1 || first.Items[0].ID != 2 || first.Page.NextCursor == "" {
		t.Fatalf("first = %#v", first)
	}
	next, err := engine.List(context.Background(), url.Values{"sort": {"value"}, "limit": {"1"}, "cursor": {first.Page.NextCursor}})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Items) != 1 || next.Items[0].ID != 1 || next.Page.HasMore {
		t.Fatalf("next = %#v", next)
	}
}

func TestCursorListPreservesModelAfterFindHook(t *testing.T) {
	db := relationshipDB(t)
	if err := db.AutoMigrate(&cursorAfterFindModel{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]cursorAfterFindModel{{ID: 1}, {ID: 2}}).Error; err != nil {
		t.Fatal(err)
	}
	engine, err := New(db, Config[cursorAfterFindModel]{Policy: Schema[cursorAfterFindModel](), DefaultLimit: 1, MaxLimit: 2, MaxOffset: 10})
	if err != nil {
		t.Fatal(err)
	}
	page, err := engine.List(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || !page.Items[0].Loaded {
		t.Fatalf("page = %#v", page)
	}
	withoutHooks, err := engine.From(db.Session(&gorm.Session{SkipHooks: true})).List(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(withoutHooks.Items) != 1 || withoutHooks.Items[0].Loaded {
		t.Fatalf("SkipHooks page = %#v", withoutHooks)
	}
}
