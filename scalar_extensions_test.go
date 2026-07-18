package query

import (
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

type scalarExtensionModel struct {
	ID     uint         `json:"id" gorm:"primaryKey"`
	Date   DateValue    `json:"date"`
	UUID   UUIDValue    `json:"uuid"`
	Amount DecimalValue `json:"amount"`
	Stamp  time.Time    `json:"stamp"`
	Code   customCode   `json:"code"`
}

type customCode string

type upperCodeCodec struct{}

func (upperCodeCodec) Name() string { return "upper-code" }
func (upperCodeCodec) ValidateType(target reflect.Type) error {
	if target.Kind() != reflect.String {
		return fmt.Errorf("expected string target")
	}
	return nil
}
func (upperCodeCodec) ParseLiteral(literal Literal, _ reflect.Type) (any, error) {
	if literal.Kind != StringLiteral {
		return nil, fmt.Errorf("expected quoted code")
	}
	value, ok := literal.Value.(string)
	if !ok || len(value) != 2 {
		return nil, fmt.Errorf("code must contain two characters")
	}
	return customCode(strings.ToUpper(value)), nil
}

func scalarExtensionSchema(t *testing.T) *ModelSchema[scalarExtensionModel] {
	t.Helper()
	schema, err := Schema[scalarExtensionModel]().
		Expose("id", Sortable()).
		Expose("date", Filterable(), Sortable()).
		Expose("uuid", Filterable(), Sortable()).
		Expose("amount", Filterable(), Sortable()).
		Field("stamp", Date, Filterable(), Sortable()).
		Expose("code", WithCodec(upperCodeCodec{}), Filterable()).
		Bind(dryRunDB(t))
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func TestDateUUIDDecimalAndCustomCodecConversion(t *testing.T) {
	t.Parallel()
	schema := scalarExtensionSchema(t)
	filter := "date eq '2026-07-16' and uuid eq '550E8400-E29B-41D4-A716-446655440000' and amount gt 12.340 and stamp eq '2026-07-16' and code eq 'ab'"
	q, err := ParseHTTP(url.Values{"filter": {filter}})
	if err != nil {
		t.Fatal(err)
	}
	validated, validationErr := validateQuery(schema, q)
	if validationErr != nil {
		t.Fatal(validationErr)
	}
	scoped, compileErr := compileGORM(dryRunDB(t).Model(&scalarExtensionModel{}), validated)
	if compileErr != nil {
		t.Fatal(compileErr)
	}
	vars := scoped.Find(&[]scalarExtensionModel{}).Statement.Vars
	if len(vars) != 5 {
		t.Fatalf("Vars = %#v", vars)
	}
	if vars[0] != DateValue("2026-07-16") || vars[1] != UUIDValue("550e8400-e29b-41d4-a716-446655440000") || vars[2] != DecimalValue("12.340") {
		t.Fatalf("scalar Vars = %#v", vars[:3])
	}
	stamp, ok := vars[3].(time.Time)
	if !ok || stamp.Format("2006-01-02") != "2026-07-16" || stamp.Location() != time.UTC {
		t.Fatalf("date time var = %#v", vars[3])
	}
	if vars[4] != customCode("AB") {
		t.Fatalf("custom codec var = %#v", vars[4])
	}
}

func TestScalarExtensionsRejectInvalidLiterals(t *testing.T) {
	t.Parallel()
	schema := scalarExtensionSchema(t)
	for _, filter := range []string{
		"date eq '2026-02-30'",
		"uuid eq 'not-a-uuid'",
		"amount eq '12.3'",
		"code eq 'toolong'",
	} {
		q, err := ParseHTTP(url.Values{"filter": {filter}})
		if err != nil {
			t.Fatal(err)
		}
		_, validationErr := validateQuery(schema, q)
		if validationErr == nil || validationErr.Code != CodeInvalidLiteral {
			t.Fatalf("filter %q error = %#v", filter, validationErr)
		}
	}
}

func TestCustomCodecConfigurationIsValidated(t *testing.T) {
	t.Parallel()
	type badCodec struct{ upperCodeCodec }
	var nilCodec *badCodec
	if _, err := Schema[scalarExtensionModel]().Expose("code", WithCodec(nilCodec)).Bind(dryRunDB(t)); err == nil {
		t.Fatal("typed nil codec was accepted")
	}
	if _, err := Schema[scalarExtensionModel]().Expose("id", WithCodec(upperCodeCodec{})).Bind(dryRunDB(t)); err == nil {
		t.Fatal("codec accepted an incompatible target")
	}
}
