package query

import (
	"encoding/hex"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type validatedQuery struct {
	filter    validatedExpr
	sort      []validatedOrder
	limit     *int
	offset    *int
	cursor    *validatedCursor
	rawCursor *string
	count     *bool
	search    *string
}

type validatedExpr interface {
	validatedExpr()
}

type validatedLogical struct {
	operator LogicalOperator
	left     validatedExpr
	right    validatedExpr
}

func (*validatedLogical) validatedExpr() {}

type validatedNot struct {
	expr validatedExpr
}

func (*validatedNot) validatedExpr() {}

type validatedComparison struct {
	field    *modelField
	table    string
	operator ComparisonOperator
	value    any
	values   []any
}

func (*validatedComparison) validatedExpr() {}

type relationshipPredicate uint8

const (
	relationshipExists relationshipPredicate = iota
	relationshipAny
	relationshipAll
)

type validatedRelationship struct {
	relationship *modelRelationship
	parentTable  string
	alias        string
	predicate    validatedExpr
	mode         relationshipPredicate
}

func (*validatedRelationship) validatedExpr() {}

type validatedOrder struct {
	field    *modelField
	desc     bool
	joinPath string
	table    string
}

type expressionValidation struct {
	source    string
	limits    Limits
	nextAlias int
	variables map[string]struct{}
	reserved  map[string]struct{}
}

type expressionScope struct {
	schema        *modelSchema
	table         string
	variable      string
	qualifyFields bool
}

func validateQuery[T any](schema *ModelSchema[T], q Query) (*validatedQuery, *Error) {
	if schema == nil || schema.modelSchema == nil {
		return nil, schemaError("model schema is nil", "")
	}
	limits := q.limits
	if limits == (Limits{}) {
		limits = defaultQueryLimits
	}
	if q.Limit != nil {
		if *q.Limit < 0 {
			return nil, queryValidationError(CodeInvalidParameter, "limit", "", 0, "limit must be non-negative", "")
		}
		if *q.Limit > limits.MaxLimit {
			return nil, queryValidationError(CodeLimitExceeded, "limit", "", 0, fmt.Sprintf("limit must not exceed %d", limits.MaxLimit), "")
		}
	}
	if q.Offset != nil {
		if *q.Offset < 0 {
			return nil, queryValidationError(CodeInvalidParameter, "offset", "", 0, "offset must be non-negative", "")
		}
		if *q.Offset > limits.MaxOffset {
			return nil, queryValidationError(CodeLimitExceeded, "offset", "", 0, fmt.Sprintf("offset must not exceed %d", limits.MaxOffset), "")
		}
	}
	if q.Cursor != nil {
		if *q.Cursor == "" {
			return nil, queryValidationError(CodeInvalidParameter, "cursor", "", 0, "cursor must not be empty", "")
		}
		if q.Offset != nil {
			return nil, queryValidationError(CodeInvalidParameter, "cursor", *q.Cursor, 0, "cursor and offset cannot be used together", "")
		}
		if len(*q.Cursor) > limits.MaxCursorBytes {
			return nil, queryValidationError(CodeLimitExceeded, "cursor", *q.Cursor, limits.MaxCursorBytes, fmt.Sprintf("cursor must not exceed %d bytes", limits.MaxCursorBytes), "")
		}
		for index := range len(*q.Cursor) {
			if !isBase64URLCharacter((*q.Cursor)[index]) {
				return nil, queryValidationError(CodeInvalidCursor, "cursor", *q.Cursor, index, "cursor must use unpadded base64url characters", "")
			}
		}
	}
	if len(q.Sort) > limits.MaxSortTerms {
		return nil, queryValidationError(CodeLimitExceeded, "sort", q.sortSource, 0, fmt.Sprintf("sort must not contain more than %d terms", limits.MaxSortTerms), "")
	}
	if q.Search != nil && len(*q.Search) > limits.MaxSearchBytes {
		return nil, queryValidationError(CodeLimitExceeded, "search", *q.Search, 0, fmt.Sprintf("search must not exceed %d bytes", limits.MaxSearchBytes), "")
	}
	validated := &validatedQuery{limit: q.Limit, offset: q.Offset, rawCursor: q.Cursor, count: q.Count, search: q.Search}
	if q.Filter != nil {
		if err := inspectAST(q.Filter, q.filterSource, limits); err != nil {
			return nil, err
		}
		state := &expressionValidation{source: q.filterSource, limits: limits, variables: make(map[string]struct{}), reserved: make(map[string]struct{})}
		collectStorageNames(schema.modelSchema, state.reserved)
		filter, err := state.validateExpr(expressionScope{schema: schema.modelSchema, table: schema.table}, q.Filter)
		if err != nil {
			return nil, err
		}
		validated.filter = filter
	}
	for _, term := range q.Sort {
		order, err := validateSortPath(schema.modelSchema, term, q.sortSource, limits)
		if err != nil {
			return nil, err
		}
		validated.sort = append(validated.sort, order)
	}
	return validated, nil
}

func inspectAST(root Expr, source string, limits Limits) *Error {
	type frame struct {
		expr       Expr
		depth      int
		quantDepth int
	}
	stack := []frame{{expr: root, depth: 1}}
	seen := make(map[Expr]struct{})
	nodes := 0

	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current.expr == nil {
			return queryValidationError(CodeInvalidSyntax, "filter", source, 0, "filter expression contains a nil node", "")
		}
		if _, duplicate := seen[current.expr]; duplicate {
			return queryValidationError(CodeInvalidSyntax, "filter", source, astOffset(current.expr), "filter expression contains a cycle or reused node", "")
		}
		seen[current.expr] = struct{}{}
		nodes++
		if nodes > limits.MaxNodes {
			return queryValidationError(CodeLimitExceeded, "filter", source, astOffset(current.expr), fmt.Sprintf("expression contains more than %d nodes", limits.MaxNodes), "")
		}
		if current.depth > limits.MaxExpressionDepth {
			return queryValidationError(CodeLimitExceeded, "filter", source, astOffset(current.expr), fmt.Sprintf("expression depth exceeds %d", limits.MaxExpressionDepth), "")
		}
		if current.quantDepth > limits.MaxQuantifierDepth {
			return queryValidationError(CodeLimitExceeded, "filter", source, astOffset(current.expr), fmt.Sprintf("quantifier depth exceeds %d", limits.MaxQuantifierDepth), "")
		}

		switch expr := current.expr.(type) {
		case *ComparisonExpr:
			if expr == nil {
				return queryValidationError(CodeInvalidSyntax, "filter", source, 0, "comparison expression is nil", "")
			}
			if len(expr.Literal.Raw) > limits.MaxLiteralBytes {
				return queryValidationError(CodeLimitExceeded, "filter", source, expr.Literal.Source.Start, fmt.Sprintf("literal must not exceed %d bytes", limits.MaxLiteralBytes), expr.Field)
			}
			if len(expr.Literals) > limits.MaxInValues {
				return queryValidationError(CodeLimitExceeded, "filter", source, expr.Source.Start, fmt.Sprintf("in list contains more than %d values", limits.MaxInValues), expr.Field)
			}
			for _, literal := range expr.Literals {
				if len(literal.Raw) > limits.MaxLiteralBytes {
					return queryValidationError(CodeLimitExceeded, "filter", source, literal.Source.Start, fmt.Sprintf("literal must not exceed %d bytes", limits.MaxLiteralBytes), expr.Field)
				}
			}
			if pathDepth(expr.Field, false) > limits.MaxPathDepth+1 {
				return queryValidationError(CodeLimitExceeded, "filter", source, expr.FieldSource.Start, fmt.Sprintf("path depth exceeds %d", limits.MaxPathDepth), expr.Field)
			}
		case *QuantifierExpr:
			if expr == nil {
				return queryValidationError(CodeInvalidSyntax, "filter", source, 0, "quantifier expression is nil", "")
			}
			if expr.Operator != Any && expr.Operator != All {
				return queryValidationError(CodeInvalidSyntax, "filter", source, expr.OperatorSource.Start, fmt.Sprintf("unsupported quantifier operator %d", expr.Operator), expr.Relationship)
			}
			if !isValidIdentifier(expr.Variable) || isReservedName(expr.Variable) {
				return queryValidationError(CodeInvalidSyntax, "filter", source, expr.VariableSource.Start, fmt.Sprintf("invalid quantifier variable %q", expr.Variable), expr.Relationship)
			}
			if pathDepth(expr.Relationship, false) > limits.MaxPathDepth+1 {
				return queryValidationError(CodeLimitExceeded, "filter", source, expr.RelationshipSource.Start, fmt.Sprintf("path depth exceeds %d", limits.MaxPathDepth), expr.Relationship)
			}
			stack = append(stack, frame{expr: expr.Predicate, depth: current.depth + 1, quantDepth: current.quantDepth + 1})
		case *NotExpr:
			if expr == nil {
				return queryValidationError(CodeInvalidSyntax, "filter", source, 0, "not expression is nil", "")
			}
			stack = append(stack, frame{expr: expr.Expr, depth: current.depth + 1, quantDepth: current.quantDepth})
		case *LogicalExpr:
			if expr == nil {
				return queryValidationError(CodeInvalidSyntax, "filter", source, 0, "logical expression is nil", "")
			}
			stack = append(stack,
				frame{expr: expr.Right, depth: current.depth + 1, quantDepth: current.quantDepth},
				frame{expr: expr.Left, depth: current.depth + 1, quantDepth: current.quantDepth},
			)
		default:
			return queryValidationError(CodeInvalidSyntax, "filter", source, astOffset(current.expr), fmt.Sprintf("unsupported expression type %T", current.expr), "")
		}
	}
	return nil
}

