package query_test

import (
	query "github.com/dmedovich/gotq"
)

type exampleCompany struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

type exampleOrder struct {
	ID     uint `json:"id"`
	UserID uint `json:"userId"`
	Total  int  `json:"total"`
}

type exampleRelationshipUser struct {
	ID        uint           `json:"id"`
	CompanyID uint           `json:"companyId"`
	Company   exampleCompany `json:"company"`
	Orders    []exampleOrder `json:"orders" gorm:"foreignKey:UserID"`
}

func ExampleSchemaBuilder_Relation() {
	company := query.Schema[exampleCompany]().
		Expose("name", query.Filterable(query.Eq), query.Sortable())
	orders := query.Schema[exampleOrder]().
		Expose("total", query.Filterable(query.Gt, query.Gte))

	policy := query.Schema[exampleRelationshipUser]().
		Expose("id", query.Sortable()).
		Relation("company", company).
		Relation("orders", orders)

	_ = policy
}
