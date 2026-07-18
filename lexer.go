package query

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type tokenKind uint8

const (
	tokenEOF tokenKind = iota
	tokenIdentifier
	tokenString
	tokenNumber
	tokenTrue
	tokenFalse
	tokenEq
	tokenNe
	tokenGt
	tokenGte
	tokenLt
	tokenLte
	tokenContains
	tokenStartsWith
	tokenEndsWith
	tokenIn
	tokenIs
	tokenNull
	tokenNot
	tokenAnd
	tokenOr
	tokenLeftParen
	tokenRightParen
	tokenComma
	tokenSlash
	tokenColon
	tokenAny
	tokenAll
)

func (k tokenKind) String() string {
	switch k {
	case tokenEOF:
		return "EOF"
	case tokenIdentifier:
		return "identifier"
	case tokenString:
		return "string"
	case tokenNumber:
		return "number"
	case tokenTrue:
		return "true"
	case tokenFalse:
		return "false"
	case tokenEq:
		return "eq"
	case tokenNe:
		return "ne"
	case tokenGt:
		return "gt"
	case tokenGte:
		return "gte"
	case tokenLt:
		return "lt"
	case tokenLte:
		return "lte"
	case tokenContains:
		return "contains"
	case tokenStartsWith:
		return "startswith"
	case tokenEndsWith:
		return "endswith"
	case tokenIn:
		return "in"
	case tokenIs:
		return "is"
	case tokenNull:
		return "null"
	case tokenNot:
		return "not"
	case tokenAnd:
		return "and"
	case tokenOr:
		return "or"
	case tokenLeftParen:
		return "("
	case tokenRightParen:
		return ")"
	case tokenComma:
		return ","
	case tokenSlash:
		return "/"
	case tokenColon:
		return ":"
	case tokenAny:
		return "any"
	case tokenAll:
		return "all"
	default:
		return fmt.Sprintf("tokenKind(%d)", k)
	}
}

type token struct {
	kind   tokenKind
	lexeme string
	value  string
	span   Span
}

type lexer struct {
	input           string
	pos             int
	maxLiteralBytes int
}

func lexFilter(input string) ([]token, *Error) {
	return lexFilterWithLimits(input, defaultQueryLimits)
}

func lexFilterWithLimits(input string, limits Limits) ([]token, *Error) {
	l := lexer{input: input, maxLiteralBytes: limits.MaxLiteralBytes}
	tokens := make([]token, 0, 16)
	for {
		tok, err := l.next()
		if err != nil {
			return nil, err
		}
		if tok.kind != tokenEOF && len(tokens) >= limits.MaxTokens {
			return nil, positionedError(CodeLimitExceeded, "filter", input, tok.span.Start, fmt.Sprintf("filter must not contain more than %d tokens", limits.MaxTokens))
		}
		if tok.kind == tokenString || tok.kind == tokenNumber {
			if len(tok.lexeme) > limits.MaxLiteralBytes {
				return nil, positionedError(CodeLimitExceeded, "filter", input, tok.span.Start, fmt.Sprintf("literal must not exceed %d bytes", limits.MaxLiteralBytes))
			}
		}
		tokens = append(tokens, tok)
		if tok.kind == tokenEOF {
			return tokens, nil
		}
	}
}

func (l *lexer) next() (token, *Error) {
	l.skipWhitespace()
	if l.pos == len(l.input) {
		return token{kind: tokenEOF, span: Span{Start: l.pos, End: l.pos}}, nil
	}

	start := l.pos
	c := l.input[l.pos]
	switch {
	case isIdentifierStart(c):
		return l.scanIdentifier(), nil
	case c == '\'':
		return l.scanString()
	case c == '-' || isDigit(c):
		return l.scanNumber()
	case c == '(':
		l.pos++
		return l.simpleToken(tokenLeftParen, start), nil
	case c == ')':
		l.pos++
		return l.simpleToken(tokenRightParen, start), nil
	case c == ',':
		l.pos++
		return l.simpleToken(tokenComma, start), nil
	case c == '/':
		l.pos++
		return l.simpleToken(tokenSlash, start), nil
	case c == ':':
		l.pos++
		return l.simpleToken(tokenColon, start), nil
	case c >= utf8.RuneSelf:
		_, size := utf8.DecodeRuneInString(l.input[l.pos:])
		if size == 1 {
			return token{}, l.invalid(l.pos, "invalid UTF-8 encoding")
		}
		return token{}, l.invalid(l.pos, "identifiers must contain only ASCII letters, digits, and underscore")
	default:
		return token{}, l.invalid(l.pos, fmt.Sprintf("unexpected character %q", c))
	}
}

func (l *lexer) skipWhitespace() {
	for l.pos < len(l.input) && isWhitespace(l.input[l.pos]) {
		l.pos++
	}
}