func astOffset(expr Expr) int {
	switch expr := expr.(type) {
	case *ComparisonExpr:
		if expr != nil {
			return expr.Source.Start
		}
	case *LogicalExpr:
		if expr != nil {
			return expr.Source.Start
		}
	case *NotExpr:
		if expr != nil {
			return expr.Source.Start
		}
	case *QuantifierExpr:
		if expr != nil {
			return expr.Source.Start
		}
	}
	return 0
}

func (v *expressionValidation) validateExpr(scope expressionScope, expression Expr) (validatedExpr, *Error) {
	if expression == nil {
		return nil, queryValidationError(CodeInvalidSyntax, "filter", v.source, 0, "filter expression is nil", "")
	}
	switch expr := expression.(type) {
	case *LogicalExpr:
		if expr == nil {
			return nil, queryValidationError(CodeInvalidSyntax, "filter", v.source, 0, "logical expression is nil", "")
		}
		if expr.Operator != And && expr.Operator != Or {
			return nil, queryValidationError(CodeInvalidSyntax, "filter", v.source, expr.Source.Start, fmt.Sprintf("unsupported logical operator %d", expr.Operator), "")
		}
		left, err := v.validateExpr(scope, expr.Left)
		if err != nil {
			return nil, err
		}
		right, err := v.validateExpr(scope, expr.Right)
		if err != nil {
			return nil, err
		}
		return &validatedLogical{operator: expr.Operator, left: left, right: right}, nil
	case *NotExpr:
		if expr == nil {
			return nil, queryValidationError(CodeInvalidSyntax, "filter", v.source, 0, "not expression is nil", "")
		}
		inner, err := v.validateExpr(scope, expr.Expr)
		if err != nil {
			return nil, err
		}
		return &validatedNot{expr: inner}, nil
	case *ComparisonExpr:
		if expr == nil {
			return nil, queryValidationError(CodeInvalidSyntax, "filter", v.source, 0, "comparison expression is nil", "")
		}
		return v.validatePathComparison(scope, expr)
	case *QuantifierExpr:
		if expr == nil {
			return nil, queryValidationError(CodeInvalidSyntax, "filter", v.source, 0, "quantifier expression is nil", "")
		}
		return v.validateQuantifier(scope, expr)
	default:
		return nil, queryValidationError(CodeInvalidSyntax, "filter", v.source, expression.Span().Start, fmt.Sprintf("unsupported expression type %T", expression), "")
	}
}

