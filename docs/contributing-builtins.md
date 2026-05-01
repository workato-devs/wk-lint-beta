# Contributing Builtin Rules

This guide covers how to add a new builtin rule — one that requires Go code because it can't be expressed with the declarative JSON assertion matchers.

Before writing a builtin, check if you can express the rule with the [declarative assertion matchers](rule-authoring.md#assertion-matchers). Declarative rules are easier to maintain and don't require recompiling the binary.

## When you need a builtin

Use a builtin when the rule requires:
- Iteration across multiple steps with cross-references (e.g., checking that step UUIDs are unique)
- Access to the IGM control flow graph (Tier 2-3 analysis)
- Complex string parsing (e.g., datapill syntax, formula methods)
- Connector rule data from `lint-rules.json` files

## File conventions

| File | Purpose |
|------|---------|
| `pkg/lint/builtin_tier1.go` | Tier 1 builtin registrations and implementations |
| `pkg/lint/builtin_tier2.go` | Tier 2 builtin registrations (requires IGM graph) |
| `pkg/lint/builtin_tier3.go` | Tier 3 builtin registrations (requires IGM graph + alias map) |
| `pkg/lint/tier1_*.go` | Tier 1 implementation files grouped by concern (steps, datapills, EIS, formulas) |
| `pkg/lint/tier2_structure.go` | Tier 2 control flow analysis |
| `pkg/lint/tier3_dataflow.go` | Tier 3 cross-step data flow analysis |
| `pkg/lint/builtin_rules.json` | JSON catalog — every rule (builtin or declarative) must have an entry here |

## Steps to add a new builtin rule

### 1. Add the JSON rule entry

Add an entry to `pkg/lint/builtin_rules.json`:

```json
{
  "rule_id": "MY_NEW_RULE",
  "tier": 1,
  "level": "warn",
  "message": "Description of what went wrong",
  "suggested_fix": "How to fix it.",
  "scope": "recipe",
  "assert": { "builtin": "check_my_new_rule" }
}
```

The `builtin` value is the registry key — it must match the name you pass to `RegisterBuiltin`.

### 2. Register the builtin function

In the appropriate `builtin_tier*.go` file, add a registration in the `init()` function:

```go
func init() {
    RegisterBuiltin("check_my_new_rule", func(ctx *BuiltinContext, rule *CustomRule) []LintDiagnostic {
        return checkMyNewRule(ctx.Parsed)
    })
}
```

### 3. Implement the check

Create or extend an implementation file. Your function receives a `BuiltinContext` and returns diagnostics:

```go
func checkMyNewRule(parsed *recipe.ParsedRecipe) []LintDiagnostic {
    var diags []LintDiagnostic
    for _, step := range parsed.Steps {
        if step.Code.Keyword != "action" {
            continue
        }
        // Your validation logic here
        if somethingIsWrong {
            diags = append(diags, LintDiagnostic{
                Message: fmt.Sprintf("Specific message about step %q", step.Code.UUID),
                Source:  &SourceRef{JSONPointer: step.JSONPointer},
            })
        }
    }
    return diags
}
```

The engine stamps `RuleID`, `Level`, `Tier`, and `SuggestedFix` from the JSON rule definition, so you only need to set `Message` and `Source` in your diagnostics.

### 4. Add the rule to the standard profile

Add the rule ID and default severity to `profiles/standard.json`. If the rule should also be escalated in strict mode, add it to `profiles/strict.json`.

### 5. Write tests

Add pass/fail test cases in the corresponding test file (e.g., `pkg/lint/tier1_steps_test.go`):

```go
func TestBuiltinRule_MyNewRule_Pass(t *testing.T) {
    parsed := buildParsedRecipe("test", []recipe.FlatStep{
        {Code: recipe.Code{Keyword: "action", UUID: "valid-step"}, JSONPointer: "/code/block/0"},
    }, nil)
    diags := evalBuiltinRulesForTest(t, parsed)
    if hasDiag(diags, "MY_NEW_RULE") {
        t.Error("expected no MY_NEW_RULE for valid step")
    }
}

func TestBuiltinRule_MyNewRule_Fail(t *testing.T) {
    parsed := buildParsedRecipe("test", []recipe.FlatStep{
        {Code: recipe.Code{Keyword: "action", UUID: "bad-step"}, JSONPointer: "/code/block/0"},
    }, nil)
    diags := evalBuiltinRulesForTest(t, parsed)
    if !hasDiag(diags, "MY_NEW_RULE") {
        t.Error("expected MY_NEW_RULE for invalid step")
    }
}
```

The `buildParsedRecipe` and `hasDiag` helpers are defined in `eval_test.go`.

### 6. Run the completeness checks

```bash
go test ./pkg/lint/ -run TestBuiltinRules_Load -v
go test ./pkg/lint/ -run TestBuiltinRegistry_Completeness -v
go test ./pkg/lint/ -run TestBuiltinRules_ProfileCompleteness -v
```

These tests verify that:
- Your rule ID exists in `builtin_rules.json`
- The builtin function name is registered in the registry
- The rule ID has an entry in the standard profile

## Multi-rule builtins

Some builtins back multiple rule IDs (e.g., `check_datapills` backs 7 rules). The pattern uses `CacheGetOrCompute` to run the analysis once and filter results by the current rule's ID:

```go
func init() {
    RegisterBuiltin("check_my_analysis", func(ctx *BuiltinContext, rule *CustomRule) []LintDiagnostic {
        all := ctx.CacheGetOrCompute("my_analysis", func() interface{} {
            return runFullAnalysis(ctx.Parsed)
        }).([]LintDiagnostic)

        var filtered []LintDiagnostic
        for _, d := range all {
            if d.RuleID == rule.RuleID {
                filtered = append(filtered, d)
            }
        }
        return filtered
    })
}
```

Each diagnostic must set its own `RuleID` so the filter works. The engine only stamps `RuleID` if the diagnostic leaves it empty.

## Tier 2-3 builtins

Tier 2-3 builtins require the IGM graph. Check for nil before using it:

```go
func checkMyGraphRule(ctx *BuiltinContext) []LintDiagnostic {
    if ctx.Graph == nil {
        return nil
    }
    // Use ctx.Graph.Nodes, ctx.Graph.Edges, ctx.Graph.AliasMap, etc.
}
```

The graph is only built when Tier 2 or 3 is requested. If the graph build fails, your function won't be called (the context's Graph field will be nil and the tier is skipped).
