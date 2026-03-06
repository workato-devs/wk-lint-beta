# Formula Method Validation

**Author:** Zayne Turner
**Status:** Draft
**Date:** March 6, 2026
**References:** ADR 0001 (Tiered Lint Architecture)

---

## Context

Testing identified invalid formula method patterns in recipe `input` fields (see `docs/TODO.md`, Rule 2). For example, `now.utc` is a Ruby method that does not exist in Workato's formula language; the correct equivalent is `now.in_time_zone("UTC")`.

This is architecturally distinct from datapill validation (Tier 1b): datapill rules validate `_dp()` reference correctness, while formula method validation checks that Ruby/Workato method calls within string values are correct. Both require scanning string values in `input` fields, but the concerns and pattern registries are separate.

ADR 0001 does not cover formula method validation in any tier.

### Why this matters more than it looks

Invalid formulas are **activation blockers** — they cause hard failures at Workato import time, not warnings. The current recipe skill takes a permissive approach ("use any valid Workato formula") with documented exceptions for known gotchas. But agents draw from Ruby training data, and the universe of invalid Ruby methods that don't exist in Workato is unbounded. A denylist of known-bad patterns can only catch mistakes we've already seen.

Workato's formula language is an allowlist of ~120 Ruby methods across 5 categories (string, number, date, array, complex). This is a bounded, documented set. The linter should validate against this allowlist, and the skill should constrain generation to it.

## Decision

### Allowlist, not denylist

The linter validates formula method calls against the complete allowlist of supported Workato formula methods (~120 methods). This is the opposite of the original denylist approach (regex patterns for known-bad methods).

**Why allowlist wins:**

| | Denylist | Allowlist |
|---|---|---|
| Catches `now.utc` | Yes | Yes |
| Catches `string.chomp` (valid Ruby, invalid Workato) | Only if we add it | Yes |
| Catches novel hallucinated methods | No | Yes |
| Maintenance burden | Grows with every new mistake | Stable (~120 methods, changes rarely) |
| False negatives | Unbounded | None (by definition) |

The denylist ceiling is low. As soon as you want systematic coverage rather than spot-checking, you need the allowlist. Since invalid formulas block activation, systematic coverage is the right bar.

### Shared source of truth: `formulas.json`

The allowlist lives in `pkg/lint/formulas.json` — a single data file that serves both the linter (machine-readable validation) and the recipe skill (human-readable reference). The file contains:

- **`methods`**: categorized allowlist of all valid Workato formula methods (string, number, date, array)
- **`forbidden_patterns`**: high-signal patterns that deserve specific error messages (e.g., `now.utc` with a suggestion to use `in_time_zone`)
- **`version`** and **`source`**: traceability back to Workato docs

The recipe skill should reference or embed this allowlist so agents generate only valid formula methods. The linter enforces the same list. Defense in depth: constrain at generation time, validate at lint time, activation check is last resort.

### Rule IDs

| Rule ID | Check |
|---|---|
| `FORMULA_METHOD_INVALID` | Method call uses a method not in the Workato formula allowlist |
| `FORMULA_FORBIDDEN_PATTERN` | Value matches a known-forbidden pattern (with specific suggestion) |

`FORMULA_FORBIDDEN_PATTERN` fires first and provides targeted suggestions (e.g., "use `in_time_zone` instead of `.utc`"). `FORMULA_METHOD_INVALID` is the catch-all for any method not in the allowlist.

### Rule Category Placement

Formula method validation is a sub-category of Tier 1b, alongside the datapill walker. Both share the string-walking infrastructure (`walkAllStrings` from `pkg/recipe/walk.go`), but formula method rules use a separate registry and produce distinct rule IDs prefixed with `FORMULA_`.

### Method extraction: simple tokenizer, not full parser

Formula method validation requires extracting method calls from formula strings. Workato formulas are `=`-prefixed strings with Ruby-like method chaining:

```
=_dp('{...}').strip.downcase
=now.in_time_zone("UTC").strftime('%Y-%m-%d')
=items.where(status: 'active').pluck('name').join(', ')
```

The extractor needs to:
1. Identify `=`-prefixed strings (formula mode)
2. Extract `.method_name` calls (dotted identifiers)
3. Check each method name against the allowlist

This is a simple tokenizer (split on `.`, strip arguments), not a full expression parser. It doesn't need to understand operator precedence, nesting, or evaluation — just extract the method names being called.

```go
// extractMethods returns all .method_name calls from a formula string.
// Input: "=_dp('{...}').strip.downcase"
// Output: ["strip", "downcase"]
func extractMethods(formula string) []string
```

Edge cases to handle:
- `_dp(...)` is not a method call (it's a datapill reference, handled by Tier 1b)
- Bracket notation `['key']` is not a method call
- Ternary `? :` operators are not method calls
- Arguments in parentheses `strftime('%Y')` — extract `strftime`, ignore args

### Relationship to Tier 1b Walker

Formula method validation runs as a callback within the same `walkAllStrings` traversal used by the datapill walker:

```
walkAllStrings(recipe)
  -> for each string value:
     -> lintDatapills(value, ctx)      // Tier 1b datapill rules
     -> lintFormulaMethods(value, ctx)  // Tier 1b formula rules
```

This avoids duplicate traversal while keeping the rule implementations independent.

## Implementation

### Phase 2 Deliverables

| Deliverable | Description |
|---|---|
| `pkg/lint/formulas.json` | Allowlist data file (created) |
| `pkg/lint/tier1_formulas.go` | Allowlist loader, method extractor, `lintFormulaMethods()` |
| `pkg/lint/tier1_formulas_test.go` | Test cases for extraction + validation |
| Integration with `walkAllStrings` | Callback registration in `pkg/recipe/walk.go` |

### Skill update (separate repo)

The recipe skill (`recipe-skills/`) should be updated to:
1. Reference the formula allowlist as a constraint, not just document gotchas
2. Include a "Supported Formulas" reference section (generated from or linking to `formulas.json`)
3. Remove the permissive "use any valid Workato formula" framing

This is a skill content change, not a linter change, tracked separately.

### Dependencies

- **Blocks on:** `pkg/recipe/walk.go` (the generic string walker, planned for Phase 2)
- **Does not block:** any other Phase 1 or Phase 2 work

## Consequences

- **Systematic coverage:** Any invalid method is caught, not just known-bad patterns. Novel hallucinations from Ruby training data are caught on first occurrence.
- **Defense in depth:** Skill constrains generation + linter enforces allowlist + activation is last resort. Invalid formulas should rarely reach Workato.
- **Stable maintenance:** The allowlist (~120 methods) changes only when Workato adds or removes formula support. This is infrequent and documented in their changelog.
- **Specific suggestions preserved:** The `forbidden_patterns` list provides targeted error messages for common mistakes (e.g., `now.utc`), layered on top of the generic allowlist check.
- **Shared source of truth:** `formulas.json` is used by both the linter and the skill, preventing drift between what's taught and what's enforced.
- **Tokenizer complexity:** The method extractor is simple but not trivial — edge cases around `_dp()`, bracket notation, and ternary operators need test coverage.