func (v *expressionValidation) validatePathComparison(scope expressionScope, expr *ComparisonExpr) (validatedExpr, *Error) {
	segments, ok := validPath(expr.Field)
	if !ok {
		return nil, queryValidationError(CodeInvalidSyntax, "filter", v.source, expr.FieldSource.Start, fmt.Sprintf("invalid field path %q", expr.Field), expr.Field)
	}
	segments, err := v.scopedPath(scope, segments, expr.Field, expr.FieldSource.Start)
	if err != nil {
		return nil, err
	}
	if len(segments) > v.limits.MaxPathDepth {
		return nil, queryValidationError(CodeLimitExceeded, "filter", v.source, expr.FieldSource.Start, fmt.Sprintf("path depth exceeds %d", v.limits.MaxPathDepth), expr.Field)
	}
	if len(segments) == 0 {
		return nil, queryValidationError(CodeInvalidSyntax, "filter", v.source, expr.FieldSource.Start, "field path must select a field", expr.Field)
	}

	type relationStep struct {
		relationship *modelRelationship
		parentTable  string
		alias        string
	}
	currentSchema, parentTable := scope.schema, scope.table
	steps := make([]relationStep, 0, len(segments)-1)
	for _, name := range segments[:len(segments)-1] {
		relationship, exists := currentSchema.relationships[name]
		if !exists {
			return nil, invalidRelationship("filter", v.source, expr.FieldSource.Start, expr.Field, fmt.Sprintf("unknown or unexposed relationship %q", name))
		}
		if relationship.cardinality != RelationshipOne {
			return nil, invalidRelationship("filter", v.source, expr.FieldSource.Start, expr.Field, fmt.Sprintf("to-many relationship %q requires any or all", name))
		}
		alias := v.alias()
		steps = append(steps, relationStep{relationship: relationship, parentTable: parentTable, alias: alias})
		currentSchema, parentTable = relationship.target, alias
	}
	fieldName := segments[len(segments)-1]
	field, exists := currentSchema.fields[fieldName]
	if !exists {
		return nil, queryValidationError(CodeUnknownField, "filter", v.source, expr.FieldSource.Start, fmt.Sprintf("unknown field %q", expr.Field), expr.Field)
	}
	table := ""
	if scope.qualifyFields || len(steps) > 0 {
		table = parentTable
	}
	result, validationErr := validateScalarComparison(field, table, expr, v.source)
	if validationErr != nil {
		return nil, validationErr
	}
	for index := len(steps) - 1; index >= 0; index-- {
		step := steps[index]
		result = &validatedRelationship{relationship: step.relationship, parentTable: step.parentTable, alias: step.alias, predicate: result, mode: relationshipExists}
	}
	return result, nil
}

