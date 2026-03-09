# Formula Method Validation

**Author:** Zayne Turner
**Status:** Implemented
**Date:** March 6, 2026
**Implemented:** March 9, 2026
**References:** ADR 0001 (Tiered Lint Architecture)

---

## Context

Testing identified invalid formula method patterns in recipe `input` fields. For example, `now.utc` is a Ruby method that does not exist in Workato's formula language; the correct equivalent is `now.in_time_zone("UTC")`.

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

`FORMULA_FORBIDDEN_PATTERN` fires first and provides targeted suggestions (e.g., "use `in_time_zone` instead of `.utc`"). Methods already covered by a forbidden pattern are suppressed from `FORMULA_METHOD_INVALID` to avoid duplicate noise. Both rules emit at `LevelWarn`, Tier 1.

### Rule Category Placement

Formula method validation is a sub-category of Tier 1b. It uses the generic string-walking infrastructure (`WalkStrings` from `pkg/recipe/walk.go`) but is wired as an independent check function in `lintTier1Steps`, consistent with every other Tier 1 rule. Formula rules use a separate registry and produce distinct rule IDs prefixed with `FORMULA_`.

### Method extraction: character-level scanner

Formula method validation requires extracting method calls from formula strings. Workato formulas are `=`-prefixed strings with Ruby-like method chaining:

```
=_dp('{...}').strip.downcase
=now.in_time_zone("UTC").strftime('%Y-%m-%d')
=items.where(status: 'active').pluck('name').join(', ')
```

A naive split-on-dot approach would false-positive on dots inside `_dp('{a.b.c}')` datapill payloads and string literals. The extractor is a character-level scanner that:

1. Strips the leading `=`
2. Skips `_dp('...')` datapill payloads (which contain dots that aren't method calls)
3. Skips `'...'` string literals
4. Skips `(...)` parenthesized arguments with depth tracking
5. On `.`, reads the subsequent identifier (`[a-z_][a-z0-9_]*\??`) as a method name

```go
func extractMethods(formula string) []string
```

Edge cases handled:
- `_dp('{a.b.c}')` — dots inside datapills are not method calls
- `'text.with.dots'` — dots inside string literals are not method calls
- `present?` — trailing `?` is part of the method name
- Nested parentheses — `gsub(/[^0-9]/, '').to_i` correctly skips the regex argument
- Ternary `? :` — not confused with `?`-suffix methods

### Independent checks, not shared walker callbacks

The original design envisioned formula and datapill validation as callbacks within a single shared `walkAllStrings` traversal. The implementation instead has each check own its own `WalkStrings` call, consistent with how all other Tier 1 checks work.

This is the right tradeoff: walking JSON strings is cheap (small payloads, no I/O), and coupling unrelated checks into a shared traversal adds complexity for negligible performance benefit. Each check is self-contained and independently testable. When the datapill walker is added, it should also call `WalkStrings` independently rather than sharing a traversal with formula validation.

## Implementation

### Delivered files

| File | Description |
|---|---|
| `pkg/recipe/walk.go` | Generic `WalkStrings` — recursively walks `json.RawMessage`, calls visitor for every string leaf with JSON pointer path |
| `pkg/recipe/walk_test.go` | Walker tests: flat objects, nested objects, arrays, mixed types, nil/empty |
| `pkg/lint/formulas.json` | Allowlist data file (embedded via `//go:embed`) |
| `pkg/lint/tier1_formulas.go` | Allowlist loader (`init()`), `extractMethods` tokenizer, `lintFormulaString`, `checkFormulaMethods` |
| `pkg/lint/tier1_formulas_test.go` | Tokenizer tests (12 cases), validation tests (6 cases), integration + fixture regression tests |
| `pkg/lint/tier1_steps.go` | One-line wiring: `checkFormulaMethods(parsed)` added to `lintTier1Steps` |

### Skill update (separate repo)

The recipe skill (`recipe-skills/`) should be updated to:
1. Reference the formula allowlist as a constraint, not just document gotchas
2. Include a "Supported Formulas" reference section (generated from or linking to `formulas.json`)
3. Remove the permissive "use any valid Workato formula" framing

This is a skill content change, not a linter change, tracked separately.

## Consequences

- **Systematic coverage:** Any invalid method is caught, not just known-bad patterns. Novel hallucinations from Ruby training data are caught on first occurrence.
- **Defense in depth:** Skill constrains generation + linter enforces allowlist + activation is last resort. Invalid formulas should rarely reach Workato.
- **Stable maintenance:** The allowlist (~120 methods) changes only when Workato adds or removes formula support. This is infrequent and documented in their changelog.
- **Specific suggestions preserved:** The `forbidden_patterns` list provides targeted error messages for common mistakes (e.g., `now.utc`), layered on top of the generic allowlist check. Forbidden pattern matches suppress duplicate allowlist findings for the same methods.
- **Shared source of truth:** `formulas.json` is used by both the linter and the skill, preventing drift between what's taught and what's enforced.
- **Scanner complexity:** The method extractor is a character-level scanner, not a simple string split. This is necessary to correctly handle `_dp()` payloads, string literals, and nested parentheses without false positives. The complexity is justified and well-covered by tests (~12 tokenizer cases).
- **Reusable walker:** `pkg/recipe/walk.go` provides `WalkStrings` as generic infrastructure. Future checks (datapill validation, EIS checking) can use it independently without coupling to formula validation.
