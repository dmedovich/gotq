package query

import (
	"fmt"
	"reflect"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type cursorSelectSQL struct {
	modelTable string
	orders     []validatedOrder
	aliases    []string
	distinct   bool
	base       clause.Expression
	columns    []clause.Column
}

func (sql cursorSelectSQL) Build(builder clause.Builder) {
	if sql.distinct {
		builder.WriteString("DISTINCT ")
	}
	if sql.base != nil {
		sql.base.Build(builder)
	} else {
		for index, column := range sql.columns {
			if index > 0 {
				builder.WriteByte(',')
			}
			builder.WriteQuoted(column)
		}
	}
	for index, order := range sql.orders {
		builder.WriteByte(',')
		builder.WriteQuoted(cursorOrderColumn(sql.modelTable, order))
		builder.WriteString(" AS ")
		builder.WriteQuoted(clause.Column{Name: sql.aliases[index]})
	}
}

func findCursorRows[T any](scope *gorm.DB, model *modelSchema, orders []validatedOrder) ([]T, [][]any, error) {
	if scope == nil {
		return nil, nil, fmt.Errorf("gotq: cursor query scope is nil")
	}
	if model == nil || model.gormSchema == nil {
		return nil, nil, fmt.Errorf("gotq: cursor model schema is unavailable")
	}
	aliases := cursorColumnAliases(model, len(orders))
	distinct, base, columns := cursorBaseProjection(scope, model, orders)
	fields := make([]reflect.StructField, 1, len(orders)+1)
	fields[0] = reflect.StructField{Name: "Item", Type: reflect.TypeFor[T](), Tag: `gorm:"embedded"`}
	for index, order := range orders {
		if order.field == nil || order.field.scalarType == nil {
			return nil, nil, fmt.Errorf("gotq: cursor sort key %d has no scalar type", index)
		}
		fields = append(fields, reflect.StructField{
			Name: "Cursor" + fmt.Sprint(index),
			Type: reflect.PointerTo(order.field.scalarType),
			Tag:  reflect.StructTag(`gorm:"column:` + aliases[index] + `"`),
		})
	}
	rowType := reflect.StructOf(fields)
	destination := reflect.New(reflect.SliceOf(rowType))
	query := scope.Clauses(clause.Select{Expression: cursorSelectSQL{
		modelTable: model.table,
		orders:     orders,
		aliases:    aliases,
		distinct:   distinct,
		base:       base,
		columns:    columns,
	}})
	result := query.Find(destination.Interface())
	if err := result.Error; err != nil {
		return nil, nil, err
	}
	rows := destination.Elem()
	items := make([]T, 0, rows.Len())
	keys := make([][]any, 0, rows.Len())
	for rowIndex := 0; rowIndex < rows.Len(); rowIndex++ {
		row := rows.Index(rowIndex)
		item := row.Field(0).Interface().(T)
		if model.gormSchema.AfterFind && !result.Statement.SkipHooks {
			hook, ok := any(&item).(interface{ AfterFind(*gorm.DB) error })
			if ok {
				if err := hook.AfterFind(result.Session(&gorm.Session{NewDB: true})); err != nil {
					return nil, nil, err
				}
			}
		}
		items = append(items, item)
		rowKeys := make([]any, len(orders))
		for orderIndex := range orders {
			value := row.Field(orderIndex + 1)
			if !value.IsNil() {
				rowKeys[orderIndex] = value.Elem().Interface()
			}
		}
		keys = append(keys, rowKeys)
	}
	return items, keys, nil
}

func cursorBaseProjection(scope *gorm.DB, model *modelSchema, orders []validatedOrder) (bool, clause.Expression, []clause.Column) {
	if selected, exists := scope.Statement.Clauses["SELECT"]; exists && selected.Expression != nil {
		return false, selected.Expression, nil
	}
	columns := make([]clause.Column, 0, len(model.gormSchema.DBNames)+len(orders))
	if len(scope.Statement.Selects) > 0 {
		for _, name := range scope.Statement.Selects {
			if field := model.gormSchema.LookUpField(name); field != nil {
				columns = append(columns, clause.Column{Name: field.DBName})
			} else {
				columns = append(columns, clause.Column{Name: name, Raw: true})
			}
		}
	} else if len(scope.Statement.Omits) > 0 {
		selected, _ := scope.Statement.SelectAndOmitColumns(false, false)
		for _, name := range model.gormSchema.DBNames {
			if included, exists := selected[name]; (exists && included) || !exists {
				columns = append(columns, clause.Column{Table: model.table, Name: name})
			}
		}
	} else {
		for _, name := range model.gormSchema.DBNames {
			columns = append(columns, clause.Column{Table: model.table, Name: name})
		}
	}
	columns = append(columns, cursorJoinedProjection(model, orders)...)
	return scope.Statement.Distinct, nil, columns
}

func cursorJoinedProjection(model *modelSchema, orders []validatedOrder) []clause.Column {
	seen := make(map[string]struct{})
	var columns []clause.Column
	for _, order := range orders {
		if order.joinPath == "" {
			continue
		}
		current := model.gormSchema
		var aliases []string
		for _, name := range strings.Split(order.joinPath, ".") {
			relationship := current.Relationships.Relations[name]
			if relationship == nil || relationship.FieldSchema == nil {
				break
			}
			aliases = append(aliases, name)
			alias := strings.Join(aliases, "__")
			if _, exists := seen[alias]; !exists {
				for _, dbName := range relationship.FieldSchema.DBNames {
					columns = append(columns, clause.Column{Table: alias, Name: dbName, Alias: alias + "__" + dbName})
				}
				seen[alias] = struct{}{}
			}
			current = relationship.FieldSchema
		}
	}
	return columns
}

func cursorColumnAliases(model *modelSchema, count int) []string {
	reserved := make(map[string]struct{}, len(model.gormSchema.FieldsByDBName)+count)
	for name := range model.gormSchema.FieldsByDBName {
		reserved[name] = struct{}{}
	}
	aliases := make([]string, count)
	for index := 0; index < count; index++ {
		candidate := cursorColumnAlias(index)
		for {
			if _, exists := reserved[candidate]; !exists {
				break
			}
			candidate += "_"
		}
		aliases[index] = candidate
		reserved[candidate] = struct{}{}
	}
	return aliases
}
