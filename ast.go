package query

// Expr is a syntactic V1 filter expression. Its implementations are closed so
// validators and compilers can handle every possible node kind exhaustively.
type Expr interface {
	Span() Span
	expr()
}

// LogicalExpr joins two expressions with and/or.
type LogicalExpr struct {
	Operator LogicalOperator
	Left     Expr
	Right    Expr
	Source   Span
}

func (e *LogicalExpr) Span() Span { return e.Source }
func (e *LogicalExpr) expr()      {}

// NotExpr negates one filter expression.
type NotExpr struct {
	Expr   Expr
	Source Span
}

func (e *NotExpr) Span() Span { return e.Source }
func (e *NotExpr) expr()      {}

// QuantifierExpr applies any/all to an explicitly exposed to-many
// relationship. Predicate field paths are rooted at Variable.
type QuantifierExpr struct {
	Relationship       string
	Operator           QuantifierOperator
	Variable           string
	Predicate          Expr
	Source             Span
	RelationshipSource Span
	OperatorSource     Span
	VariableSource     Span
}

func (e *QuantifierExpr) Span() Span { return e.Source }
func (e *QuantifierExpr) expr()      {}

// ComparisonExpr compares one unresolved public field with a syntactic
// literal. Field resolution and literal conversion happen during validation.
type ComparisonExpr struct {
	Field       string
	Operator    ComparisonOperator
	Literal     Literal
	Literals    []Literal
	Source      Span
	FieldSource Span
	OpSource    Span
}

func (e *ComparisonExpr) Span() Span { return e.Source }
func (e *ComparisonExpr) expr()      {}

// LiteralKind classifies syntax before a model schema supplies a target kind.
type LiteralKind uint8

const (
	StringLiteral LiteralKind = iota
	NumberLiteral
	BoolLiteral
)

// Literal preserves raw syntax and its syntax-level value.
type Literal struct {
	Kind   LiteralKind
	Raw    string
	Value  any
	Source Span
}

// SortTerm is an unresolved public sort field.
type SortTerm struct {
	Field  string
	Desc   bool
	Source Span
}

// OrderTerm is retained as a source-compatible alias for the pre-release API.
type OrderTerm = SortTerm

// Query is the transport-level query returned by ParseHTTP.
type Query struct {
	Filter Expr
	Sort   []SortTerm
	Limit  *int
	Offset *int
	Cursor *string
	Count  *bool
	Search *string

	filterSource string
	sortSource   string
	limits       Limits
}

// Limits bounds client-controlled query complexity.
type Limits struct {
	MaxQueryBytes      int `json:"maxQueryBytes"`
	MaxFilterBytes     int `json:"maxFilterBytes"`
	MaxTokens          int `json:"maxTokens"`
	MaxLiteralBytes    int `json:"maxLiteralBytes"`
	MaxInValues        int `json:"maxInValues"`
	MaxLimit           int `json:"maxLimit"`
	MaxOffset          int `json:"maxOffset"`
	MaxSortTerms       int `json:"maxSortTerms"`
	MaxSearchBytes     int `json:"maxSearchBytes"`
	MaxExpressionDepth int `json:"maxExpressionDepth"`
	MaxNodes           int `json:"maxNodes"`
	MaxPathDepth       int `json:"maxPathDepth"`
	MaxQuantifierDepth int `json:"maxQuantifierDepth"`
	MaxCursorBytes     int `json:"maxCursorBytes"`
}

var defaultQueryLimits = Limits{
	MaxQueryBytes:      16 << 10,
	MaxFilterBytes:     8 << 10,
	MaxTokens:          512,
	MaxLiteralBytes:    4 << 10,
	MaxInValues:        100,
	MaxLimit:           100,
	MaxOffset:          100_000,
	MaxSortTerms:       5,
	MaxSearchBytes:     256,
	MaxExpressionDepth: 16,
	MaxNodes:           100,
	MaxPathDepth:       8,
	MaxQuantifierDepth: 4,
	MaxCursorBytes:     4 << 10,
}
