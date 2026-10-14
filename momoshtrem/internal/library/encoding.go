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
