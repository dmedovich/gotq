package query

import (
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestParseHTTPCompleteQuery(t *testing.T) {
	t.Parallel()

	values := url.Values{
		"filter": {"age gt 18 and name contains 'ann'"},
		"sort":   {"-createdAt,name"},
		"limit":  {"20"},
		"offset": {"5"},
		"count":  {"true"},
		"search": {"ann"},
	}
	query, err := ParseHTTP(values)
	if err != nil {
		t.Fatalf("ParseHTTP() error = %v", err)
	}
	if got, want := formatTestExpr(query.Filter), "(age gt 18 and name contains 'ann')"; got != want {
		t.Errorf("filter = %q, want %q", got, want)
	}
	wantSort := []SortTerm{
		{Field: "createdAt", Desc: true, Source: Span{0, 10}},
		{Field: "name", Source: Span{11, 15}},
	}
	if !reflect.DeepEqual(query.Sort, wantSort) {
		t.Errorf("sort = %#v, want %#v", query.Sort, wantSort)
	}
	if query.Limit == nil || *query.Limit != 20 || query.Offset == nil || *query.Offset != 5 || query.Count == nil || !*query.Count || query.Search == nil || *query.Search != "ann" {
		t.Errorf("pagination/count/search = limit:%v offset:%v count:%v search:%v", query.Limit, query.Offset, query.Count, query.Search)
	}
}

func TestParseHTTPRejectsParameterAmbiguityAndMalformedScalars(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		values    url.Values
		code      ErrorCode
		parameter string
	}{
		{name: "empty filter", values: url.Values{"filter": {""}}, code: CodeInvalidParameter, parameter: "filter"},
		{name: "duplicate filter", values: url.Values{"filter": {"age eq 1", "age eq 2"}}, code: CodeInvalidParameter, parameter: "filter"},
		{name: "limit leading zero", values: url.Values{"limit": {"01"}}, code: CodeInvalidParameter, parameter: "limit"},
		{name: "limit negative", values: url.Values{"limit": {"-1"}}, code: CodeInvalidParameter, parameter: "limit"},
		{name: "limit whitespace", values: url.Values{"limit": {" 1"}}, code: CodeInvalidParameter, parameter: "limit"},
		{name: "limit above default", values: url.Values{"limit": {"101"}}, code: CodeLimitExceeded, parameter: "limit"},
		{name: "offset above default", values: url.Values{"offset": {"100001"}}, code: CodeLimitExceeded, parameter: "offset"},
		{name: "offset overflow", values: url.Values{"offset": {strconv.FormatUint(uint64(maxInt())+1, 10)}}, code: CodeInvalidParameter, parameter: "offset"},
		{name: "count case", values: url.Values{"count": {"True"}}, code: CodeInvalidParameter, parameter: "count"},
		{name: "empty search", values: url.Values{"search": {""}}, code: CodeInvalidParameter, parameter: "search"},
		{name: "empty cursor", values: url.Values{"cursor": {""}}, code: CodeInvalidParameter, parameter: "cursor"},
		{name: "cursor with offset", values: url.Values{"cursor": {"abc"}, "offset": {"1"}}, code: CodeInvalidParameter, parameter: "cursor"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseHTTP(tt.values)
			queryErr, ok := err.(*Error)
			if !ok || queryErr.Code != tt.code || queryErr.Parameter != tt.parameter {
				t.Fatalf("ParseHTTP() error = %#v, want %q for %q", err, tt.code, tt.parameter)
			}
		})
	}
}

func TestParseHTTPSortGrammar(t *testing.T) {
	t.Parallel()

	valid := []string{"-createdAt,name", " name , -createdAt "}
	for _, input := range valid {
		if _, err := ParseHTTP(url.Values{"sort": {input}}); err != nil {
			t.Errorf("ParseHTTP(sort=%q) error = %v", input, err)
		}
	}
	invalid := []string{"name,", "name,,age", "name DESC", "name,name", "-", "   "}
	for _, input := range invalid {
		_, err := ParseHTTP(url.Values{"sort": {input}})
		queryErr, ok := err.(*Error)
		if !ok || queryErr.Code != CodeInvalidParameter || queryErr.Parameter != "sort" {
			t.Errorf("ParseHTTP(sort=%q) error = %#v", input, err)
		}
	}
}

