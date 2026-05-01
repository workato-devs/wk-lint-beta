package lint

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/workato-devs/wk-lint-beta/pkg/recipe"
)

// evalCustomRules evaluates all custom rules for the given tier and returns diagnostics.
func evalCustomRules(ctx *BuiltinContext, rules []CustomRule, tier int) []LintDiagnostic {
	var diags []LintDiagnostic
	for _, rule := range rules {
		if rule.Tier != tier {
			continue
		}
		if rule.Assert.Builtin != nil {
			diags = append(diags, evalBuiltinRule(ctx, rule)...)
			continue
		}
		switch rule.Scope {
		case "recipe":
			diags = append(diags, evalRecipeScope(ctx.Parsed, rule)...)
		case "step":
			diags = append(diags, evalStepScope(ctx.Parsed, rule)...)
		}
	}
	return diags
}

// evalBuiltinRule dispatches to a registered Go function.
func evalBuiltinRule(ctx *BuiltinContext, rule CustomRule) []LintDiagnostic {
	name := *rule.Assert.Builtin
	fn, ok := builtinRegistry[name]
	if !ok {
		return []LintDiagnostic{{
			Level:   LevelWarn,
			RuleID:  "BUILTIN_NOT_FOUND",
			Message: fmt.Sprintf("builtin %q not found in registry", name),
			Tier:    rule.Tier,
		}}
	}
	rawDiags := fn(ctx, &rule)
	for i := range rawDiags {
		if rawDiags[i].RuleID == "" {
			rawDiags[i].RuleID = rule.RuleID
		}
		if rawDiags[i].Level == "" {
			rawDiags[i].Level = rule.Level
		}
		if rawDiags[i].SuggestedFix == "" {
			rawDiags[i].SuggestedFix = rule.SuggestedFix
		}
		rawDiags[i].Tier = rule.Tier
	}
	return rawDiags
}

func evalRecipeScope(parsed *recipe.ParsedRecipe, rule CustomRule) []LintDiagnostic {
	if !evalAssertion(nil, parsed, rule.Assert) {
		return []LintDiagnostic{{
			Level:        rule.Level,
			Message:      rule.Message,
			RuleID:       rule.RuleID,
			Tier:         rule.Tier,
			SuggestedFix: rule.SuggestedFix,
		}}
	}
	return nil
}

func evalStepScope(parsed *recipe.ParsedRecipe, rule CustomRule) []LintDiagnostic {
	var diags []LintDiagnostic
	for i := range parsed.Steps {
		step := &parsed.Steps[i]
		if rule.Where != nil && !matchesWhere(step, rule.Where) {
			continue
		}
		if !evalAssertion(step, parsed, rule.Assert) {
			diags = append(diags, LintDiagnostic{
				Level:        rule.Level,
				Message:      rule.Message,
				Source:       &SourceRef{JSONPointer: step.JSONPointer},
				RuleID:       rule.RuleID,
				Tier:         rule.Tier,
				SuggestedFix: rule.SuggestedFix,
			})
		}
	}
	return diags
}

// evalAssertion returns true if the assertion passes.
func evalAssertion(step *recipe.FlatStep, parsed *recipe.ParsedRecipe, a Assertion) bool {
	switch {
	case a.FieldExists != nil:
		_, ok := resolveFieldPath(step, a.FieldExists.Path)
		return ok

	case a.FieldAbsent != nil:
		_, ok := resolveFieldPath(step, a.FieldAbsent.Path)
		return !ok

	case a.FieldMatches != nil:
		return evalFieldMatches(step, a.FieldMatches)

	case a.FieldEquals != nil:
		return evalFieldEquals(step, a.FieldEquals)

	case a.StepCount != nil:
		return evalStepCount(parsed, a.StepCount)

	case a.EISEmpty != nil && *a.EISEmpty:
		return evalEISEmpty(step)

	case a.EISFieldType != nil:
		return evalEISFieldType(step, a.EISFieldType)

	case len(a.AllOf) > 0:
		for _, sub := range a.AllOf {
			if !evalAssertion(step, parsed, sub) {
				return false
			}
		}
		return true

	case len(a.AnyOf) > 0:
		for _, sub := range a.AnyOf {
			if evalAssertion(step, parsed, sub) {
				return true
			}
		}
		return false

	case a.Not != nil:
		return !evalAssertion(step, parsed, *a.Not)

	default:
		return false
	}
}

func evalFieldMatches(step *recipe.FlatStep, a *AssertFieldMatches) bool {
	val, ok := resolveFieldPath(step, a.Path)
	if !ok {
		return false
	}
	s, ok := val.(string)
	if !ok {
		s = fmt.Sprintf("%v", val)
	}
	re, err := regexp.Compile(a.Pattern)
	if err != nil {
		return false
	}
	return re.MatchString(s)
}

