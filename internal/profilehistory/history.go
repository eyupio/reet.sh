// Package profilehistory compares profile revisions without depending on setup or storage.
package profilehistory

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
)

var ErrConflict = errors.New("setup profile histories differ; no revisions were overwritten. Preserve both copies under different names, then pull the server copy before creating a new revision")

// CanAdvance permits an identical history or an append to the exact existing history.
// A revision number alone cannot distinguish independent edits on two machines.
func CanAdvance(current, incoming []byte) error {
	var old, next struct {
		Revisions []any `json:"revisions"`
	}
	decode := func(data []byte, v any) error {
		d := json.NewDecoder(bytes.NewReader(data))
		d.UseNumber()
		return d.Decode(v)
	}
	if decode(current, &old) != nil || decode(incoming, &next) != nil || len(old.Revisions) > len(next.Revisions) {
		return ErrConflict
	}
	for i, r := range old.Revisions {
		if !reflect.DeepEqual(normalize(r), normalize(next.Revisions[i])) {
			return ErrConflict
		}
	}
	return nil
}

// Older writers omit optional zero fields; newer typed writers may emit them.
// These encodings describe the same metadata and must not fork a history.
func normalize(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, value := range x {
			if n := normalize(value); n != nil {
				out[k] = n
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case []any:
		if len(x) == 0 {
			return nil
		}
		out := make([]any, len(x))
		for i, value := range x {
			out[i] = normalize(value)
		}
		return out
	case string:
		if x == "" || x == "0001-01-01T00:00:00Z" {
			return nil
		}
	case bool:
		if !x {
			return nil
		}
	case json.Number:
		if x == "0" {
			return nil
		}
	}
	return v
}
