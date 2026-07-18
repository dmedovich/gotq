package query

import (
	"fmt"
	"strings"
)

const maxParenthesisNesting = 128

type parsedExpr struct {
	expr        Expr
	syntaxStart int
	syntaxEnd   int
	nodes       int
	depth       int
}

type filterParser struct {
	input      string
	tokens     []token
	current    int
	limits     Limits
	parenDepth int
	quantDepth int
	variables  []string
}

func parseFilter(input string, limits Limits) (Expr, *Error) {
	if limits.MaxFilterBytes <= 0 {
		limits.MaxFilterBytes = defaultQueryLimits.MaxFilterBytes
	}
	if limits.MaxTokens <= 0 {
		limits.MaxTokens = defaultQueryLimits.MaxTokens
	}
	if limits.MaxLiteralBytes <= 0 {
		limits.MaxLiteralBytes = defaultQueryLimits.MaxLiteralBytes
	}
	if limits.MaxInValues <= 0 {
		limits.MaxInValues = defaultQueryLimits.MaxInValues
	}
	if limits.MaxPathDepth <= 0 {
		limits.MaxPathDepth = defaultQueryLimits.MaxPathDepth
	}
	if limits.MaxQuantifierDepth <= 0 {
		limits.MaxQuantifierDepth = defaultQueryLimits.MaxQuantifierDepth
	}
	if len(input) > limits.MaxFilterBytes {
		return nil, positionedError(CodeLimitExceeded, "filter", input, limits.MaxFilterBytes, fmt.Sprintf("filter must not exceed %d bytes", limits.MaxFilterBytes))
	}
	tokens, err := lexFilterWithLimits(input, limits)
	if err != nil {
		return nil, err
	}
	p := filterParser{input: input, tokens: tokens, limits: limits}
	parsed, parseErr := p.parseOr()
	if parseErr != nil {
		return nil, parseErr
	}
	if tok := p.peek(); tok.kind != tokenEOF {
		return nil, p.syntax(tok.span.Start, fmt.Sprintf("unexpected %s after complete expression", tok.kind))
	}
	return parsed.expr, nil
}

func (p *filterParser) parseOr() (parsedExpr, *Error) {
	left, err := p.parseAnd()
	if err != nil {
		return parsedExpr{}, err
	}
	for p.peek().kind == tokenOr {
		op := p.advance()
		if !p.requiredSpacing(left.syntaxEnd, op.span.Start) {
			return parsedExpr{}, p.syntax(op.span.Start, "whitespace is required before or")
		}
		right, rightErr := p.parseAnd()
		if rightErr != nil {
			return parsedExpr{}, rightErr
		}
		if !p.requiredSpacing(op.span.End, right.syntaxStart) {
			return parsedExpr{}, p.syntax(right.syntaxStart, "whitespace is required after or")
		}
		left, err = p.join(Or, op, left, right)
		if err != nil {
			return parsedExpr{}, err
		}
	}
	return left, nil
}

func (p *filterParser) parseAnd() (parsedExpr, *Error) {
	left, err := p.parsePrimary()
	if err != nil {
		return parsedExpr{}, err
	}
	for p.peek().kind == tokenAnd {
		op := p.advance()
		if !p.requiredSpacing(left.syntaxEnd, op.span.Start) {
			return parsedExpr{}, p.syntax(op.span.Start, "whitespace is required before and")
		}
		right, rightErr := p.parsePrimary()
		if rightErr != nil {
			return parsedExpr{}, rightErr
		}
		if !p.requiredSpacing(op.span.End, right.syntaxStart) {
			return parsedExpr{}, p.syntax(right.syntaxStart, "whitespace is required after and")
		}
		left, err = p.join(And, op, left, right)
		if err != nil {
			return parsedExpr{}, err
		}
	}
	return left, nil
}

