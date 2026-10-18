package query

import (
	"context"
	"errors"
	"net/url"
	"reflect"
	"sync"
	"testing"

	"gorm.io/gorm"
)

func TestListObservationsDescribeExecutedPhases(t *testing.T) {
	for _, tc := range []struct {
		name                              string
		values                            url.Values
		phases                            []QueryPhase
		code                              ErrorCode
		badScope, canceled, searchFailure bool
	}{
		{name: "data", phases: []QueryPhase{PhaseParse, PhaseValidate, PhaseData}},
		{name: "count and data", values: url.Values{"count": {"true"}}, phases: []QueryPhase{PhaseParse, PhaseValidate, PhaseCount, PhaseData}},
		{name: "zero limit", values: url.Values{"limit": {"0"}}, phases: []QueryPhase{PhaseParse, PhaseValidate}},
		{name: "count only", values: url.Values{"limit": {"0"}, "count": {"true"}}, phases: []QueryPhase{PhaseParse, PhaseValidate, PhaseCount}},
		{name: "parse error", values: url.Values{"limit": {"bad"}}, phases: []QueryPhase{PhaseParse}, code: CodeInvalidParameter},
		{name: "validation error", values: url.Values{"filter": {"missing eq 1"}}, phases: []QueryPhase{PhaseParse, PhaseValidate}, code: CodeUnknownField},
		{name: "count error", values: url.Values{"count": {"true"}}, phases: []QueryPhase{PhaseParse, PhaseValidate, PhaseCount}, code: CodeExecutionFailed, badScope: true},
		{name: "data error", phases: []QueryPhase{PhaseParse, PhaseValidate, PhaseData}, code: CodeExecutionFailed, badScope: true},
		{name: "canceled", phases: []QueryPhase{PhaseParse, PhaseValidate, PhaseData}, code: CodeExecutionFailed, canceled: true},
		{name: "count search error", values: url.Values{"count": {"true"}, "search": {"secret"}}, phases: []QueryPhase{PhaseParse, PhaseValidate, PhaseCount}, code: CodeExecutionFailed, searchFailure: true},
		{name: "data search error", values: url.Values{"search": {"secret"}}, phases: []QueryPhase{PhaseParse, PhaseValidate, PhaseData}, code: CodeExecutionFailed, searchFailure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := newEngineTestDB(t)
			config := newUserEngine(t, db).config
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.canceled {
				cancel()
			}
			var events []Observation
			config.Observe = func(got context.Context, event Observation) {
				if got != ctx {
					t.Error("observer did not receive caller context")
				}
				if event.Duration < 0 {
					t.Error("negative duration")
				}
				events = append(events, event)
			}
			if tc.searchFailure {
				config.Search = func(context.Context, *gorm.DB, string) (*gorm.DB, error) {
					return nil, errors.New("private database details")
				}
			}
			engine, err := New(db, config)
			if err != nil {
				t.Fatal(err)
			}
			base := db.Where("tenant_id = ?", 1)
			if tc.badScope {
				base = base.Where("nonexistent_column = ?", 1)
			}
			_, err = engine.From(base).List(ctx, tc.values)
			if tc.code == "" && err != nil {
				t.Fatal(err)
			}
			if tc.code != "" && err == nil {
				t.Fatal("missing operation error")
			}
			var phases []QueryPhase
			for i, event := range events {
				phases = append(phases, event.Phase)
				want := ErrorCode("")
				if i == len(events)-1 {
					want = tc.code
				}
				if event.ErrorCode != want {
					t.Fatalf("event=%#v want error=%q", event, want)
				}
			}
			if !reflect.DeepEqual(phases, tc.phases) {
				t.Fatalf("phases=%v want=%v", phases, tc.phases)
			}
		})
	}
}

func TestObserverIsOptInAndSupportsConcurrentLists(t *testing.T) {
	db := newEngineTestDB(t)
	plain := newUserEngine(t, db)
	var mu sync.Mutex
	events := 0
	config := plain.config
	config.Observe = func(_ context.Context, _ Observation) { mu.Lock(); events++; mu.Unlock() }
	engine, err := New(db, config)
	if err != nil {
		t.Fatal(err)
	}
	q, err := engine.Parse(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = engine.Apply(context.Background(), db, q); err != nil {
		t.Fatal(err)
	}
	if events != 0 {
		t.Fatal("Parse/Apply emitted List observations")
	}
	expected, err := plain.List(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := engine.List(context.Background(), nil)
			if err != nil {
				t.Error(err)
				return
			}
			if !reflect.DeepEqual(got, expected) {
				t.Error("observation changed results")
			}
		}()
	}
	wg.Wait()
	if events != 24 {
		t.Fatalf("events=%d want=24", events)
	}
}
