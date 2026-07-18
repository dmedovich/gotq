package query

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type relationshipCountry struct {
	ID   uint   `json:"id"`
	Code string `json:"code"`
}

type relationshipCompany struct {
	ID        uint                   `json:"id"`
	Name      string                 `json:"name"`
	CountryID uint                   `json:"countryId"`
	Country   relationshipCountry    `json:"country"`
	Users     []relationshipTestUser `json:"users" gorm:"foreignKey:CompanyID"`
	DeletedAt gorm.DeletedAt         `json:"-"`
}

type relationshipTestUser struct {
	ID        uint                `json:"id"`
	CompanyID *uint               `json:"companyId"`
	Employer  relationshipCompany `json:"company" gorm:"foreignKey:CompanyID"`
	Orders    []relationshipOrder `json:"orders" gorm:"foreignKey:UserID"`
	Roles     []relationshipRole  `json:"roles" gorm:"many2many:relationship_user_roles"`
}

type relationshipOrder struct {
	ID        uint               `json:"id"`
	UserID    uint               `json:"userId"`
	Total     int                `json:"total"`
	Status    *string            `json:"status"`
	Items     []relationshipItem `json:"items" gorm:"foreignKey:OrderID"`
	DeletedAt gorm.DeletedAt     `json:"-"`
}

func (relationshipOrder) TableName() string { return "gotq_rel_1" }

type relationshipItem struct {
	ID        uint           `json:"id"`
	OrderID   uint           `json:"orderId"`
	Price     int            `json:"price"`
	DeletedAt gorm.DeletedAt `json:"-"`
}

type relationshipRole struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

type relationshipPolymorphicOwner struct {
	ID   uint                         `json:"id"`
	Toys []relationshipPolymorphicToy `json:"toys" gorm:"polymorphic:Owner"`
}

type relationshipPolymorphicToy struct {
	ID        uint   `json:"id"`
	OwnerID   uint   `json:"ownerId"`
	OwnerType string `json:"ownerType"`
	Name      string `json:"name"`
}

func relationshipPolicies() (*SchemaBuilder[relationshipTestUser], *SchemaBuilder[relationshipCompany]) {
	country := Schema[relationshipCountry]().Expose("code", Filterable(Eq), Sortable())
	company := Schema[relationshipCompany]().
		Expose("name", Filterable(Eq), Sortable()).
		Relation("country", country)
	user := Schema[relationshipTestUser]().
		Expose("id", Sortable()).
		Relation("company", company, RelationGoField("Employer")).
		Relation("orders", Schema[relationshipOrder]().
			Expose("total", Filterable(Gt, Gte)).
			Expose("status", Filterable(Eq)).
			Relation("items", Schema[relationshipItem]().Expose("price", Filterable(Gte)))).
		Relation("roles", Schema[relationshipRole]().Expose("name", Filterable(Eq)))
	return user, company
}

func relationshipString(value string) *string { return &value }

func deletedAt(value time.Time) gorm.DeletedAt {
	return gorm.DeletedAt{Time: value, Valid: true}
}

func relationshipDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestRelationshipPolicyBindsExplicitNestedSchema(t *testing.T) {
	db := relationshipDB(t)
	policy, _ := relationshipPolicies()
	schema, err := policy.Bind(db)
	if err != nil {
		t.Fatal(err)
	}
	if got := schema.relationships["company"].cardinality; got != RelationshipOne {
		t.Fatalf("company cardinality = %q", got)
	}
	if got := schema.relationships["orders"].cardinality; got != RelationshipMany {
		t.Fatalf("orders cardinality = %q", got)
	}
	if got := schema.relationships["roles"].metadata.JoinTable.Table; got != "relationship_user_roles" {
		t.Fatalf("join table = %q", got)
	}

	description := schema.Describe()
	if len(description.Relationships) != 3 {
		t.Fatalf("relationships = %#v", description.Relationships)
	}
	company := description.Relationships[0]
	if company.Name != "company" || company.Cardinality != RelationshipOne || !company.Filterable || !company.Sortable || len(company.Schema.Relationships) != 1 {
		t.Fatalf("company description = %#v", company)
	}
	orders := description.Relationships[1]
	if orders.Name != "orders" || orders.Sortable || !orders.Filterable {
		t.Fatalf("orders description = %#v", orders)
	}
	encoded, err := json.Marshal(description)
	if err != nil {
		t.Fatal(err)
	}
	for _, storageName := range []string{"Employer", "CompanyID", "company_id", "relationship_user_roles"} {
		if strings.Contains(string(encoded), storageName) {
			t.Fatalf("description leaked %q: %s", storageName, encoded)
		}
	}
	description.Relationships[0].Schema.Fields[0].Name = "changed"
	again := schema.Describe()
	if again.Relationships[0].Schema.Fields[0].Name == "changed" {
		t.Fatal("nested description mutation changed bound policy")
	}
}

func TestRelationshipPolicyRejectsInvalidDeclarations(t *testing.T) {
	db := relationshipDB(t)
	validTarget := Schema[relationshipCompany]().Expose("name", Filterable())
	tests := []struct {
		name   string
		policy *SchemaBuilder[relationshipTestUser]
	}{
		{name: "nil target", policy: Schema[relationshipTestUser]().Relation("company", (*SchemaBuilder[relationshipCompany])(nil), RelationGoField("Employer"))},
		{name: "wrong target model", policy: Schema[relationshipTestUser]().Relation("company", Schema[relationshipOrder](), RelationGoField("Employer"))},
		{name: "missing association", policy: Schema[relationshipTestUser]().Relation("missing", validTarget)},
		{name: "duplicate", policy: Schema[relationshipTestUser]().Relation("company", validTarget, RelationGoField("Employer")).Relation("company", validTarget, RelationGoField("Employer"))},
		{name: "field conflict", policy: Schema[relationshipTestUser]().Expose("company", GoField("CompanyID")).Relation("company", validTarget, RelationGoField("Employer"))},
		{name: "empty override", policy: Schema[relationshipTestUser]().Relation("company", validTarget, RelationGoField(""))},
		{name: "nil option", policy: Schema[relationshipTestUser]().Relation("company", validTarget, RelationGoField("Employer"), nil)},
		{name: "duplicate override", policy: Schema[relationshipTestUser]().Relation("company", validTarget, RelationGoField("Employer"), RelationGoField("Employer"))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := test.policy.Bind(db); err == nil {
				t.Fatal("Bind accepted invalid relationship policy")
			}
		})
	}
	if _, err := Schema[relationshipTestUser]().Relation("company", validTarget, RelationGoField("Employer")).Build(); err == nil {
		t.Fatal("Build accepted relationships without GORM binding")
	}
}

func TestRelationshipPolicyRejectsCycles(t *testing.T) {
	db := relationshipDB(t)
	users := Schema[relationshipTestUser]()
	companies := Schema[relationshipCompany]()
	users.Relation("company", companies, RelationGoField("Employer"))
	companies.Relation("users", users)
	if _, err := users.Bind(db); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("Bind cycle error = %v", err)
	}
}

func TestRelationshipPolicyRejectsUnsupportedPolymorphicMetadata(t *testing.T) {
	db := relationshipDB(t)
	policy := Schema[relationshipPolymorphicOwner]().Relation("toys", Schema[relationshipPolymorphicToy]().Expose("name", Filterable()))
	if _, err := policy.Bind(db); err == nil || !strings.Contains(err.Error(), "polymorphic") {
		t.Fatalf("polymorphic Bind error = %v", err)
	}
}