func (l *lexer) scanIdentifier() token {
	start := l.pos
	l.pos++
	for l.pos < len(l.input) && isIdentifierContinue(l.input[l.pos]) {
		l.pos++
	}
	lexeme := l.input[start:l.pos]
	kind := tokenIdentifier
	if keyword, ok := keywordTokens[lexeme]; ok {
		kind = keyword
	}
	return token{kind: kind, lexeme: lexeme, value: lexeme, span: Span{Start: start, End: l.pos}}
}

func (l *lexer) scanString() (token, *Error) {
	start := l.pos
	l.pos++
	var value strings.Builder
	for l.pos < len(l.input) {
		c := l.input[l.pos]
		if c == '\'' {
			if l.pos+1 < len(l.input) && l.input[l.pos+1] == '\'' {
				value.WriteByte('\'')
				l.pos += 2
				continue
			}
			l.pos++
			return token{
				kind:   tokenString,
				lexeme: l.input[start:l.pos],
				value:  value.String(),
				span:   Span{Start: start, End: l.pos},
			}, nil
		}
		if c <= 0x1f || c == 0x7f {
			return token{}, l.invalid(l.pos, "control characters are not allowed in string literals")
		}
		if c < utf8.RuneSelf {
			value.WriteByte(c)
			l.pos++
			continue
		}
		_, size := utf8.DecodeRuneInString(l.input[l.pos:])
		if size == 1 {
			return token{}, l.invalid(l.pos, "invalid UTF-8 encoding")
		}
		value.WriteString(l.input[l.pos : l.pos+size])
		l.pos += size
	}
	return token{}, l.invalid(len(l.input), "unterminated string literal")
}

func (l *lexer) scanNumber() (token, *Error) {
	start := l.pos
	if l.input[l.pos] == '-' {
		l.pos++
		if l.pos == len(l.input) {
			return token{}, l.invalid(l.pos, "minus must be followed by a digit")
		}
		if !isDigit(l.input[l.pos]) {
			return token{}, l.invalid(l.pos, "minus must be followed by a digit")
		}
	}

	if l.input[l.pos] == '0' {
		l.pos++
		if l.pos < len(l.input) && isDigit(l.input[l.pos]) {
			return token{}, l.invalid(l.pos, "leading zeroes are not allowed")
		}
	} else {
		for l.pos < len(l.input) && isDigit(l.input[l.pos]) {
			l.pos++
		}
	}

	if l.pos < len(l.input) && l.input[l.pos] == '.' {
		l.pos++
		if l.pos == len(l.input) {
			return token{}, l.invalid(l.pos, "fraction requires at least one digit")
		}
		if !isDigit(l.input[l.pos]) {
			return token{}, l.invalid(l.pos, "fraction requires at least one digit")
		}
		for l.pos < len(l.input) && isDigit(l.input[l.pos]) {
			l.pos++
		}
	}

	if l.pos < len(l.input) && (l.input[l.pos] == 'e' || l.input[l.pos] == 'E') {
		l.pos++
		if l.pos < len(l.input) && (l.input[l.pos] == '+' || l.input[l.pos] == '-') {
			l.pos++
		}
		if l.pos == len(l.input) {
			return token{}, l.invalid(l.pos, "exponent requires at least one digit")
		}
		if !isDigit(l.input[l.pos]) {
			return token{}, l.invalid(l.pos, "exponent requires at least one digit")
		}
		for l.pos < len(l.input) && isDigit(l.input[l.pos]) {
			l.pos++
		}
	}

	if l.pos < len(l.input) && l.input[l.pos] == '.' {
		return token{}, l.invalid(l.pos, "number contains more than one decimal point")
	}

	return token{
		kind:   tokenNumber,
		lexeme: l.input[start:l.pos],
		value:  l.input[start:l.pos],
		span:   Span{Start: start, End: l.pos},
	}, nil
}

func (l *lexer) simpleToken(kind tokenKind, start int) token {
	return token{kind: kind, lexeme: l.input[start:l.pos], value: l.input[start:l.pos], span: Span{Start: start, End: l.pos}}
}

func (l *lexer) invalid(offset int, message string) *Error {
	return positionedError(CodeInvalidToken, "filter", l.input, offset, message)
}

var keywordTokens = map[string]tokenKind{
	"true":       tokenTrue,
	"false":      tokenFalse,
	"eq":         tokenEq,
	"ne":         tokenNe,
	"gt":         tokenGt,
	"gte":        tokenGte,
	"lt":         tokenLt,
	"lte":        tokenLte,
	"contains":   tokenContains,
	"startswith": tokenStartsWith,
	"endswith":   tokenEndsWith,
	"in":         tokenIn,
	"is":         tokenIs,
	"null":       tokenNull,
	"not":        tokenNot,
	"and":        tokenAnd,
	"or":         tokenOr,
	"any":        tokenAny,
	"all":        tokenAll,
}

func isWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}

func isIdentifierStart(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isIdentifierContinue(c byte) bool {
	return isIdentifierStart(c) || isDigit(c)
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}