func (p *filterParser) parsePrimary() (parsedExpr, *Error) {
	if p.peek().kind == tokenNot {
		notToken := p.advance()
		if !p.requiredSpacing(notToken.span.End, p.peek().span.Start) {
			return parsedExpr{}, p.syntax(p.peek().span.Start, "whitespace is required after not")
		}
		inner, err := p.parsePrimary()
		if err != nil {
			return parsedExpr{}, err
		}
		nodes, depth := inner.nodes+1, inner.depth+1
		if nodes > p.limits.MaxNodes {
			return parsedExpr{}, p.limit(notToken.span.Start, fmt.Sprintf("expression contains more than %d nodes", p.limits.MaxNodes))
		}
		if depth > p.limits.MaxExpressionDepth {
			return parsedExpr{}, p.limit(notToken.span.Start, fmt.Sprintf("expression depth exceeds %d", p.limits.MaxExpressionDepth))
		}
		return parsedExpr{
			expr:        &NotExpr{Expr: inner.expr, Source: Span{Start: notToken.span.Start, End: inner.expr.Span().End}},
			syntaxStart: notToken.span.Start,
			syntaxEnd:   inner.syntaxEnd,
			nodes:       nodes,
			depth:       depth,
		}, nil
	}
	if p.peek().kind != tokenLeftParen {
		return p.parsePathPredicate()
	}

	open := p.advance()
	p.parenDepth++
	if p.parenDepth > maxParenthesisNesting {
		return parsedExpr{}, p.limit(open.span.Start, fmt.Sprintf("parenthesis nesting exceeds %d", maxParenthesisNesting))
	}
	inner, err := p.parseOr()
	p.parenDepth--
	if err != nil {
		return parsedExpr{}, err
	}
	if p.peek().kind != tokenRightParen {
		return parsedExpr{}, p.syntax(p.peek().span.Start, "expected closing parenthesis")
	}
	close := p.advance()
	inner.syntaxStart = open.span.Start
	inner.syntaxEnd = close.span.End
	return inner, nil
}

func (p *filterParser) parsePathPredicate() (parsedExpr, *Error) {
	first := p.peek()
	if first.kind != tokenIdentifier {
		return parsedExpr{}, p.syntax(first.span.Start, fmt.Sprintf("expected field or relationship identifier, got %s", first.kind))
	}
	segments := []token{p.advance()}
	for p.peek().kind == tokenSlash {
		slash := p.advance()
		if slash.span.Start != segments[len(segments)-1].span.End || p.peek().span.Start != slash.span.End {
			return parsedExpr{}, p.syntax(slash.span.Start, "whitespace is not allowed around '/' in paths")
		}
		next := p.peek()
		if next.kind == tokenAny || next.kind == tokenAll {
			p.advance()
			return p.parseQuantifier(segments, next)
		}
		if next.kind != tokenIdentifier {
			return parsedExpr{}, p.syntax(next.span.Start, fmt.Sprintf("expected path identifier, got %s", next.kind))
		}
		segments = append(segments, p.advance())
		if p.pathDepth(segments) > p.limits.MaxPathDepth {
			return parsedExpr{}, p.limit(next.span.Start, fmt.Sprintf("path depth exceeds %d", p.limits.MaxPathDepth))
		}
	}
	if p.pathDepth(segments) > p.limits.MaxPathDepth {
		return parsedExpr{}, p.limit(segments[len(segments)-1].span.Start, fmt.Sprintf("path depth exceeds %d", p.limits.MaxPathDepth))
	}
	values := make([]string, len(segments))
	for i, segment := range segments {
		values[i] = segment.value
	}
	fieldSpan := Span{Start: first.span.Start, End: segments[len(segments)-1].span.End}
	return p.parseComparison(strings.Join(values, "/"), fieldSpan)
}

