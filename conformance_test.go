package query

import (
	"encoding/json"
	"net/url"
	"os"
	"testing"
)

type conformanceFile struct {
	SyntaxVersion string            `json:"syntaxVersion"`
	Cases         []conformanceCase `json:"cases"`
}

type conformanceCase struct {
	Name                 string              `json:"name"`
	Values               map[string][]string `json:"values"`
	CompatibilityAliases bool                `json:"compatibilityAliases"`
	Accept               bool                `json:"accept"`
	ErrorCode            ErrorCode           `json:"errorCode"`
	Parameter            string              `json:"parameter"`
}

func TestPublishedV1ConformanceFixtures(t *testing.T) {
	data, err := os.ReadFile("conformance/v1/queries.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures conformanceFile
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	if fixtures.SyntaxVersion != SyntaxVersion {
		t.Fatalf("fixture syntax = %q, library syntax = %q", fixtures.SyntaxVersion, SyntaxVersion)
	}
	if len(fixtures.Cases) == 0 {
		t.Fatal("fixture set is empty")
	}
	seen := make(map[string]struct{}, len(fixtures.Cases))
	for _, fixture := range fixtures.Cases {
		fixture := fixture
		t.Run(fixture.Name, func(t *testing.T) {
			if fixture.Name == "" {
				t.Fatal("fixture name is empty")
			}
			if _, duplicate := seen[fixture.Name]; duplicate {
				t.Fatalf("duplicate fixture name %q", fixture.Name)
			}
			seen[fixture.Name] = struct{}{}
			var options []ParseOption
			if fixture.CompatibilityAliases {
				options = append(options, WithCompatibilityAliases())
			}
			_, parseErr := ParseHTTP(url.Values(fixture.Values), options...)
			if fixture.Accept {
				if parseErr != nil {
					t.Fatalf("ParseHTTP = %v", parseErr)
				}
				return
			}
			if parseErr == nil {
				t.Fatal("ParseHTTP accepted rejected fixture")
			}
			queryErr, ok := parseErr.(*Error)
			if !ok {
				t.Fatalf("error type = %T", parseErr)
			}
			if queryErr.Code != fixture.ErrorCode || queryErr.Parameter != fixture.Parameter {
				t.Fatalf("error = %#v, want code %q parameter %q", queryErr, fixture.ErrorCode, fixture.Parameter)
			}
		})
	}
}
