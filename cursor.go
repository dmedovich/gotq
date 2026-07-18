package query

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strconv"
)

const cursorVersion = 1

type cursorEnvelope struct {
	Version   int               `json:"v"`
	Signature string            `json:"s"`
	Values    []json.RawMessage `json:"k"`
}

type validatedCursor struct {
	values []any
}

func effectiveSort(model *modelSchema, requested []validatedOrder) ([]validatedOrder, *Error) {
	orders := append([]validatedOrder(nil), requested...)
	for index := range orders {
		if orders[index].table == "" {
			orders[index].table = model.table
		}
	}
	orderedRootColumns := make(map[string]struct{}, len(requested))
	for _, order := range requested {
		if order.joinPath == "" {
			orderedRootColumns[order.field.column] = struct{}{}
		}
	}
	for _, column := range model.primaryColumns {
		if _, exists := orderedRootColumns[column]; exists {
			continue
		}
		gormField := model.gormSchema.FieldsByDBName[column]
		if gormField == nil {
			return nil, schemaError("primary-key field metadata is missing", "")
		}
		scalarType := dereferenceType(gormField.FieldType)
		if scalarType == nil {
			return nil, schemaError("primary-key field type is invalid", "")
		}
		orders = append(orders, validatedOrder{field: &modelField{
			goName:     gormField.Name,
			column:     column,
			scalarType: scalarType,
		}, table: model.table})
	}
	return orders, nil
}

func decodeCursor(encoded string, maxBytes int, model *modelSchema, orders []validatedOrder) (*validatedCursor, *Error) {
	if len(encoded) > maxBytes {
		return nil, cursorError(CodeLimitExceeded, encoded, maxBytes, fmt.Sprintf("cursor must not exceed %d bytes", maxBytes))
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || base64.RawURLEncoding.EncodeToString(raw) != encoded {
		return nil, cursorError(CodeInvalidCursor, encoded, 0, "cursor is not canonical base64url")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var envelope cursorEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return nil, cursorError(CodeInvalidCursor, encoded, 0, "cursor payload is invalid")
	}
	if err := requireJSONEOF(decoder); err != nil {
		return nil, cursorError(CodeInvalidCursor, encoded, 0, "cursor payload has trailing data")
	}
	canonical, err := json.Marshal(envelope)
	if err != nil || !bytes.Equal(canonical, raw) {
		return nil, cursorError(CodeInvalidCursor, encoded, 0, "cursor payload is not canonical")
	}
	if envelope.Version != cursorVersion {
		return nil, cursorError(CodeInvalidCursor, encoded, 0, "cursor version is not supported")
	}
	if envelope.Signature != cursorSignature(model, orders) {
		return nil, cursorError(CodeInvalidCursor, encoded, 0, "cursor does not match the effective sort")
	}
	if len(envelope.Values) != len(orders) {
		return nil, cursorError(CodeInvalidCursor, encoded, 0, "cursor key count does not match the effective sort")
	}
	values := make([]any, len(orders))
	for index, rawValue := range envelope.Values {
		if bytes.Equal(rawValue, []byte("null")) {
			if orders[index].joinPath == "" && !orders[index].field.nullable {
				return nil, cursorError(CodeInvalidCursor, encoded, 0, "cursor contains null for a non-null sort key")
			}
			continue
		}
		value, valueErr := decodeCursorValue(rawValue, orders[index].field.scalarType)
		if valueErr != nil {
			return nil, cursorError(CodeInvalidCursor, encoded, 0, "cursor contains an invalid sort key")
		}
		values[index] = value
	}
	return &validatedCursor{values: values}, nil
}

func encodeCursor(model *modelSchema, orders []validatedOrder, values []any, maxBytes int) (string, error) {
	if len(values) != len(orders) {
		return "", fmt.Errorf("gotq: cursor key count does not match effective sort")
	}
	envelope := cursorEnvelope{
		Version:   cursorVersion,
		Signature: cursorSignature(model, orders),
		Values:    make([]json.RawMessage, len(values)),
	}
	for index, value := range values {
		raw, err := json.Marshal(value)
		if err != nil {
			return "", fmt.Errorf("gotq: encode cursor key %d: %w", index, err)
		}
		if !json.Valid(raw) {
			return "", fmt.Errorf("gotq: encode cursor key %d: invalid JSON", index)
		}
		envelope.Values[index] = raw
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		return "", fmt.Errorf("gotq: encode cursor payload: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	if len(encoded) > maxBytes {
		return "", fmt.Errorf("gotq: encoded cursor exceeds %d bytes", maxBytes)
	}
	return encoded, nil
}

func decodeCursorValue(raw json.RawMessage, target reflect.Type) (any, error) {
	if target == nil {
		return nil, fmt.Errorf("missing cursor target type")
	}
	pointer := reflect.New(target)
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(pointer.Interface()); err != nil {
		return nil, err
	}
	if err := requireJSONEOF(decoder); err != nil {
		return nil, err
	}
	value := pointer.Elem().Interface()
	canonical, err := json.Marshal(value)
	if err != nil || !bytes.Equal(canonical, raw) {
		return nil, fmt.Errorf("non-canonical cursor key")
	}
	return value, nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if err == io.EOF {
		return nil
	}
	if err == nil {
		return fmt.Errorf("unexpected JSON value")
	}
	return err
}

func cursorSignature(model *modelSchema, orders []validatedOrder) string {
	digest := sha256.New()
	_, _ = digest.Write([]byte("gotq-cursor-v1\x00"))
	_, _ = digest.Write([]byte(model.table))
	for index, order := range orders {
		table := order.table
		if table == "" {
			table = model.table
		}
		_, _ = fmt.Fprintf(digest, "\x00%d\x00%s\x00%s\x00%s\x00%s\x00%s\x00%t", index, table, order.field.column, order.joinPath, order.field.scalarType.PkgPath(), order.field.scalarType.String(), order.desc)
	}
	return base64.RawURLEncoding.EncodeToString(digest.Sum(nil)[:18])
}

func cursorError(code ErrorCode, input string, offset int, message string) *Error {
	if offset > len(input) {
		offset = len(input)
	}
	return queryValidationError(code, "cursor", input, offset, message, "")
}

func cursorColumnAlias(index int) string {
	return "__gotq_cursor_" + strconv.Itoa(index)
}