func (p *filterParser) parseComparison(fieldName string, fieldSpan Span) (parsedExpr, *Error) {

	opStart := p.peek().span.Start
	if !p.requiredSpacing(fieldSpan.End, opStart) {
		return parsedExpr{}, p.syntax(opStart, "whitespace is required before comparison operator")
	}
	op, opEnd, err := p.parseComparisonOperator()
	if err != nil {
		return parsedExpr{}, err
	}
	expr := &ComparisonExpr{
		Field:       fieldName,
		Operator:    op,
		FieldSource: fieldSpan,
		OpSource:    Span{Start: opStart, End: opEnd},
	}
	syntaxEnd := opEnd
	switch op {
	case IsNull, IsNotNull:
		expr.Source = Span{Start: fieldSpan.Start, End: opEnd}
	case In, NotIn:
		if !p.requiredSpacing(opEnd, p.peek().span.Start) {
			return parsedExpr{}, p.syntax(p.peek().span.Start, "whitespace is required before in list")
		}
		if p.peek().kind != tokenLeftParen {
			return parsedExpr{}, p.syntax(p.peek().span.Start, "expected opening parenthesis for in list")
		}
		p.advance()
		if p.peek().kind == tokenRightParen {
			return parsedExpr{}, p.syntax(p.peek().span.Start, "in list must contain at least one literal")
		}
		for {
			literalToken := p.peek()
			literal, ok := literalFromToken(literalToken)
			if !ok {
				return parsedExpr{}, p.syntax(literalToken.span.Start, fmt.Sprintf("expected literal in list, got %s", literalToken.kind))
			}
			p.advance()
			expr.Literals = append(expr.Literals, literal)
			if len(expr.Literals) > p.limits.MaxInValues {
				return parsedExpr{}, p.limit(literalToken.span.Start, fmt.Sprintf("in list contains more than %d values", p.limits.MaxInValues))
			}
			if p.peek().kind == tokenRightParen {
				close := p.advance()
				syntaxEnd = close.span.End
				expr.Source = Span{Start: fieldSpan.Start, End: close.span.End}
				break
			}
			if p.peek().kind != tokenComma {
				return parsedExpr{}, p.syntax(p.peek().span.Start, "expected comma or closing parenthesis in list")
			}
			p.advance()
			if p.peek().kind == tokenRightParen {
				return parsedExpr{}, p.syntax(p.peek().span.Start, "expected literal after comma")
			}
		}
	default:
		literalToken := p.peek()
		literal, ok := literalFromToken(literalToken)
		if !ok {
			return parsedExpr{}, p.syntax(literalToken.span.Start, fmt.Sprintf("expected literal, got %s", literalToken.kind))
		}
		if !p.requiredSpacing(opEnd, literalToken.span.Start) {
			return parsedExpr{}, p.syntax(literalToken.span.Start, "whitespace is required before literal")
		}
		p.advance()
		expr.Literal = literal
		syntaxEnd = literalToken.span.End
		expr.Source = Span{Start: fieldSpan.Start, End: literalToken.span.End}
	}

	if p.limits.MaxNodes < 1 || p.limits.MaxExpressionDepth < 1 {
		return parsedExpr{}, p.limit(fieldSpan.Start, "comparison exceeds expression limits")
	}
	return parsedExpr{
		expr:        expr,
		syntaxStart: fieldSpan.Start,
		syntaxEnd:   syntaxEnd,
		nodes:       1,
		depth:       1,
	}, nil
}