func evalFieldEquals(step *recipe.FlatStep, a *AssertFieldEquals) bool {
	val, ok := resolveFieldPath(step, a.Path)
	if !ok {
		return false
	}
	return fmt.Sprintf("%v", val) == fmt.Sprintf("%v", a.Value)
}

func evalStepCount(parsed *recipe.ParsedRecipe, a *AssertStepCount) bool {
	count := 0
	for i := range parsed.Steps {
		step := &parsed.Steps[i]
		if a.Where != nil && !matchesWhere(step, a.Where) {
			continue
		}
		count++
	}
	if a.Exact != nil && count != *a.Exact {
		return false
	}
	if a.Min != nil && count < *a.Min {
		return false
	}
	if a.Max != nil && count > *a.Max {
		return false
	}
	return true
}

func evalEISEmpty(step *recipe.FlatStep) bool {
	if step == nil {
		return true
	}
	raw := step.Code.ExtendedInputSchema
	if len(raw) == 0 {
		return true
	}
	trimmed := strings.TrimSpace(string(raw))
	return trimmed == "null" || trimmed == "[]"
}

func evalEISFieldType(step *recipe.FlatStep, a *AssertEISFieldType) bool {
	if step == nil {
		return false
	}
	fields, err := parseEIS(step.Code.ExtendedInputSchema)
	if err != nil {
		return false
	}
	for _, f := range fields {
		if f.Name != a.Name {
			continue
		}
		if a.Type != "" && f.Type != a.Type {
			return false
		}
		if a.ParseOutput != "" && f.ParseOutput != a.ParseOutput {
			return false
		}
		return true
	}
	return false
}

// matchesWhere returns true if a step matches the selector criteria.
func matchesWhere(step *recipe.FlatStep, where *StepSelector) bool {
	if where == nil {
		return true
	}
	if len(where.Keyword) > 0 && !where.Keyword.Contains(step.Code.Keyword) {
		return false
	}
	if len(where.Provider) > 0 {
		if step.Code.Provider == nil || !where.Provider.Contains(*step.Code.Provider) {
			return false
		}
	}
	if len(where.ActionName) > 0 && !where.ActionName.Contains(step.Code.Name) {
		return false
	}
	return true
}

// resolveFieldPath resolves a dotted path to a value on a step.
// Top-level segments map to step fields; remaining segments navigate into raw JSON.
func resolveFieldPath(step *recipe.FlatStep, path string) (interface{}, bool) {
	if step == nil || path == "" {
		return nil, false
	}

	parts := strings.SplitN(path, ".", 2)
	head := parts[0]
	tail := ""
	if len(parts) > 1 {
		tail = parts[1]
	}

	var raw json.RawMessage
	switch head {
	case "uuid":
		if tail != "" {
			return nil, false
		}
		return step.Code.UUID, step.Code.UUID != ""
	case "name":
		if tail != "" {
			return nil, false
		}
		return step.Code.Name, step.Code.Name != ""
	case "keyword":
		if tail != "" {
			return nil, false
		}
		return step.Code.Keyword, step.Code.Keyword != ""
	case "provider":
		if tail != "" {
			return nil, false
		}
		if step.Code.Provider == nil {
			return nil, false
		}
		return *step.Code.Provider, true
	case "as":
		if tail != "" {
			return nil, false
		}
		return step.Code.As, step.Code.As != ""
	case "input":
		raw = step.Code.Input
	case "extended_input_schema":
		raw = step.Code.ExtendedInputSchema
	case "extended_output_schema":
		raw = step.Code.ExtendedOutputSchema
	case "dynamicPickListSelection":
		raw = step.Code.DynamicPickListSelection
	default:
		return nil, false
	}

	if len(raw) == 0 {
		return nil, false
	}

	if tail == "" {
		var v interface{}
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, false
		}
		return v, true
	}

	return navigateJSON(raw, tail)
}

func navigateJSON(raw json.RawMessage, path string) (interface{}, bool) {
	parts := strings.SplitN(path, ".", 2)
	head := parts[0]
	tail := ""
	if len(parts) > 1 {
		tail = parts[1]
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, false
	}

	child, ok := m[head]
	if !ok {
		return nil, false
	}

	if tail == "" {
		var v interface{}
		if err := json.Unmarshal(child, &v); err != nil {
			return nil, false
		}
		return v, true
	}

	return navigateJSON(child, tail)
}

func expandMessage(template string, vars map[string]string) string {
	result := template
	for k, v := range vars {
		result = strings.ReplaceAll(result, "{"+k+"}", v)
	}
	return result
}
