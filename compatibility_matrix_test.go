package query

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestReleaseDependencySupportMatrix(t *testing.T) {
	module, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"gorm.io/gorm":            "v1.31.2",
		"gorm.io/driver/sqlite":   "v1.6.0",
		"gorm.io/driver/postgres": "v1.6.0",
		"gorm.io/driver/mysql":    "v1.6.0",
	}
	for dependency, version := range want {
		declaration := "\t" + dependency + " " + version + "\n"
		if !strings.Contains(string(module), declaration) {
			t.Errorf("go.mod does not pin supported dependency %s@%s", dependency, version)
		}
	}

	db := newEngineTestDB(t)
	var sqliteVersion string
	if err := db.Raw("SELECT sqlite_version()").Scan(&sqliteVersion).Error; err != nil {
		t.Fatal(err)
	}
	if sqliteVersion != "3.45.1" {
		t.Fatalf("SQLite runtime version = %q, supported embedded version is 3.45.1", sqliteVersion)
	}
	t.Logf("support matrix: Go %s, SQLite %s", runtime.Version(), sqliteVersion)
}
