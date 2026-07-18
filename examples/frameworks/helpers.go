package frameworkexamples

import (
	"encoding/json"
	"io"
)

func writeJSON(writer io.Writer, value any) error {
	return json.NewEncoder(writer).Encode(value)
}