func (v *expressionValidation) validateQuantifier(scope expressionScope, expr *QuantifierExpr) (validatedExpr, *Error) {
	if expr.Operator != Any && expr.Operator != All {
		return nil, queryValidationError(CodeInvalidSyntax, "filter", v.source, expr.OperatorSource.Start, fmt.Sprintf("unsupported quantifier operator %d", expr.Operator), expr.Relationship)
	}
	if !isValidIdentifier(expr.Variable) || isReservedName(expr.Variable) {
		return nil, queryValidationError(CodeInvalidSyntax, "filter", v.source, expr.VariableSource.Start, fmt.Sprintf("invalid quantifier variable %q", expr.Variable), expr.Relationship)
	}
	if _, duplicate := v.variables[expr.Variable]; duplicate {
		return nil, invalidRelationship("filter", v.source, expr.VariableSource.Start, expr.Relationship, fmt.Sprintf("quantifier variable %q shadows an active variable", expr.Variable))
	}
	segments, ok := validPath(expr.Relationship)
	if !ok {
		return nil, queryValidationError(CodeInvalidSyntax, "filter", v.source, expr.RelationshipSource.Start, fmt.Sprintf("invalid relationship path %q", expr.Relationship), expr.Relationship)
	}
	segments, err := v.scopedPath(scope, segments, expr.Relationship, expr.RelationshipSource.Start)
	if err != nil {
		return nil, err
	}
	if len(segments) == 0 || len(segments) > v.limits.MaxPathDepth {
		return nil, queryValidationError(CodeLimitExceeded, "filter", v.source, expr.RelationshipSource.Start, fmt.Sprintf("path depth exceeds %d", v.limits.MaxPathDepth), expr.Relationship)
	}

	type relationStep struct {
		relationship *modelRelationship
		parentTable  string
		alias        string
	}
	currentSchema, parentTable := scope.schema, scope.table
	steps := make([]relationStep, 0, len(segments))
	for index, name := range segments {
		relationship, exists := currentSchema.relationships[name]
		if !exists {
			return nil, invalidRelationship("filter", v.source, expr.RelationshipSource.Start, expr.Relationship, fmt.Sprintf("unknown or unexposed relationship %q", name))
		}
		last := index == len(segments)-1
		if last && relationship.cardinality != RelationshipMany {
			return nil, invalidRelationship("filter", v.source, expr.OperatorSource.Start, expr.Relationship, fmt.Sprintf("relationship %q is not to-many", name))
		}
		if !last && relationship.cardinality != RelationshipOne {
			return nil, invalidRelationship("filter", v.source, expr.RelationshipSource.Start, expr.Relationship, fmt.Sprintf("to-many relationship %q requires its own quantifier", name))
		}
		alias := v.alias()
		steps = append(steps, relationStep{relationship: relationship, parentTable: parentTable, alias: alias})
		currentSchema, parentTable = relationship.target, alias
	}

	v.variables[expr.Variable] = struct{}{}
	predicate, validationErr := v.validateExpr(expressionScope{schema: currentSchema, table: parentTable, variable: expr.Variable, qualifyFields: true}, expr.Predicate)
	delete(v.variables, expr.Variable)
	if validationErr != nil {
		return nil, validationErr
	}
	last := steps[len(steps)-1]
	mode := relationshipAny
	if expr.Operator == All {
		mode = relationshipAll
	}
	var result validatedExpr = &validatedRelationship{relationship: last.relationship, parentTable: last.parentTable, alias: last.alias, predicate: predicate, mode: mode}
	for index := len(steps) - 2; index >= 0; index-- {
		step := steps[index]
		result = &validatedRelationship{relationship: step.relationship, parentTable: step.parentTable, alias: step.alias, predicate: result, mode: relationshipExists}
	}
	return result, nil
}