func (p *filterParser) parseQuantifier(segments []token, operatorToken token) (parsedExpr, *Error) {
	if p.pathDepth(segments) > p.limits.MaxPathDepth {
		return parsedExpr{}, p.limit(segments[len(segments)-1].span.Start, fmt.Sprintf("path depth exceeds %d", p.limits.MaxPathDepth))
	}
	if p.peek().kind != tokenLeftParen || p.peek().span.Start != operatorToken.span.End {
		return parsedExpr{}, p.syntax(p.peek().span.Start, fmt.Sprintf("expected '(' immediately after %s", operatorToken.value))
	}
	open := p.advance()
	p.parenDepth++
	if p.parenDepth > maxParenthesisNesting {
		p.parenDepth--
		return parsedExpr{}, p.limit(open.span.Start, fmt.Sprintf("parenthesis nesting exceeds %d", maxParenthesisNesting))
	}
	p.quantDepth++
	if p.quantDepth > p.limits.MaxQuantifierDepth {
		p.quantDepth--
		p.parenDepth--
		return parsedExpr{}, p.limit(operatorToken.span.Start, fmt.Sprintf("quantifier depth exceeds %d", p.limits.MaxQuantifierDepth))
	}
	variable := p.peek()
	if variable.kind != tokenIdentifier {
		p.quantDepth--
		p.parenDepth--
		return parsedExpr{}, p.syntax(variable.span.Start, fmt.Sprintf("expected quantifier variable, got %s", variable.kind))
	}
	p.advance()
	if p.peek().kind != tokenColon {
		p.quantDepth--
		p.parenDepth--
		return parsedExpr{}, p.syntax(p.peek().span.Start, "expected ':' after quantifier variable")
	}
	colon := p.advance()
	if !p.requiredSpacing(colon.span.End, p.peek().span.Start) {
		p.quantDepth--
		p.parenDepth--
		return parsedExpr{}, p.syntax(p.peek().span.Start, "whitespace is required after ':'")
	}
	p.variables = append(p.variables, variable.value)
	predicate, err := p.parseOr()
	p.variables = p.variables[:len(p.variables)-1]
	p.quantDepth--
	p.parenDepth--
	if err != nil {
		return parsedExpr{}, err
	}
	if p.peek().kind != tokenRightParen {
		return parsedExpr{}, p.syntax(p.peek().span.Start, "expected closing parenthesis after quantifier predicate")
	}
	close := p.advance()
	nodes, depth := predicate.nodes+1, predicate.depth+1
	if nodes > p.limits.MaxNodes {
		return parsedExpr{}, p.limit(operatorToken.span.Start, fmt.Sprintf("expression contains more than %d nodes", p.limits.MaxNodes))
	}
	if depth > p.limits.MaxExpressionDepth {
		return parsedExpr{}, p.limit(operatorToken.span.Start, fmt.Sprintf("expression depth exceeds %d", p.limits.MaxExpressionDepth))
	}
	values := make([]string, len(segments))
	for i, segment := range segments {
		values[i] = segment.value
	}
	operator := Any
	if operatorToken.kind == tokenAll {
		operator = All
	}
	start := segments[0].span.Start
	return parsedExpr{
		expr: &QuantifierExpr{
			Relationship:       strings.Join(values, "/"),
			Operator:           operator,
			Variable:           variable.value,
			Predicate:          predicate.expr,
			Source:             Span{Start: start, End: close.span.End},
			RelationshipSource: Span{Start: start, End: segments[len(segments)-1].span.End},
			OperatorSource:     operatorToken.span,
			VariableSource:     variable.span,
		},
		syntaxStart: start,
		syntaxEnd:   close.span.End,
		nodes:       nodes,
		depth:       depth,
	}, nil
}

func (p *filterParser) pathDepth(segments []token) int {
	if len(p.variables) > 0 && len(segments) > 0 && segments[0].value == p.variables[len(p.variables)-1] {
		return len(segments) - 1
	}
	return len(segments)
}

