package recipe

import (
	"encoding/json"
	"regexp"
	"strconv"
)

// StringContext provides contextual information about a string found during walking.
type StringContext struct {
	Pointer   string // JSON pointer path
	Value     string // the string value
	IsCondLHS bool   // true when inside input/conditions/N/lhs
}

// ContextStringVisitor is called for every string leaf with context information.
type ContextStringVisitor func(ctx StringContext)

// condLHSPattern matches paths ending in conditions/<digit>/lhs.
var condLHSPattern = regexp.MustCompile(`/conditions/\d+/lhs$`)

// WalkStringsWithContext recursively walks a json.RawMessage, calling visitor
// for every string leaf with its JSON pointer path and contextual metadata.
func WalkStringsWithContext(raw json.RawMessage, basePath string, visitor ContextStringVisitor) {
	if len(raw) == 0 {
		return
	}

	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return
	}
	walkValueWithContext(v, basePath, visitor)
}

func walkValueWithContext(v interface{}, path string, visitor ContextStringVisitor) {
	switch val := v.(type) {
	case map[string]interface{}:
		for k, child := range val {
			walkValueWithContext(child, path+"/"+k, visitor)
		}
	case []interface{}:
		for i, child := range val {
			walkValueWithContext(child, path+"/"+strconv.Itoa(i), visitor)
		}
	case string:
		visitor(StringContext{
			Pointer:   path,
			Value:     val,
			IsCondLHS: condLHSPattern.MatchString(path),
		})
	}
}
