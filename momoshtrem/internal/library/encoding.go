package library

import (
	"encoding/json"
	"fmt"
)

// encodeStrings encodes a string list for a JSONB column. nil encodes as [],
// never null, so readers don't need to tell the two apart.
func encodeStrings(values []string) (string, error) {
	if values == nil {
		values = []string{}
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return "", fmt.Errorf("failed to encode list: %w", err)
	}
	return string(raw), nil
}

// decodeIDs decodes a bigint[] read as JSON (to_jsonb). Empty input, JSON
// null and [] decode as nil.
func decodeIDs(raw []byte) ([]int64, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var ids []int64
	if err := json.Unmarshal(raw, &ids); err != nil {
		return nil, fmt.Errorf("failed to decode ids: %w", err)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	return ids, nil
}

// decodeStrings decodes a JSON string list read from the database. Empty
// input and JSON null decode as nil.
func decodeStrings(raw []byte) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("failed to decode list: %w", err)
	}
	if len(values) == 0 {
		return nil, nil
	}
	return values, nil
}

// nullStreamRef scans a nullable stream_ref (JSONB) column: Ref is nil for
// NULL. A malformed document fails the Scan.
type nullStreamRef struct {
	Ref *StreamRef
}

func (n *nullStreamRef) Scan(src any) error {
	n.Ref = nil
	var raw []byte
	switch v := src.(type) {
	case nil:
		return nil
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("stream_ref: unexpected %T", src)
	}
	ref := &StreamRef{}
	if err := json.Unmarshal(raw, ref); err != nil {
		return fmt.Errorf("failed to decode stream_ref: %w", err)
	}
	n.Ref = ref
	return nil
}