func (v *expressionValidation) scopedPath(scope expressionScope, segments []string, fullPath string, offset int) ([]string, *Error) {
	if scope.variable == "" {
		return segments, nil
	}
	if len(segments) == 0 || segments[0] != scope.variable {
		return nil, invalidRelationship("filter", v.source, offset, fullPath, fmt.Sprintf("path must be rooted at quantifier variable %q", scope.variable))
	}
	return segments[1:], nil
}

func (v *expressionValidation) alias() string {
	for {
		v.nextAlias++
		candidate := fmt.Sprintf("gotq_rel_%d", v.nextAlias)
		if _, reserved := v.reserved[candidate]; reserved {
			continue
		}
		v.reserved[candidate] = struct{}{}
		return candidate
	}
}

func collectStorageNames(schema *modelSchema, names map[string]struct{}) {
	collectStorageNamesSeen(schema, names, make(map[*modelSchema]struct{}))
}

func collectStorageNamesSeen(schema *modelSchema, names map[string]struct{}, seen map[*modelSchema]struct{}) {
	if schema == nil {
		return
	}
	if _, visited := seen[schema]; visited {
		return
	}
	seen[schema] = struct{}{}
	names[schema.table] = struct{}{}
	for _, relationship := range schema.relationships {
		if relationship.metadata != nil && relationship.metadata.JoinTable != nil {
			names[relationship.metadata.JoinTable.Table] = struct{}{}
		}
		collectStorageNamesSeen(relationship.target, names, seen)
	}
}

