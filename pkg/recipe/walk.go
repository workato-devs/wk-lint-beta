package recipe

import (
	"encoding/json"
	"strconv"
)

// StringVisitor is called for every string leaf found during a JSON walk.
type StringVisitor func(pointer string, value string)

// WalkStrings recursively walks a json.RawMessage, calling visitor for every
// string leaf with its JSON pointer path.
func WalkStrings(raw json.RawMessage, basePath string, visitor StringVisitor) {
	if len(raw) == 0 {
		return
	}

	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return
	}
	walkValue(v, basePath, visitor)
}

func walkValue(v interface{}, path string, visitor StringVisitor) {
	switch val := v.(type) {
	case map[string]interface{}:
		for k, child := range val {
			walkValue(child, path+"/"+k, visitor)
		}
	case []interface{}:
		for i, child := range val {
			walkValue(child, path+"/"+strconv.Itoa(i), visitor)
		}
	case string:
		visitor(path, val)
	}
}
