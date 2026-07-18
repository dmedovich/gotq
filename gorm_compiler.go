package query

import (
	"fmt"
	"reflect"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

func compileGORM(db *gorm.DB, query *validatedQuery) (*gorm.DB, *Error) {
	if db == nil {
		return nil, schemaError("gorm DB is nil", "")
	}
	if query == nil {
		return nil, schemaError("validated query is nil", "")
	}

	scoped := db
	joined := make(map[string]struct{})
	for _, order := range query.sort {
		if order.joinPath == "" {
			continue
		}
		if _, exists := joined[order.joinPath]; exists {
			continue
		}
		joined[order.joinPath] = struct{}{}
		scoped = scoped.Joins(order.joinPath)
	}
	if query.filter != nil {
		expression, err := compileGORMExprWithOptions(query.filter, scoped.Statement.Unscoped)
		if err != nil {
			return nil, err
		}
		scoped = scoped.Where(expression)
	}
	if query.cursor != nil {
		if len(query.cursor.values) != len(query.sort) {
			return nil, schemaError("validated cursor does not match effective sort", "")
		}
		scoped = scoped.Where(cursorPredicateSQL{modelTable: scoped.Statement.Table, orders: query.sort, values: query.cursor.values})
	}
	if len(query.sort) > 0 {
		scoped = scoped.Order(clause.OrderBy{Expression: stableOrderSQL{modelTable: scoped.Statement.Table, orders: query.sort}})
	}
	if query.limit != nil {
		scoped = scoped.Limit(*query.limit)
	}
	if query.offset != nil {
		scoped = scoped.Offset(*query.offset)
	}
	return scoped, nil
}

type stableOrderSQL struct {
	modelTable string
	orders     []validatedOrder
}

func (sql stableOrderSQL) Build(builder clause.Builder) {
	for index, order := range sql.orders {
		if index > 0 {
			builder.WriteByte(',')
		}
		column := cursorOrderColumn(sql.modelTable, order)
		builder.WriteString("CASE WHEN ")
		builder.WriteQuoted(column)
		builder.WriteString(" IS NULL THEN 1 ELSE 0 END ASC,")
		builder.WriteQuoted(column)
		if order.desc {
			builder.WriteString(" DESC")
		} else {
			builder.WriteString(" ASC")
		}
	}
}

type cursorPredicateSQL struct {
	modelTable string
	orders     []validatedOrder
	values     []any
}

func (sql cursorPredicateSQL) Build(builder clause.Builder) {
	prefix := make([]clause.Expression, 0, len(sql.orders))
	branches := make([]clause.Expression, 0, len(sql.orders))
	for index, order := range sql.orders {
		column := cursorOrderColumn(sql.modelTable, order)
		value := sql.values[index]
		if value != nil {
			var comparison clause.Expression
			if order.desc {
				comparison = clause.Lt{Column: column, Value: value}
			} else {
				comparison = clause.Gt{Column: column, Value: value}
			}
			after := clause.Or(clause.Eq{Column: column, Value: nil}, comparison)
			conditions := append(append([]clause.Expression(nil), prefix...), after)
			branches = append(branches, clause.And(conditions...))
		}
		prefix = append(prefix, clause.Eq{Column: column, Value: value})
	}
	if len(branches) == 0 {
		builder.WriteString("1 = 0")
		return
	}
	clause.Or(branches...).Build(builder)
}

func cursorOrderColumn(modelTable string, order validatedOrder) clause.Column {
	table := order.table
	if table == "" {
		table = modelTable
	}
	return clause.Column{Table: table, Name: order.field.column}
}

func compileGORMExpr(expr validatedExpr) (clause.Expression, *Error) {
	return compileGORMExprWithOptions(expr, false)
}

func compileGORMExprWithOptions(expr validatedExpr, includeDeleted bool) (clause.Expression, *Error) {
	switch expr := expr.(type) {
	case *validatedLogical:
		left, err := compileGORMExprWithOptions(expr.left, includeDeleted)
		if err != nil {
			return nil, err
		}
		right, err := compileGORMExprWithOptions(expr.right, includeDeleted)
		if err != nil {
			return nil, err
		}
		switch expr.operator {
		case And:
			return clause.And(left, right), nil
		case Or:
			return clause.Or(left, right), nil
		default:
			return nil, schemaError(fmt.Sprintf("unsupported logical operator %d", expr.operator), "")
		}
	case *validatedNot:
		inner, err := compileGORMExprWithOptions(expr.expr, includeDeleted)
		if err != nil {
			return nil, err
		}
		return clause.Not(inner), nil
	case *validatedComparison:
		column := clause.Column{Table: expr.table, Name: expr.field.column}
		switch expr.operator {
		case Eq:
			return clause.Eq{Column: column, Value: expr.value}, nil
		case Ne:
			return clause.Neq{Column: column, Value: expr.value}, nil
		case Gt:
			return clause.Gt{Column: column, Value: expr.value}, nil
		case Gte:
			return clause.Gte{Column: column, Value: expr.value}, nil
		case Lt:
			return clause.Lt{Column: column, Value: expr.value}, nil
		case Lte:
			return clause.Lte{Column: column, Value: expr.value}, nil
		case Contains:
			value := reflect.ValueOf(expr.value)
			if !value.IsValid() || value.Kind() != reflect.String {
				return nil, schemaError("contains value is not a string", expr.field.publicName, expr.field.kind)
			}
			return containsExpression{column: column, value: literalContainsPattern(value.String())}, nil
		case StartsWith, EndsWith:
			value := reflect.ValueOf(expr.value)
			if !value.IsValid() || value.Kind() != reflect.String {
				return nil, schemaError("string pattern value is not a string", expr.field.publicName, expr.field.kind)
			}
			prefix, suffix := false, false
			if expr.operator == StartsWith {
				suffix = true
			} else {
				prefix = true
			}
			return containsExpression{column: column, value: literalLikePattern(value.String(), prefix, suffix)}, nil
		case In:
			return clause.IN{Column: column, Values: expr.values}, nil
		case NotIn:
			return clause.Not(clause.IN{Column: column, Values: expr.values}), nil
		case IsNull:
			return clause.Eq{Column: column, Value: nil}, nil
		case IsNotNull:
			return clause.Neq{Column: column, Value: nil}, nil
		default:
			return nil, schemaError(fmt.Sprintf("unsupported comparison operator %d", expr.operator), expr.field.publicName, expr.field.kind)
		}
	case *validatedRelationship:
		if expr.relationship == nil || expr.relationship.metadata == nil || expr.predicate == nil {
			return nil, schemaError("invalid validated relationship expression", "")
		}
		predicate, err := compileGORMExprWithOptions(expr.predicate, includeDeleted)
		if err != nil {
			return nil, err
		}
		return relationshipSQL{expression: expr, predicate: predicate, includeDeleted: includeDeleted}, nil
	default:
		return nil, schemaError(fmt.Sprintf("unsupported validated expression type %T", expr), "")
	}
}

type relationshipSQL struct {
	expression     *validatedRelationship
	predicate      clause.Expression
	includeDeleted bool
}

func (sql relationshipSQL) Build(builder clause.Builder) {
	expression := sql.expression
	metadata := expression.relationship.metadata
	negated := expression.mode == relationshipAll
	if negated {
		builder.WriteString("NOT ")
	}
	builder.WriteString("EXISTS (SELECT 1 FROM ")
	if metadata.Type == schema.Many2Many {
		sql.buildManyToMany(builder)
	} else {
		builder.WriteQuoted(clause.Table{Name: metadata.FieldSchema.Table, Alias: expression.alias})
		conditions := relationshipConditions(expression)
		if !sql.includeDeleted {
			conditions = append(conditions, softDeleteConditions(metadata.FieldSchema, expression.alias)...)
		}
		conditions = append(conditions, relationshipPredicateExpression(sql.predicate, negated))
		writeWhere(builder, conditions)
	}
	builder.WriteByte(')')
}

func (sql relationshipSQL) buildManyToMany(builder clause.Builder) {
	expression := sql.expression
	metadata := expression.relationship.metadata
	joinAlias := expression.alias + "_join"
	builder.WriteQuoted(clause.Table{Name: metadata.JoinTable.Table, Alias: joinAlias})
	builder.WriteString(" INNER JOIN ")
	builder.WriteQuoted(clause.Table{Name: metadata.FieldSchema.Table, Alias: expression.alias})
	builder.WriteString(" ON ")
	joinTarget := make([]clause.Expression, 0, len(metadata.References))
	correlations := make([]clause.Expression, 0, len(metadata.References)+2)
	for _, reference := range metadata.References {
		if reference.PrimaryValue != "" {
			correlations = append(correlations, clause.Eq{Column: clause.Column{Table: joinAlias, Name: reference.ForeignKey.DBName}, Value: reference.PrimaryValue})
			continue
		}
		if reference.OwnPrimaryKey {
			correlations = append(correlations, columnEquality(joinAlias, reference.ForeignKey.DBName, expression.parentTable, reference.PrimaryKey.DBName))
		} else {
			joinTarget = append(joinTarget, columnEquality(joinAlias, reference.ForeignKey.DBName, expression.alias, reference.PrimaryKey.DBName))
		}
	}
	if len(joinTarget) == 0 {
		builder.WriteString("1 = 0")
	} else {
		clause.And(joinTarget...).Build(builder)
	}
	if !sql.includeDeleted {
		correlations = append(correlations, softDeleteConditions(metadata.JoinTable, joinAlias)...)
		correlations = append(correlations, softDeleteConditions(metadata.FieldSchema, expression.alias)...)
	}
	correlations = append(correlations, relationshipPredicateExpression(sql.predicate, expression.mode == relationshipAll))
	writeWhere(builder, correlations)
}

func relationshipConditions(expression *validatedRelationship) []clause.Expression {
	metadata := expression.relationship.metadata
	conditions := make([]clause.Expression, 0, len(metadata.References))
	for _, reference := range metadata.References {
		if reference.PrimaryValue != "" {
			conditions = append(conditions, clause.Eq{Column: clause.Column{Table: expression.alias, Name: reference.ForeignKey.DBName}, Value: reference.PrimaryValue})
			continue
		}
		if reference.OwnPrimaryKey {
			conditions = append(conditions, columnEquality(expression.alias, reference.ForeignKey.DBName, expression.parentTable, reference.PrimaryKey.DBName))
		} else {
			conditions = append(conditions, columnEquality(expression.alias, reference.PrimaryKey.DBName, expression.parentTable, reference.ForeignKey.DBName))
		}
	}
	return conditions
}

func relationshipPredicateExpression(predicate clause.Expression, negate bool) clause.Expression {
	if negate {
		return notTrueExpression{expression: predicate}
	}
	return predicate
}

func columnEquality(leftTable, leftColumn, rightTable, rightColumn string) clause.Expression {
	return clause.Eq{
		Column: clause.Column{Table: leftTable, Name: leftColumn},
		Value:  clause.Column{Table: rightTable, Name: rightColumn},
	}
}

func softDeleteConditions(model *schema.Schema, table string) []clause.Expression {
	if model == nil {
		return nil
	}
	deletedAtType := reflect.TypeOf(gorm.DeletedAt{})
	conditions := make([]clause.Expression, 0, 1)
	for _, field := range model.Fields {
		fieldType := field.FieldType
		for fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}
		if fieldType == deletedAtType && field.DBName != "" {
			conditions = append(conditions, clause.Eq{Column: clause.Column{Table: table, Name: field.DBName}, Value: nil})
		}
	}
	return conditions
}

