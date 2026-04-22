package digestion

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// computeHash returns a deterministic SHA-256 hash of the row data.
// json.Marshal on map[string]any produces sorted keys in Go.
func computeHash(row map[string]any) string {
	b, err := json.Marshal(normalizedRow(row))
	if err != nil {
		// Fallback: hash the sorted key=value pairs.
		keys := make([]string, 0, len(row))
		for k := range row {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		h := sha256.New()
		enc := json.NewEncoder(h)
		for _, k := range keys {
			_ = enc.Encode(k)
			_ = enc.Encode(row[k])
		}
		return hex.EncodeToString(h.Sum(nil))
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// normalizedRow returns a copy of the row with all values converted to
// JSON-primitive types so that the hash is stable across serialization.
func normalizedRow(row map[string]any) map[string]any {
	out := make(map[string]any, len(row))
	for k, v := range row {
		out[k] = v // already normalized by connector.normalizeValue
	}
	return out
}

// changedColumns returns the names of columns whose values differ between
// oldData and newData.
func changedColumns(oldData, newData map[string]any) []string {
	oldJSON, _ := json.Marshal(normalizedRow(oldData))
	newJSON, _ := json.Marshal(normalizedRow(newData))

	var old, new_ map[string]any
	_ = json.Unmarshal(oldJSON, &old)
	_ = json.Unmarshal(newJSON, &new_)

	seen := make(map[string]struct{})
	var cols []string

	for k, ov := range old {
		nv := new_[k]
		if !jsonEqual(ov, nv) {
			cols = append(cols, k)
		}
		seen[k] = struct{}{}
	}
	for k := range new_ {
		if _, ok := seen[k]; !ok {
			cols = append(cols, k)
		}
	}
	sort.Strings(cols)
	return cols
}

func jsonEqual(a, b any) bool {
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	return string(aj) == string(bj)
}
