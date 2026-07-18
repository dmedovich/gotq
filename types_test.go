package query

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestErrorJSONUsesStableNamesAndOmitsAbsentMetadata(t *testing.T) {
	t.Parallel()

	position := Position{Offset: 4, Line: 1, Column: 5}
	kind, operator := Int, Contains
	err := &Error{
		Code:             CodeOperatorNotAllowed,
		Parameter:        "filter",
		Position:         &position,
		Message:          "not allowed",
		Field:            "age",
		Kind:             &kind,
		Operator:         &operator,
		AllowedOperators: []ComparisonOperator{Eq, Ne},
	}
	encoded, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	want := `{"code":"operator_not_allowed","parameter":"filter","position":{"offset":4,"line":1,"column":5},"message":"not allowed","field":"age","kind":"int","operator":"contains","allowedOperators":["eq","ne"]}`
	if got := string(encoded); got != want {
		t.Fatalf("JSON = %s\nwant = %s", got, want)
	}

	encoded, marshalErr = json.Marshal(&Error{Code: CodeInvalidSchema, Message: "bad schema"})
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	for _, absent := range []string{"parameter", "position", "field", "kind", "operator", "allowedOperators"} {
		if strings.Contains(string(encoded), `"`+absent+`"`) {
			t.Errorf("optional field %q is present in %s", absent, encoded)
		}
	}
}
