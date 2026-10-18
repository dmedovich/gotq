package query_test

import (
	"context"
	"fmt"

	query "github.com/dmedovich/gotq"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func ExampleConfig() {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		panic(err)
	}
	defer sqlDB.Close()
	if err = db.AutoMigrate(&User{}); err != nil {
		panic(err)
	}
	if err = db.Create(&[]User{{Name: "Bob"}, {Name: "Alice"}}).Error; err != nil {
		panic(err)
	}
	users, err := query.New(db, query.Config[User]{
		Policy:       query.Schema[User]().Expose("name", query.Sortable()),
		DefaultLimit: 1, MaxLimit: 100, MaxOffset: 1000,
		DefaultSort: "name",
		Observe: func(_ context.Context, event query.Observation) {
			// Replace this with your concurrency-safe histogram/counter recorder.
			// Only fixed phase and error-code labels are needed.
			fmt.Println(event.Phase, event.ErrorCode == "")
		},
	})
	if err != nil {
		panic(err)
	}
	page, err := users.List(context.Background(), nil)
	if err != nil {
		panic(err)
	}
	fmt.Println(page.Items[0].Name, page.Page.HasMore)
	// Output:
	// parse true
	// validate true
	// data true
	// Alice true
}