func validateScalarComparison(field *modelField, table string, expr *ComparisonExpr, source string) (validatedExpr, *Error) {
	if !field.filterable {
		return nil, queryValidationError(CodeNotFilterable, "filter", source, expr.FieldSource.Start, fmt.Sprintf("field %q is not filterable", expr.Field), expr.Field)
	}
	if _, ok := field.operatorSet[expr.Operator]; !ok {
		err := queryValidationError(
			CodeOperatorNotAllowed,
			"filter",
			source,
			expr.OpSource.Start,
			fmt.Sprintf("operator %q cannot be used with field %q of type %s\nallowed operators: %s", expr.Operator, expr.Field, field.kind, joinOperators(field.operators)),
			expr.Field,
		)
		kind, operator := field.kind, expr.Operator
		err.Kind = &kind
		err.Operator = &operator
		err.AllowedOperators = append([]ComparisonOperator(nil), field.operators...)
		return nil, err
	}
	if expr.Operator == IsNull || expr.Operator == IsNotNull {
		if literalPresent(expr.Literal) || len(expr.Literals) != 0 {
			return nil, queryValidationError(CodeInvalidSyntax, "filter", source, expr.Source.Start, "null predicate must not contain literals", expr.Field)
		}
		return &validatedComparison{field: field, table: table, operator: expr.Operator}, nil
	}
	if expr.Operator == In || expr.Operator == NotIn {
		if literalPresent(expr.Literal) {
			return nil, queryValidationError(CodeInvalidSyntax, "filter", source, expr.Source.Start, "in predicate must use only its literal list", expr.Field)
		}
		if len(expr.Literals) == 0 {
			return nil, queryValidationError(CodeInvalidSyntax, "filter", source, expr.Source.Start, "in list must contain at least one literal", expr.Field)
		}
		values := make([]any, 0, len(expr.Literals))
		for _, literal := range expr.Literals {
			value, conversionErr := convertLiteral(field, literal)
			if conversionErr != nil {
				return nil, literalValidationError(field, expr, literal, source, conversionErr)
			}
			values = append(values, value)
		}
		return &validatedComparison{field: field, table: table, operator: expr.Operator, values: values}, nil
	}
	if len(expr.Literals) != 0 {
		return nil, queryValidationError(CodeInvalidSyntax, "filter", source, expr.Source.Start, "scalar predicate must contain exactly one literal", expr.Field)
	}
	value, conversionErr := convertLiteral(field, expr.Literal)
	if conversionErr != nil {
		return nil, literalValidationError(field, expr, expr.Literal, source, conversionErr)
	}
	return &validatedComparison{field: field, table: table, operator: expr.Operator, value: value}, nil
}

