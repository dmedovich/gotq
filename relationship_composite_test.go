package query

import (
	"context"
	"net/url"
	"testing"
)

type compositeAccount struct {
	TenantID uint             `json:"tenantId" gorm:"primaryKey;autoIncrement:false"`
	Code     string           `json:"code" gorm:"primaryKey"`
	Entries  []compositeEntry `json:"entries" gorm:"foreignKey:TenantID,AccountCode;references:TenantID,Code"`
}

type compositeEntry struct {
	ID          uint   `json:"id"`
	TenantID    uint   `json:"tenantId"`
	AccountCode string `json:"accountCode"`
	Value       int    `json:"value"`
}

type compositeStudent struct {
	TenantID  uint              `json:"tenantId" gorm:"primaryKey;autoIncrement:false"`
	StudentID uint              `json:"studentId" gorm:"primaryKey;autoIncrement:false"`
	Courses   []compositeCourse `json:"courses" gorm:"many2many:composite_enrollments;foreignKey:TenantID,StudentID;joinForeignKey:StudentTenantID,StudentID;references:TenantID,Code;joinReferences:CourseTenantID,CourseCode"`
}

type compositeCourse struct {
	TenantID uint   `json:"tenantId" gorm:"primaryKey;autoIncrement:false"`
	Code     string `json:"code" gorm:"primaryKey"`
	Title    string `json:"title"`
}

func TestCompositeRelationshipCorrelatesEveryKeyAndPreservesTenantIsolation(t *testing.T) {
	db := relationshipDB(t)
	if err := db.AutoMigrate(&compositeAccount{}, &compositeEntry{}); err != nil {
		t.Fatal(err)
	}
	accounts := []compositeAccount{{TenantID: 1, Code: "shared"}, {TenantID: 2, Code: "shared"}}
	entries := []compositeEntry{{ID: 1, TenantID: 1, AccountCode: "shared", Value: 100}}
	if err := db.Create(&accounts).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entries).Error; err != nil {
		t.Fatal(err)
	}
	policy := Schema[compositeAccount]().
		Expose("tenantId", Sortable()).
		Expose("code", Sortable()).
		Relation("entries", Schema[compositeEntry]().Expose("value", Filterable(Gte)))
	engine, err := New(db, Config[compositeAccount]{Policy: policy, DefaultLimit: 10, MaxLimit: 20, MaxOffset: 100, AllowCount: true})
	if err != nil {
		t.Fatal(err)
	}
	page, err := engine.List(context.Background(), url.Values{"filter": {"entries/any(e: e/value gte 100)"}, "count": {"true"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].TenantID != 1 || page.Total == nil || *page.Total != 1 {
		t.Fatalf("cross-tenant composite result = %#v", page)
	}
	page, err = engine.From(db.Where("tenant_id = ?", 2)).List(context.Background(), url.Values{"filter": {"entries/any(e: e/value gte 100)"}, "count": {"true"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 0 || page.Total == nil || *page.Total != 0 {
		t.Fatalf("base tenant scope leaked relationship rows: %#v", page)
	}
}

func TestCompositeManyToManyCorrelatesRootAndTargetKeys(t *testing.T) {
	db := relationshipDB(t)
	if err := db.AutoMigrate(&compositeStudent{}, &compositeCourse{}); err != nil {
		t.Fatal(err)
	}
	students := []compositeStudent{{TenantID: 1, StudentID: 7}, {TenantID: 2, StudentID: 7}}
	courses := []compositeCourse{{TenantID: 1, Code: "go", Title: "public"}, {TenantID: 2, Code: "go", Title: "private"}}
	if err := db.Create(&students).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&courses).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&students[0]).Association("Courses").Append(&courses[0]); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&students[1]).Association("Courses").Append(&courses[1]); err != nil {
		t.Fatal(err)
	}
	policy := Schema[compositeStudent]().Relation("courses", Schema[compositeCourse]().Expose("title", Filterable(Eq)))
	engine, err := New(db, Config[compositeStudent]{Policy: policy, DefaultLimit: 10, MaxLimit: 20, MaxOffset: 100})
	if err != nil {
		t.Fatal(err)
	}
	page, err := engine.List(context.Background(), url.Values{"filter": {"courses/any(c: c/title eq 'private')"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].TenantID != 2 {
		t.Fatalf("composite many-to-many result = %#v", page.Items)
	}
}