func writeWhere(builder clause.Builder, conditions []clause.Expression) {
	builder.WriteString(" WHERE ")
	if len(conditions) == 0 {
		builder.WriteString("1 = 0")
		return
	}
	clause.And(conditions...).Build(builder)
}

type notTrueExpression struct {
	expression clause.Expression
}

func (expression notTrueExpression) Build(builder clause.Builder) {
	builder.WriteString("COALESCE((")
	expression.expression.Build(builder)
	builder.WriteString("), FALSE) = FALSE")
}

type containsExpression struct {
	column clause.Column
	value  string
}

func (expression containsExpression) Build(builder clause.Builder) {
	builder.WriteQuoted(expression.column)
	builder.WriteString(" LIKE ")
	builder.AddVar(builder, expression.value)
	builder.WriteString(" ESCAPE '!'")
}

func literalContainsPattern(value string) string {
	return literalLikePattern(value, true, true)
}

func literalLikePattern(value string, prefix, suffix bool) string {
	var escaped strings.Builder
	escaped.Grow(len(value) + 2)
	if prefix {
		escaped.WriteByte('%')
	}
	for _, r := range value {
		switch r {
		case '%', '_', '!':
			escaped.WriteByte('!')
		}
		escaped.WriteRune(r)
	}
	if suffix {
		escaped.WriteByte('%')
	}
	return escaped.String()
}
