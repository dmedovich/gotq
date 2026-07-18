package query

import "fmt"

// DateValue marks a timezone-free ISO calendar date (YYYY-MM-DD) for schema
// inference. It is bound to the database as its canonical string form.
type DateValue string

// UUIDValue marks a canonical RFC 4122 UUID string for schema inference.
type UUIDValue string

// DecimalValue marks an exact decimal represented without binary floating
// point conversion.
type DecimalValue string

// Kind describes a model field's scalar semantics, independently of its
// database type.
type Kind uint8

const (
	String Kind = iota
	Bool
	Int
	Uint
	Float
	Time
	Date
	UUID
	Decimal
	Custom
)

func (k Kind) String() string {
	switch k {
	case String:
		return "string"
	case Bool:
		return "bool"
	case Int:
		return "int"
	case Uint:
		return "uint"
	case Float:
		return "float"
	case Time:
		return "time"
	case Date:
		return "date"
	case UUID:
		return "uuid"
	case Decimal:
		return "decimal"
	case Custom:
		return "custom"
	default:
		return fmt.Sprintf("Kind(%d)", k)
	}
}

func (k Kind) MarshalText() ([]byte, error) { return []byte(k.String()), nil }

// ComparisonOperator is a closed set of V1 field comparison operators.
type ComparisonOperator uint8

const (
	Eq ComparisonOperator = iota
	Ne
	Gt
	Gte
	Lt
	Lte
	Contains
	StartsWith
	EndsWith
	In
	NotIn
	IsNull
	IsNotNull
)

func (op ComparisonOperator) String() string {
	switch op {
	case Eq:
		return "eq"
	case Ne:
		return "ne"
	case Gt:
		return "gt"
	case Gte:
		return "gte"
	case Lt:
		return "lt"
	case Lte:
		return "lte"
	case Contains:
		return "contains"
	case StartsWith:
		return "startswith"
	case EndsWith:
		return "endswith"
	case In:
		return "in"
	case NotIn:
		return "not in"
	case IsNull:
		return "is null"
	case IsNotNull:
		return "is not null"
	default:
		return fmt.Sprintf("ComparisonOperator(%d)", op)
	}
}

func (op ComparisonOperator) MarshalText() ([]byte, error) {
	return []byte(op.String()), nil
}

// LogicalOperator is a closed set of V1 boolean operators.
type LogicalOperator uint8

const (
	And LogicalOperator = iota
	Or
)

func (op LogicalOperator) String() string {
	switch op {
	case And:
		return "and"
	case Or:
		return "or"
	default:
		return fmt.Sprintf("LogicalOperator(%d)", op)
	}
}

func (op LogicalOperator) MarshalText() ([]byte, error) { return []byte(op.String()), nil }

// QuantifierOperator is a closed set of collection relationship predicates.
type QuantifierOperator uint8

const (
	Any QuantifierOperator = iota
	All
)

func (op QuantifierOperator) String() string {
	switch op {
	case Any:
		return "any"
	case All:
		return "all"
	default:
		return fmt.Sprintf("QuantifierOperator(%d)", op)
	}
}

func (op QuantifierOperator) MarshalText() ([]byte, error) { return []byte(op.String()), nil }

// Span is a half-open byte range in a decoded query parameter value.
type Span struct {
	Start int
	End   int
}

// ErrorCode is stable and intended for machine-readable client errors.
type ErrorCode string

const (
	CodeInvalidParameter    ErrorCode = "invalid_parameter"
	CodeInvalidToken        ErrorCode = "invalid_token"
	CodeInvalidSyntax       ErrorCode = "invalid_syntax"
	CodeLimitExceeded       ErrorCode = "limit_exceeded"
	CodeUnknownField        ErrorCode = "unknown_field"
	CodeNotFilterable       ErrorCode = "field_not_filterable"
	CodeNotSortable         ErrorCode = "field_not_sortable"
	CodeOperatorNotAllowed  ErrorCode = "operator_not_allowed"
	CodeInvalidLiteral      ErrorCode = "invalid_literal"
	CodeInvalidSchema       ErrorCode = "invalid_schema"
	CodeExecutionFailed     ErrorCode = "execution_failed"
	CodeInvalidRelationship ErrorCode = "invalid_relationship"
	CodeInvalidCursor       ErrorCode = "invalid_cursor"
)

// Position identifies a byte in a decoded parameter value.
type Position struct {
	Offset int `json:"offset"`
	Line   int `json:"line"`
	Column int `json:"column"`
}

// Error is returned for invalid client queries and invalid model schemas.
// Message is human-readable; Code and the typed fields are stable.
type Error struct {
	Code             ErrorCode            `json:"code"`
	Parameter        string               `json:"parameter,omitempty"`
	Position         *Position            `json:"position,omitempty"`
	Message          string               `json:"message"`
	Field            string               `json:"field,omitempty"`
	Kind             *Kind                `json:"kind,omitempty"`
	Operator         *ComparisonOperator  `json:"operator,omitempty"`
	AllowedOperators []ComparisonOperator `json:"allowedOperators,omitempty"`
	Cause            error                `json:"-"`
}

// Unwrap exposes an internal execution cause to errors.Is/errors.As without
// serializing it to clients.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	return e.Message
}

func positionedError(code ErrorCode, parameter, input string, offset int, message string) *Error {
	position := positionAt(input, offset)
	return &Error{
		Code:      code,
		Parameter: parameter,
		Position:  &position,
		Message:   message,
	}
}

func positionAt(input string, offset int) Position {
	if offset < 0 {
		offset = 0
	}
	if offset > len(input) {
		offset = len(input)
	}

	line, column := 1, 1
	for i := 0; i < offset; i++ {
		if input[i] == '\n' {
			line++
			column = 1
		} else {
			column++
		}
	}
	return Position{Offset: offset, Line: line, Column: column}
}