func validateSortPath(schema *modelSchema, term SortTerm, source string, limits Limits) (validatedOrder, *Error) {
	segments, ok := validPath(term.Field)
	if !ok {
		return validatedOrder{}, queryValidationError(CodeInvalidParameter, "sort", source, term.Source.Start, fmt.Sprintf("invalid sort path %q", term.Field), term.Field)
	}
	if len(segments) > limits.MaxPathDepth {
		return validatedOrder{}, queryValidationError(CodeLimitExceeded, "sort", source, term.Source.Start, fmt.Sprintf("sort path depth exceeds %d", limits.MaxPathDepth), term.Field)
	}
	current := schema
	goNames := make([]string, 0, len(segments)-1)
	for _, name := range segments[:len(segments)-1] {
		relationship, exists := current.relationships[name]
		if !exists {
			return validatedOrder{}, invalidRelationship("sort", source, term.Source.Start, term.Field, fmt.Sprintf("unknown or unexposed relationship %q", name))
		}
		if relationship.cardinality != RelationshipOne {
			return validatedOrder{}, invalidRelationship("sort", source, term.Source.Start, term.Field, fmt.Sprintf("sorting through to-many relationship %q is not allowed", name))
		}
		goNames = append(goNames, relationship.goName)
		current = relationship.target
	}
	field, exists := current.fields[segments[len(segments)-1]]
	if !exists {
		return validatedOrder{}, queryValidationError(CodeUnknownField, "sort", source, term.Source.Start, fmt.Sprintf("unknown field %q", term.Field), term.Field)
	}
	if !field.sortable {
		return validatedOrder{}, queryValidationError(CodeNotSortable, "sort", source, term.Source.Start, fmt.Sprintf("field %q is not sortable", term.Field), term.Field)
	}
	order := validatedOrder{field: field, desc: term.Desc}
	if len(goNames) > 0 {
		order.joinPath = strings.Join(goNames, ".")
		order.table = strings.Join(goNames, "__")
	}
	return order, nil
}

func validPath(path string) ([]string, bool) {
	if path == "" {
		return nil, false
	}
	segments := strings.Split(path, "/")
	for _, segment := range segments {
		if !isValidIdentifier(segment) || isReservedName(segment) {
			return nil, false
		}
	}
	return segments, true
}

func pathDepth(path string, scoped bool) int {
	segments, ok := validPath(path)
	if !ok {
		return 0
	}
	if scoped && len(segments) > 0 {
		return len(segments) - 1
	}
	return len(segments)
}

func invalidRelationship(parameter, source string, offset int, path, message string) *Error {
	return queryValidationError(CodeInvalidRelationship, parameter, source, offset, message, path)
}

func literalPresent(literal Literal) bool {
	return literal.Kind != StringLiteral || literal.Raw != "" || literal.Value != nil || literal.Source != (Span{})
}

func literalValidationError(field *modelField, expr *ComparisonExpr, literal Literal, source string, cause error) *Error {
	err := queryValidationError(CodeInvalidLiteral, "filter", source, literal.Source.Start, fmt.Sprintf("invalid literal %q for field %q of type %s: %v", literal.Raw, expr.Field, field.kind, cause), expr.Field)
	kind, operator := field.kind, expr.Operator
	err.Kind = &kind
	err.Operator = &operator
	return err
}

