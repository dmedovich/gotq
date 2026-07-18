package query

import (
	"testing"
)

func TestLexerTokenKindsValuesAndSpans(t *testing.T) {
	t.Parallel()

	input := "age gt 18 and name contains 'O''Brien'"
	tokens, err := lexFilter(input)
	if err != nil {
		t.Fatalf("lexFilter() error = %v", err)
	}

	want := []token{
		{kind: tokenIdentifier, lexeme: "age", value: "age", span: Span{0, 3}},
		{kind: tokenGt, lexeme: "gt", value: "gt", span: Span{4, 6}},
		{kind: tokenNumber, lexeme: "18", value: "18", span: Span{7, 9}},
		{kind: tokenAnd, lexeme: "and", value: "and", span: Span{10, 13}},
		{kind: tokenIdentifier, lexeme: "name", value: "name", span: Span{14, 18}},
		{kind: tokenContains, lexeme: "contains", value: "contains", span: Span{19, 27}},
		{kind: tokenString, lexeme: "'O''Brien'", value: "O'Brien", span: Span{28, 38}},
		{kind: tokenEOF, span: Span{38, 38}},
	}

	if len(tokens) != len(want) {
		t.Fatalf("len(tokens) = %d, want %d: %#v", len(tokens), len(want), tokens)
	}
	for i := range want {
		if tokens[i] != want[i] {
			t.Errorf("token[%d] = %#v, want %#v", i, tokens[i], want[i])
		}
	}
}

func TestLexerKeywordsAreExact(t *testing.T) {
	t.Parallel()

	tokens, err := lexFilter("android or Contains eq true")
	if err != nil {
		t.Fatalf("lexFilter() error = %v", err)
	}
	want := []tokenKind{tokenIdentifier, tokenOr, tokenIdentifier, tokenEq, tokenTrue, tokenEOF}
	for i, kind := range want {
		if tokens[i].kind != kind {
			t.Errorf("token[%d].kind = %s, want %s", i, tokens[i].kind, kind)
		}
	}
}

func TestLexerStringUnicodeAndEscapedApostrophe(t *testing.T) {
	t.Parallel()

	input := "city eq 'Волгоград'"
	tokens, err := lexFilter(input)
	if err != nil {
		t.Fatalf("lexFilter() error = %v", err)
	}
	if got, want := tokens[2].value, "Волгоград"; got != want {
		t.Errorf("string value = %q, want %q", got, want)
	}
	if got, want := tokens[2].span, (Span{Start: 8, End: len(input)}); got != want {
		t.Errorf("string span = %#v, want %#v", got, want)
	}
}

func TestLexerValidNumbers(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"0", "-0", "42", "-17", "0.5", "-12.75", "1e3", "6.02E+23", "1e-9"} {
		input := input
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			tokens, err := lexFilter(input)
			if err != nil {
				t.Fatalf("lexFilter(%q) error = %v", input, err)
			}
			if len(tokens) != 2 || tokens[0].kind != tokenNumber || tokens[0].lexeme != input || tokens[1].kind != tokenEOF {
				t.Fatalf("lexFilter(%q) = %#v", input, tokens)
			}
		})
	}
}

func TestLexerMalformedInputPositions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		offset int
	}{
		{name: "raw operator", input: "age > 18", offset: 4},
		{name: "leading plus", input: "age gt +1", offset: 7},
		{name: "bare minus", input: "age eq -", offset: 8},
		{name: "minus before non-digit", input: "age eq -x", offset: 8},
		{name: "leading zero", input: "age eq 01", offset: 8},
		{name: "fraction EOF", input: "score eq 1.", offset: 11},
		{name: "exponent EOF", input: "score eq 1e", offset: 11},
		{name: "relation path", input: "user.name eq 'ann'", offset: 4},
		{name: "unterminated string", input: "name eq 'ann", offset: 12},
		{name: "control in string", input: "name eq 'a\nb'", offset: 10},
		{name: "invalid UTF-8", input: "name eq 'a\xffb'", offset: 10},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := lexFilter(tt.input)
			if err == nil {
				t.Fatalf("lexFilter(%q) returned nil error", tt.input)
			}
			if err.Code != CodeInvalidToken {
				t.Errorf("error code = %q, want %q", err.Code, CodeInvalidToken)
			}
			if err.Position == nil || err.Position.Offset != tt.offset {
				t.Errorf("error position = %#v, want offset %d", err.Position, tt.offset)
			}
		})
	}
}

func TestPositionAtTracksByteColumnsAndLines(t *testing.T) {
	t.Parallel()

	input := "a\nВx"
	if got, want := positionAt(input, len(input)), (Position{Offset: 5, Line: 2, Column: 4}); got != want {
		t.Fatalf("positionAt() = %#v, want %#v", got, want)
	}
}
