package lint

import (
	"github.com/workato-devs/wk-lint-beta/pkg/igm"
	"github.com/workato-devs/wk-lint-beta/pkg/recipe"
)

// BuiltinContext provides everything a builtin rule function might need.
type BuiltinContext struct {
	Parsed    *recipe.ParsedRecipe
	Graph     *igm.Graph
	ConnRules map[string]*ConnectorRules
	Filename  string
	cache     map[string]interface{}
}

// CacheGetOrCompute returns a cached value for key, computing it on first access.
func (c *BuiltinContext) CacheGetOrCompute(key string, compute func() interface{}) interface{} {
	if c.cache == nil {
		c.cache = make(map[string]interface{})
	}
	if v, ok := c.cache[key]; ok {
		return v
	}
	v := compute()
	c.cache[key] = v
	return v
}

// BuiltinFunc is a registered Go function backing a builtin assertion.
// It returns zero or more diagnostics with Message and Source filled in.
// The engine stamps RuleID, Level, and Tier from the JSON rule definition.
type BuiltinFunc func(ctx *BuiltinContext, rule *CustomRule) []LintDiagnostic

var builtinRegistry = map[string]BuiltinFunc{}

func init() {
	RegisterBuiltin("__tier0__", func(ctx *BuiltinContext, rule *CustomRule) []LintDiagnostic {
		return nil
	})
}

// RegisterBuiltin adds a named builtin function to the registry.
func RegisterBuiltin(name string, fn BuiltinFunc) {
	builtinRegistry[name] = fn
}