func TestParseHTTPCompatibilityAliases(t *testing.T) {
	t.Parallel()

	ignored, err := ParseHTTP(url.Values{"orderby": {"createdAt desc"}, "top": {"20"}, "skip": {"5"}})
	if err != nil || ignored.Sort != nil || ignored.Limit != nil || ignored.Offset != nil {
		t.Fatalf("aliases without option = (%#v, %v), want ignored", ignored, err)
	}
	parsed, err := ParseHTTP(url.Values{"orderby": {"createdAt desc"}, "top": {"20"}, "skip": {"5"}}, WithCompatibilityAliases())
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Sort) != 1 || !parsed.Sort[0].Desc || parsed.Limit == nil || *parsed.Limit != 20 || parsed.Offset == nil || *parsed.Offset != 5 {
		t.Fatalf("compatibility query = %#v", parsed)
	}
	_, err = ParseHTTP(url.Values{"sort": {"name"}, "orderby": {"name asc"}}, WithCompatibilityAliases())
	queryErr, ok := err.(*Error)
	if !ok || queryErr.Code != CodeInvalidParameter || queryErr.Parameter != "sort" {
		t.Fatalf("ambiguous aliases error = %#v", err)
	}
}

func TestParseHTTPRelationshipSortPaths(t *testing.T) {
	query, err := ParseHTTP(url.Values{"sort": {"-company/country/code,company/name"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(query.Sort) != 2 || query.Sort[0].Field != "company/country/code" || !query.Sort[0].Desc || query.Sort[1].Field != "company/name" {
		t.Fatalf("sort = %#v", query.Sort)
	}
	query, err = ParseHTTP(url.Values{"orderby": {"company/name desc"}}, WithCompatibilityAliases())
	if err != nil {
		t.Fatal(err)
	}
	if len(query.Sort) != 1 || query.Sort[0].Field != "company/name" || !query.Sort[0].Desc {
		t.Fatalf("legacy sort = %#v", query.Sort)
	}
}

func TestParseHTTPCustomLimits(t *testing.T) {
	t.Parallel()

	limits := Limits{MaxQueryBytes: 100, MaxFilterBytes: 80, MaxTokens: 10, MaxLiteralBytes: 10, MaxInValues: 5, MaxLimit: 5, MaxOffset: 10, MaxSortTerms: 1, MaxSearchBytes: 3, MaxExpressionDepth: 1, MaxNodes: 1, MaxPathDepth: 2, MaxQuantifierDepth: 1, MaxCursorBytes: 3}
	_, err := ParseHTTP(url.Values{"filter": {"a eq 1 and b eq 2"}}, WithLimits(limits))
	queryErr, ok := err.(*Error)
	if !ok || queryErr.Code != CodeLimitExceeded || queryErr.Parameter != "filter" {
		t.Fatalf("ParseHTTP() error = %#v", err)
	}
	for _, values := range []url.Values{
		{"limit": {"6"}},
		{"offset": {"11"}},
		{"sort": {"name,age"}},
		{"search": {strings.Repeat("x", 4)}},
		{"sort": {"company/country/name"}},
		{"cursor": {"abcd"}},
	} {
		_, err = ParseHTTP(values, WithLimits(limits))
		queryErr, ok = err.(*Error)
		if !ok || queryErr.Code != CodeLimitExceeded {
			t.Fatalf("ParseHTTP(%v) error = %#v", values, err)
		}
	}
	_, err = ParseHTTP(nil, WithLimits(Limits{}))
	queryErr, ok = err.(*Error)
	if !ok || queryErr.Code != CodeInvalidSchema {
		t.Fatalf("ParseHTTP() config error = %#v", err)
	}
}

func TestParseHTTPResourceByteTokenAndLiteralLimits(t *testing.T) {
	t.Parallel()

	base := defaultQueryLimits
	tests := []struct {
		name      string
		values    url.Values
		configure func(*Limits)
		parameter string
	}{
		{name: "decoded query", values: url.Values{"unknown": {"12345"}}, configure: func(l *Limits) { l.MaxQueryBytes = 10 }, parameter: "query"},
		{name: "filter bytes", values: url.Values{"filter": {"age eq 18"}}, configure: func(l *Limits) { l.MaxFilterBytes = 8 }, parameter: "filter"},
		{name: "tokens", values: url.Values{"filter": {"age eq 18"}}, configure: func(l *Limits) { l.MaxTokens = 2 }, parameter: "filter"},
		{name: "literal bytes", values: url.Values{"filter": {"name eq 'abcdef'"}}, configure: func(l *Limits) { l.MaxLiteralBytes = 5 }, parameter: "filter"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limits := base
			tt.configure(&limits)
			_, err := ParseHTTP(tt.values, WithLimits(limits))
			queryErr, ok := err.(*Error)
			if !ok || queryErr.Code != CodeLimitExceeded || queryErr.Parameter != tt.parameter {
				t.Fatalf("ParseHTTP() error = %#v", err)
			}
		})
	}
}

func TestParseHTTPResourceLimitsAreInclusive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		values    url.Values
		configure func(*Limits, int)
		at        int
	}{
		{name: "decoded query bytes", values: url.Values{"filter": {"a eq 1"}}, at: len("filter") + len("a eq 1"), configure: func(l *Limits, n int) { l.MaxQueryBytes = n }},
		{name: "filter bytes", values: url.Values{"filter": {"a eq 1"}}, at: len("a eq 1"), configure: func(l *Limits, n int) { l.MaxFilterBytes = n }},
		{name: "tokens", values: url.Values{"filter": {"a eq 1"}}, at: 3, configure: func(l *Limits, n int) { l.MaxTokens = n }},
		{name: "literal bytes", values: url.Values{"filter": {"a eq 'x'"}}, at: len("'x'"), configure: func(l *Limits, n int) { l.MaxLiteralBytes = n }},
		{name: "in values", values: url.Values{"filter": {"a in (1,2)"}}, at: 2, configure: func(l *Limits, n int) { l.MaxInValues = n }},
		{name: "sort terms", values: url.Values{"sort": {"a,b"}}, at: 2, configure: func(l *Limits, n int) { l.MaxSortTerms = n }},
		{name: "search bytes", values: url.Values{"search": {"abc"}}, at: 3, configure: func(l *Limits, n int) { l.MaxSearchBytes = n }},
		{name: "limit", values: url.Values{"limit": {"5"}}, at: 5, configure: func(l *Limits, n int) { l.MaxLimit = n }},
		{name: "offset", values: url.Values{"offset": {"5"}}, at: 5, configure: func(l *Limits, n int) { l.MaxOffset = n }},
		{name: "cursor bytes", values: url.Values{"cursor": {"abc"}}, at: 3, configure: func(l *Limits, n int) { l.MaxCursorBytes = n }},
		{name: "nodes", values: url.Values{"filter": {"a eq 1 and b eq 2"}}, at: 3, configure: func(l *Limits, n int) { l.MaxNodes = n }},
		{name: "depth", values: url.Values{"filter": {"a eq 1 and b eq 2"}}, at: 2, configure: func(l *Limits, n int) { l.MaxExpressionDepth = n }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accepted := defaultQueryLimits
			tt.configure(&accepted, tt.at)
			if _, err := ParseHTTP(tt.values, WithLimits(accepted)); err != nil {
				t.Fatalf("limit boundary rejected: %v", err)
			}
			rejected := defaultQueryLimits
			tt.configure(&rejected, tt.at-1)
			_, err := ParseHTTP(tt.values, WithLimits(rejected))
			queryErr, ok := err.(*Error)
			if !ok || queryErr.Code != CodeLimitExceeded {
				t.Fatalf("limit+1 error = %#v", err)
			}
		})
	}
}