func convertLiteral(field *modelField, literal Literal) (any, error) {
	if field.codec != nil {
		value, err := field.codec.ParseLiteral(literal, field.scalarType)
		if err != nil {
			return nil, fmt.Errorf("codec %q: %w", field.codec.Name(), err)
		}
		return convertToFieldType(field, value)
	}
	switch field.kind {
	case String:
		if literal.Kind != StringLiteral {
			return nil, fmt.Errorf("expected a quoted string")
		}
		value, ok := literal.Value.(string)
		if !ok {
			return nil, fmt.Errorf("string literal has invalid value type %T", literal.Value)
		}
		return convertToFieldType(field, value)
	case Bool:
		if literal.Kind != BoolLiteral {
			return nil, fmt.Errorf("expected true or false")
		}
		value, ok := literal.Value.(bool)
		if !ok {
			return nil, fmt.Errorf("boolean literal has invalid value type %T", literal.Value)
		}
		return convertToFieldType(field, value)
	case Int:
		if literal.Kind != NumberLiteral || strings.ContainsAny(literal.Raw, ".eE") {
			return nil, fmt.Errorf("expected a base-10 integer")
		}
		value, err := strconv.ParseInt(literal.Raw, 10, field.bits)
		if err != nil {
			return nil, err
		}
		return convertToFieldType(field, value)
	case Uint:
		if literal.Kind != NumberLiteral || strings.ContainsAny(literal.Raw, ".eE") || strings.HasPrefix(literal.Raw, "-") {
			return nil, fmt.Errorf("expected a non-negative base-10 integer")
		}
		value, err := strconv.ParseUint(literal.Raw, 10, field.bits)
		if err != nil {
			return nil, err
		}
		return convertToFieldType(field, value)
	case Float:
		if literal.Kind != NumberLiteral {
			return nil, fmt.Errorf("expected a number")
		}
		value, err := strconv.ParseFloat(literal.Raw, field.bits)
		if err != nil {
			return nil, err
		}
		if math.IsInf(value, 0) || math.IsNaN(value) {
			return nil, fmt.Errorf("expected a finite number")
		}
		return convertToFieldType(field, value)
	case Time:
		if literal.Kind != StringLiteral {
			return nil, fmt.Errorf("expected a quoted RFC 3339 timestamp")
		}
		text, ok := literal.Value.(string)
		if !ok {
			return nil, fmt.Errorf("time literal has invalid value type %T", literal.Value)
		}
		value, err := time.Parse(time.RFC3339, text)
		if err != nil {
			return nil, fmt.Errorf("expected a quoted RFC 3339 timestamp: %w", err)
		}
		return convertToFieldType(field, value)
	case Date:
		if literal.Kind != StringLiteral {
			return nil, fmt.Errorf("expected a quoted YYYY-MM-DD date")
		}
		text, ok := literal.Value.(string)
		if !ok {
			return nil, fmt.Errorf("date literal has invalid value type %T", literal.Value)
		}
		value, err := time.Parse("2006-01-02", text)
		if err != nil {
			return nil, fmt.Errorf("expected a quoted YYYY-MM-DD date: %w", err)
		}
		if field.scalarType.Kind() == reflect.String {
			return convertToFieldType(field, text)
		}
		return convertToFieldType(field, value)
	case UUID:
		return convertUUIDLiteral(field, literal)
	case Decimal:
		if literal.Kind != NumberLiteral {
			return nil, fmt.Errorf("expected an unquoted decimal number")
		}
		return convertToFieldType(field, literal.Raw)
	default:
		return nil, fmt.Errorf("unknown field type %d", field.kind)
	}
}

func convertUUIDLiteral(field *modelField, literal Literal) (any, error) {
	if literal.Kind != StringLiteral {
		return nil, fmt.Errorf("expected a quoted UUID")
	}
	text, ok := literal.Value.(string)
	if !ok {
		return nil, fmt.Errorf("UUID literal has invalid value type %T", literal.Value)
	}
	text = strings.ToLower(text)
	if len(text) != 36 || text[8] != '-' || text[13] != '-' || text[18] != '-' || text[23] != '-' {
		return nil, fmt.Errorf("expected canonical UUID form xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx")
	}
	bytes, err := hex.DecodeString(strings.ReplaceAll(text, "-", ""))
	if err != nil || len(bytes) != 16 {
		return nil, fmt.Errorf("expected hexadecimal UUID")
	}
	if field.scalarType.Kind() == reflect.String {
		return convertToFieldType(field, text)
	}
	var value [16]byte
	copy(value[:], bytes)
	return convertToFieldType(field, value)
}

func convertToFieldType(field *modelField, value any) (any, error) {
	source := reflect.ValueOf(value)
	if !source.IsValid() || !source.Type().ConvertibleTo(field.scalarType) {
		return nil, fmt.Errorf("value of type %T cannot be converted to Go field type %s", value, field.scalarType)
	}
	return source.Convert(field.scalarType).Interface(), nil
}

func queryValidationError(code ErrorCode, parameter, source string, offset int, message, field string) *Error {
	position := positionAt(source, offset)
	return &Error{
		Code:      code,
		Parameter: parameter,
		Position:  &position,
		Message:   message,
		Field:     field,
	}
}

func joinOperators(operators []ComparisonOperator) string {
	values := make([]string, len(operators))
	for i, operator := range operators {
		values[i] = operator.String()
	}
	return strings.Join(values, ", ")
}