func (p *filterParser) parseComparisonOperator() (ComparisonOperator, int, *Error) {
	first := p.peek()
	if op, ok := comparisonOperator(first.kind); ok {
		p.advance()
		return op, first.span.End, nil
	}
	switch first.kind {
	case tokenNot:
		p.advance()
		if p.peek().kind != tokenIn || !p.requiredSpacing(first.span.End, p.peek().span.Start) {
			return 0, 0, p.syntax(p.peek().span.Start, "expected in after not")
		}
		last := p.advance()
		return NotIn, last.span.End, nil
	case tokenIs:
		p.advance()
		if !p.requiredSpacing(first.span.End, p.peek().span.Start) {
			return 0, 0, p.syntax(p.peek().span.Start, "whitespace is required after is")
		}
		if p.peek().kind == tokenNull {
			last := p.advance()
			return IsNull, last.span.End, nil
		}
		if p.peek().kind != tokenNot {
			return 0, 0, p.syntax(p.peek().span.Start, "expected null or not null after is")
		}
		notToken := p.advance()
		if p.peek().kind != tokenNull || !p.requiredSpacing(notToken.span.End, p.peek().span.Start) {
			return 0, 0, p.syntax(p.peek().span.Start, "expected null after is not")
		}
		last := p.advance()
		return IsNotNull, last.span.End, nil
	default:
		return 0, 0, p.syntax(first.span.Start, fmt.Sprintf("expected comparison operator, got %s", first.kind))
	}
}

func (p *filterParser) join(op LogicalOperator, opToken token, left, right parsedExpr) (parsedExpr, *Error) {
	nodes := 1 + left.nodes + right.nodes
	depth := 1 + max(left.depth, right.depth)
	if nodes > p.limits.MaxNodes {
		return parsedExpr{}, p.limit(opToken.span.Start, fmt.Sprintf("expression contains more than %d nodes", p.limits.MaxNodes))
	}
	if depth > p.limits.MaxExpressionDepth {
		return parsedExpr{}, p.limit(opToken.span.Start, fmt.Sprintf("expression depth exceeds %d", p.limits.MaxExpressionDepth))
	}
	expr := &LogicalExpr{
		Operator: op,
		Left:     left.expr,
		Right:    right.expr,
		Source:   Span{Start: left.expr.Span().Start, End: right.expr.Span().End},
	}
	return parsedExpr{
		expr:        expr,
		syntaxStart: left.syntaxStart,
		syntaxEnd:   right.syntaxEnd,
		nodes:       nodes,
		depth:       depth,
	}, nil
}

func (p *filterParser) requiredSpacing(start, end int) bool {
	if start >= end {
		return false
	}
	for i := start; i < end; i++ {
		if !isWhitespace(p.input[i]) {
			return false
		}
	}
	return true
}

func (p *filterParser) peek() token {
	return p.tokens[p.current]
}

func (p *filterParser) advance() token {
	tok := p.tokens[p.current]
	if tok.kind != tokenEOF {
		p.current++
	}
	return tok
}

func (p *filterParser) syntax(offset int, message string) *Error {
	return positionedError(CodeInvalidSyntax, "filter", p.input, offset, message)
}

func (p *filterParser) limit(offset int, message string) *Error {
	return positionedError(CodeLimitExceeded, "filter", p.input, offset, message)
}

func comparisonOperator(kind tokenKind) (ComparisonOperator, bool) {
	switch kind {
	case tokenEq:
		return Eq, true
	case tokenNe:
		return Ne, true
	case tokenGt:
		return Gt, true
	case tokenGte:
		return Gte, true
	case tokenLt:
		return Lt, true
	case tokenLte:
		return Lte, true
	case tokenContains:
		return Contains, true
	case tokenStartsWith:
		return StartsWith, true
	case tokenEndsWith:
		return EndsWith, true
	case tokenIn:
		return In, true
	default:
		return 0, false
	}
}

func literalFromToken(tok token) (Literal, bool) {
	switch tok.kind {
	case tokenString:
		return Literal{Kind: StringLiteral, Raw: tok.lexeme, Value: tok.value, Source: tok.span}, true
	case tokenNumber:
		return Literal{Kind: NumberLiteral, Raw: tok.lexeme, Value: tok.value, Source: tok.span}, true
	case tokenTrue:
		return Literal{Kind: BoolLiteral, Raw: tok.lexeme, Value: true, Source: tok.span}, true
	case tokenFalse:
		return Literal{Kind: BoolLiteral, Raw: tok.lexeme, Value: false, Source: tok.span}, true
	default:
		return Literal{}, false
	}
}
