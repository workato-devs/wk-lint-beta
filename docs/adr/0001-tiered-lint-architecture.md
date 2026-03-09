# Recipe Linter: Tiered Validation for Workato Recipe JSON

**Author:** Zayne Turner
**Status:** Accepted — Partially Implemented (Tiers 0 + 1a)
**Date:** February 23, 2026 (original) · March 2, 2026 (amendments incorporated)

---

## What Is It?

A tiered validation pipeline for Workato recipe JSON, shipped as a `wk` CLI plugin (`recipe-lint`) that registers the `wk lint` command and a `pre-push` hook. It replaces the current approach — agents reading prose checklists and self-validating — with deterministic, machine-executable lint rules that emit structured diagnostics. The core linter library lives in `pkg/lint/` (importable by the plugin, CI tools, and future commands). No runtime dependencies beyond the `wk` binary and the installed plugin, sub-100ms execution.

## Why Should I Care?

The v5 IGM experiment revealed that agents using recipe-skills produce structurally sound recipes (Q1-Q8 pass rates near 100%), but **cannot reliably validate their own output against the skills' validation checklists.** The failure mode is false negatives — the agent says "looks good" when it isn't. This isn't a skills content problem; the checklists are thorough. It's a representation problem: agents can't hold 30+ checklist items in working memory while scanning 50-90KB of deeply nested JSON.

The existing IGM transformer (`recipe-visualizer/claude/core/transformer.ts`) already catches some structural issues — missing `code` field, unknown keywords, multiple catch blocks. But its diagnostic coverage is incidental to its primary job (graph construction), and it's TypeScript locked inside a VS Code extension. A Go-native linter compiled into a CLI plugin puts validation where it needs to be: in CI pipelines, in `wk push` pre-flight, and in agent validation loops — all without requiring Node.js.

---

## Language Decision: Go, Not TypeScript

The linter is Go, matching the `wk` CLI. This is a deliberate choice.

**Why not TypeScript (the IGM transformer's language)?**

The `wk` CLI's entire value proposition is "single binary, no runtime dependencies, installs in seconds." A TypeScript linter requires Node.js — either as a runtime dependency for `wk lint` or as a sidecar process over JSON-RPC. Both violate the CLI's install story. Node.js cold start (~500ms-1s) matters when linting 50 recipes in CI. And the connector-specific extensibility model uses JSON data files, not code — any language can load them.

**What about the IGM dependency?**

Tiers 0-1 (schema validation, step-level rules, datapill syntax, EIS checking) have zero IGM dependency. These are pure JSON tree walking and pattern matching — the highest-value rules and the ones that catch the false negatives we care about most. They ship first.

Tiers 2-3 (structural assertions, data flow resolution) consume the IGM graph. When these tiers are needed, the IGM transformer gets ported to Go as `pkg/igm/` in the CLI monorepo. The transformer is ~800 lines of recursive tree walking — straightforward to port. The TypeScript version stays in `recipe-visualizer` for the VS Code extension. The IGM v1.2 spec is the shared contract; both implementations produce conformant graphs, validated by snapshot tests against the same fixtures.

---

## Plugin Architecture: Linter Ships as a Plugin from Day 1

The linter is a plugin, not a built-in command. Users install it via `wk plugins install`. It registers the `wk lint` command and a `pre-push` hook via its `plugin.toml` manifest.

### Why plugin-first?

The question that forced this decision: *"If we release new linting rules, do we have to release a new CLI?"* With a built-in linter, the answer is yes — every rule is compiled into the binary. With a plugin, the answer is `wk plugins update recipe-lint`. Independent versioning, independent release cadence.

This also validates the plugin system with a real, complex plugin (not just the example `hello` command), and it sets up the A → C evolution path for external rule plugins naturally — the linter plugin *is* the first rule plugin.

### Plugin manifest

```toml
name = "recipe-lint"
version = "0.1.0"
description = "Tiered validation for Workato recipe JSON"
entrypoint = "./recipe-lint"

[[commands]]
name = "lint"
description = "Lint Workato recipe JSON files"
method = "lint.run"

  [[commands.subcommands]]
  name = "validate"
  description = "Alias for lint (wk recipes validate compatibility)"
  method = "lint.run"

[hooks]
pre-push = "lint.pre_push"
```

### Process overhead

Every `wk lint` invocation spawns the plugin binary (~10-20ms Go process startup). For a single recipe, this is negligible. For `wk lint ./recipes/` with 50 recipes, the plugin is spawned once and processes all files in a single RPC call — not 50 spawns. The `pre-push` hook is a single RPC call with the full file list.

### Installation and distribution

Phase 1 ships the plugin in the `wk-cli-beta` repo at `plugins/recipe-lint/`. Users install from a local build:

```bash
cd plugins/recipe-lint && go build -o recipe-lint . && cd ../..
wk plugins install ./plugins/recipe-lint
```

Future: GoReleaser builds the plugin binary alongside the CLI binary. `wk plugins install recipe-lint` fetches from a release URL (GitHub Releases or a plugin registry). This is a distribution concern, not an architecture concern — it can be solved later without changing the plugin interface.

---

## Tiered Architecture

Validation rules fall into four tiers based on what context they need to evaluate. Each tier uses the cheapest sufficient tool for its class of rules.

### Tier 0: Schema Validation

**What it checks:** Is this syntactically valid recipe JSON with the right shape?

**Tool:** Go struct validation (recipe JSON is a known shape — hand-rolled validation against typed structs is more precise than generic JSON Schema)

**What it catches:**

- Missing required top-level keys (`name`, `version`, `private`, `concurrency`, `code`, `config`)
- `code` is an object, not an array
- `code` is not wrapped in a `recipe` object
- Every step block has `keyword`, `number`, `uuid`
- `uuid` values are strings ≤ 36 characters
- `number` fields are integers
- `config` is an array of objects with `keyword: "application"`

**What it can't catch:** Anything requiring cross-reference between steps. Sequential numbering requires ordering context. UUID uniqueness requires a set. These graduate to Tier 1.

**Diagnostic examples:**
```
error | / | Missing required field: "code"
error | /code | "code" must be an object, got array
error | /code/block/3 | Step missing required field: "uuid"
error | /code/block/3 | UUID exceeds 36 characters: "this-is-a-really-long-uuid-that-will-..."
```

### Tier 1: Intra-Step Validation

**What it checks:** Is each step internally correct, without needing to know about other steps?

**Tool:** Go tree walker that visits every step and every string value within steps.

This is the tier where the **false negatives live**. Datapill syntax rules, `lhs` formula mode violations, `extended_input_schema` completeness — these all require parsing field values within a single step and knowing the context they appear in.

#### 1a: Step-Level Rules

| Rule ID | Check | Source |
|---------|-------|--------|
| `STEP_NUMBERING` | Action `number` fields are sequential (0, 1, 2, 3...) with no gaps | base validation-checklist |
| `UUID_UNIQUE` | All `uuid` values unique within the recipe | base validation-checklist |
| `UUID_DESCRIPTIVE` | UUID matches `/{a-z}+-{a-z}+-\d{3}/` pattern (warn, not error) | SKILL_INSTRUCTIONS |
| `TRIGGER_NUMBER_ZERO` | Trigger has `number: 0` | base validation-checklist |
| `FILENAME_MATCH` | Filename matches `name` field (lowercase, underscores) | SKILL_INSTRUCTIONS |
| `CONFIG_NO_WORKATO` | `config` array does not include `workato` provider | base validation-checklist |
| `CONFIG_PROVIDER_MATCH` | Every `provider` used in actions has a matching `config` entry | base validation-checklist |
| `ACTION_NAME_VALID` | Action `name` is in the allowed set for its `provider` (via `valid_action_names` in connector rules) | TODO Rule 1 |
| `CONFIG_NOT_IN_ACTION` | No action block contains a `config` key | SKILL_INSTRUCTIONS |
| `IF_NO_PROVIDER` | `if` blocks have no `provider` field | if-else.md |
| `ELSE_NO_PROVIDER` | `else` blocks have no `provider` field | if-else.md |
| `CATCH_PROVIDER_NULL` | `catch` blocks have `"provider": null` (explicit null) | try-catch.md |
| `TRY_NO_AS` | `try` blocks have no `as` field | try-catch.md |
| `CATCH_HAS_AS` | `catch` blocks have an `as` field | try-catch.md |
| `CATCH_HAS_RETRY` | `catch` `input` has `max_retry_count` and `retry_interval` | try-catch.md |
| `NO_ELSIF` | No step uses `keyword: "elsif"` | if-else.md |
| `RESPONSE_CODES_DEFINED` | For API endpoint triggers, all `return_response` status codes appear in trigger `responses` | api-endpoint.md |
| `PICK_LIST_MATCH` | `extended_input_schema` `pick_list` on `return_response` matches trigger response names exactly | SKILL_INSTRUCTIONS |

#### 1b: Datapill Validation (the hard part)

A dedicated sub-linter that walks every string value in the recipe, extracts `_dp()` calls, and validates them in context.

**Datapill regex validation requirement:** Before the extraction pattern `_dp\('(\{[^']+\})'\)` is committed, it must be validated against two recipe corpora (dewy-resort + a second repository). Any delta between the proposed regex and a broad `_dp(...)` pattern means the regex is missing datapills and must be revised.

| Rule ID | Check | Source |
|---------|-------|--------|
| `DP_VALID_JSON` | `_dp()` payload is parseable JSON after unescaping | base validation-checklist |
| `DP_LHS_NO_FORMULA` | In `if` condition `lhs` fields: no `=` prefix, only `#{_dp(...)}` interpolation | if-else.md, datapill-syntax.md |
| `DP_INTERPOLATION_SINGLE` | Single datapill uses `#{_dp(...)}` not `=_dp(...)` | datapill-syntax.md |
| `DP_FORMULA_CONCAT` | Multiple datapills use `=` prefix with `+` operator, no `#{}` wrapper | datapill-syntax.md |
| `DP_NO_OUTER_PARENS` | Formula mode expressions don't use outer parentheses (except ternary) | datapill-syntax.md |
| `DP_NO_BODY_NATIVE` | Datapill paths for native connectors don't include `["body"]` | base validation-checklist |
| `DP_CATCH_PROVIDER` | Datapills referencing catch data use `"provider":"catch"`, not `"provider":null` | base validation-checklist |

**Implementation sketch — datapill walker:**

```go
// DatapillContext tracks where a string value appears in the recipe tree,
// giving the linter the context it needs for rules like DP_LHS_NO_FORMULA.
type DatapillContext struct {
    StepPointer    string // JSON pointer to the containing step
    FieldPath      string // e.g., "input.lhs", "input.response.email"
    IsConditionLHS bool   // true when inside conditions[].lhs
    StepKeyword    string // "if", "action", "try", etc.
    StepProvider   string // provider of the containing step
}

var dpPattern = regexp.MustCompile(`_dp\('(\{[^']+\})'\)`)

func lintDatapills(value string, ctx DatapillContext) []LintDiagnostic {
    var diags []LintDiagnostic

    // Rule: DP_LHS_NO_FORMULA
    if ctx.IsConditionLHS && strings.HasPrefix(value, "=") {
        diags = append(diags, LintDiagnostic{
            Level:   "error",
            Message: `Condition lhs uses formula mode ("=") — strips app recognition ` +
                     `from child actions. Use #{_dp(...)} interpolation only.`,
            Source:  &SourceRef{JSONPointer: ctx.StepPointer},
            RuleID:  "DP_LHS_NO_FORMULA",
            Tier:    1,
        })
    }

    // Rule: DP_VALID_JSON — extract and parse all _dp() payloads
    matches := dpPattern.FindAllStringSubmatch(value, -1)
    for _, match := range matches {
        var payload map[string]interface{}
        if err := json.Unmarshal([]byte(match[1]), &payload); err != nil {
            diags = append(diags, LintDiagnostic{
                Level:   "error",
                Message: fmt.Sprintf("Unparseable datapill JSON in %s", ctx.FieldPath),
                Source:  &SourceRef{JSONPointer: ctx.StepPointer},
                RuleID:  "DP_VALID_JSON",
                Tier:    1,
            })
        }
    }

    // Rule: DP_INTERPOLATION_SINGLE
    if len(matches) == 1 &&
        !strings.Contains(value, ".present?") &&
        !strings.Contains(value, "+") &&
        strings.HasPrefix(value, "=_dp(") {
        diags = append(diags, LintDiagnostic{
            Level:   "warn",
            Message: "Single datapill should use #{_dp(...)} interpolation, " +
                     "not =_dp(...) formula mode",
            Source:  &SourceRef{JSONPointer: ctx.StepPointer},
            RuleID:  "DP_INTERPOLATION_SINGLE",
            Tier:    1,
        })
    }

    return diags
}
```

**Walking strategy:** Recursively visit every string value in `step.input` using `encoding/json` raw message walking, tracking the field path. When visiting `if` blocks, mark `conditions[].lhs` fields as `IsConditionLHS: true`. This gives the datapill linter the context it needs for rules like `DP_LHS_NO_FORMULA`.

#### 1c: Extended Input Schema Validation

| Rule ID | Check | Source |
|---------|-------|--------|
| `EIS_MIRRORS_INPUT` | Every field in `input` has a corresponding entry in `extended_input_schema` | base validation-checklist |
| `EIS_NESTED_MATCH` | Nested objects in `input` have matching nested `properties` in EIS | base validation-checklist |
| `EIS_NAME_MATCH` | Field names match exactly between `input` and EIS | base validation-checklist |
| `EIS_NO_CONNECTOR_INTERNAL` | Connector-internal fields (e.g., Salesforce `sobject_name`, `limit`) are NOT in EIS | SKILL_INSTRUCTIONS, salesforce validation-checklist |
| `EIS_OUTPUT_MIRRORS_INPUT` | `extended_output_schema` mirrors `extended_input_schema` for `return_response` actions | SKILL_INSTRUCTIONS |

The connector-internal exception list is loaded from JSON data files in the skills directory (see Connector-Specific Extensibility below).

### Tier 2: Inter-Step Structure (requires IGM)

**What it checks:** Are the relationships between steps correct?

**Tool:** Go IGM graph assertions. Generate the IGM via `pkg/igm/`, then run rules against the node/edge structure.

**Dependency:** Requires the IGM transformer ported to Go (see Implementation Sequencing).

| Rule ID | Check | Source |
|---------|-------|--------|
| `CATCH_LAST_IN_TRY` | Catch node is the last child step in its parent try block | try-catch.md |
| `ELSE_LAST_IN_IF` | Else node is the last child step in its parent if block | if-else.md |
| `SUCCESS_BEFORE_CATCH` | For API endpoint recipes with try/catch, the success `return_response` is inside the try block before the catch | try-catch.md |
| `TERMINAL_COVERAGE` | Every HTTP status code defined in trigger `responses` has at least one corresponding `return_response` action | api-endpoint.md |
| `ALL_PATHS_RETURN` | For API endpoint recipes, every control flow path terminates in a `return_response` (no dangling paths) | SKILL_INSTRUCTIONS |
| `CATCH_RETURNS_ALL_FIELDS` | Catch block `return_response` provides values for all fields defined in trigger response schema (using `=null` for unavailable) | base validation-checklist |
| `RECIPE_CALL_ZIP_NAME` | Recipe function `call` actions include `zip_name` in `flow_id` | SKILL_INSTRUCTIONS |

**Implementation sketch — IGM-based rules:**

```go
func lintStructure(graph *IgmGraph, recipe map[string]interface{}) []LintDiagnostic {
    var diags []LintDiagnostic

    // TERMINAL_COVERAGE: declared response codes vs actual terminal nodes
    declaredCodes := extractDeclaredResponseCodes(recipe)
    actualCodes := make(map[string]bool)
    for _, node := range graph.Nodes {
        if node.UI != nil && node.UI.IsTerminal && node.UI.HTTPStatus != "" {
            actualCodes[node.UI.HTTPStatus] = true
        }
    }
    for _, code := range declaredCodes {
        if !actualCodes[code] {
            diags = append(diags, LintDiagnostic{
                Level:   "warn",
                Message: fmt.Sprintf(
                    "Trigger declares response code %s but no return_response uses it", code),
                Source:  &SourceRef{JSONPointer: "/code"},
                RuleID:  "TERMINAL_COVERAGE",
                Tier:    2,
            })
        }
    }

    // ALL_PATHS_RETURN: non-terminal nodes with only a "next" edge to ::end
    for _, edge := range graph.Edges {
        if edge.Kind == "next" && edge.To == "::end" {
            node := findNode(graph, edge.From)
            if node != nil && node.Kind != "end" &&
                (node.UI == nil || !node.UI.IsTerminal) {
                diags = append(diags, LintDiagnostic{
                    Level:  "warn",
                    Message: fmt.Sprintf(
                        `Path ending at "%s" reaches end without return_response`, node.Label),
                    Source: &node.Source,
                    RuleID: "ALL_PATHS_RETURN",
                    Tier:   2,
                })
            }
        }
    }

    return diags
}
```

### Tier 3: Cross-Step Data Flow (requires IGM)

**What it checks:** Do datapill references resolve to real steps, and are the paths plausible?

**Tool:** IGM alias map + parsed datapill payloads. Extends the alias map the IGM transformer builds during graph construction into a validation input.

**Dependency:** Requires the IGM transformer ported to Go.

| Rule ID | Check | Source |
|---------|-------|--------|
| `DP_LINE_RESOLVES` | Datapill `line` value matches an `as` alias on a step in the recipe | base validation-checklist |
| `DP_PROVIDER_MATCHES` | Datapill `provider` matches the resolved step's actual `provider` | base validation-checklist |
| `DP_STEP_REACHABLE` | The referenced step is reachable from the current step (not in a different branch) | new — structural |
| `DP_TRIGGER_PATH` | API endpoint datapills use `["request", "field"]` paths (no `["body"]` wrapper) | base validation-checklist |

```go
func lintDataFlow(
    recipe map[string]interface{},
    aliasMap map[string]string,    // step.as → nodeId
    stepProviders map[string]string, // nodeId → provider
) []LintDiagnostic {
    var diags []LintDiagnostic

    walkAllDatapills(recipe, func(payload DatapillPayload, ctx DatapillContext) {
        // DP_LINE_RESOLVES
        if _, ok := aliasMap[payload.Line]; !ok {
            diags = append(diags, LintDiagnostic{
                Level:  "error",
                Message: fmt.Sprintf(
                    `Datapill references "%s" but no step has as="%s"`,
                    payload.Line, payload.Line),
                Source: &SourceRef{JSONPointer: ctx.StepPointer},
                RuleID: "DP_LINE_RESOLVES",
                Tier:   3,
            })
            return
        }

        // DP_PROVIDER_MATCHES
        nodeID := aliasMap[payload.Line]
        actualProvider := stepProviders[nodeID]
        if actualProvider != "" && payload.Provider != actualProvider {
            if !(payload.Provider == "catch" && actualProvider == "null") {
                diags = append(diags, LintDiagnostic{
                    Level:  "error",
                    Message: fmt.Sprintf(
                        `Datapill provider "%s" doesn't match "%s" provider "%s"`,
                        payload.Provider, payload.Line, actualProvider),
                    Source: &SourceRef{JSONPointer: ctx.StepPointer},
                    RuleID: "DP_PROVIDER_MATCHES",
                    Tier:   3,
                })
            }
        }
    })

    return diags
}
```

---

## Diagnostic Format

All tiers emit the same structured diagnostic:

```go
type LintDiagnostic struct {
    Level   string     `json:"level"`    // "info", "warn", "error"
    Message string     `json:"message"`
    Source  *SourceRef `json:"source,omitempty"` // JSON pointer for navigation
    RuleID  string     `json:"rule_id"`  // Machine-readable rule identifier
    Tier    int        `json:"tier"`     // 0, 1, 2, or 3
}

type SourceRef struct {
    JSONPointer string `json:"json_pointer"`
}
```

This format is compatible with the IGM transformer's existing `Diagnostic` type. When the IGM is ported to Go, its diagnostics get `RuleID: "IGM_INTERNAL"` and merge into the same output stream.

---

## CLI Integration

The linter ships as a plugin (`recipe-lint`) that registers the `wk lint` command. Zero additional setup beyond `wk plugins install`.

```bash
# Lint a single recipe
wk lint recipe.json

# Lint with specific tiers
wk lint recipe.json --tier 0,1       # schema + intra-step only (no IGM needed)
wk lint recipe.json --tier all       # all tiers (default, requires IGM after Phase 3)

# JSON output for agents and CI
wk lint recipe.json --json

# Lint all recipes in a project
wk lint ./recipes/

# Lint with connector-specific rules from a skills directory
wk lint recipe.json --skills-path ./skills/

# Exit codes
#   0 = no errors (warnings OK)
#   1 = one or more errors
#   2 = invalid input (not JSON, file not found)
```

**Plugin command flags:** The plugin binary parses its own flags (Option A). The CLI passes all args as a flat string array via JSON-RPC. The plugin uses Go's `flag` package internally to parse `--tier`, `--skills-path`, etc. The global `--json` flag controls CLI output formatting — the plugin always returns structured JSON via RPC.

### `wk recipes validate` Unification

The CLI PRD already specifies `wk recipes validate <path>` for local validation. The plugin registers `validate` as a subcommand alias for `lint`:

```bash
wk recipes validate recipe.json          # → wk lint recipe.json
wk recipes validate recipe.json --json   # → wk lint recipe.json --json
```

Remote validation (`wk recipes validate <id>`) continues to call the Workato API.

### `wk push` Pre-Flight Hook

`wk push` dispatches `pre-push` hooks to all installed plugins that declare one. The `recipe-lint` plugin's pre-push hook lints all `.recipe.json` files in the push set. Errors block the push; warnings are displayed but don't block. `--skip-lint` bypasses hook dispatch entirely.

**Pre-push hook protocol (JSON-RPC):**

Request:
```json
{
  "jsonrpc": "2.0",
  "method": "lint.pre_push",
  "params": {
    "project_root": "/path/to/project",
    "files": [
      "recipes/submit_maintenance_request.recipe.json",
      "recipes/check_in_guest.recipe.json"
    ],
    "config_path": "/path/to/project/.wklintrc.json"
  },
  "id": 1
}
```

Response:
```json
{
  "jsonrpc": "2.0",
  "result": {
    "passed": false,
    "diagnostics": [
      {
        "file": "recipes/submit_maintenance_request.recipe.json",
        "level": "error",
        "message": "Condition lhs uses formula mode...",
        "source": {"json_pointer": "/code/block/0/block/2"},
        "rule_id": "DP_LHS_NO_FORMULA",
        "tier": 1
      }
    ],
    "summary": {"errors": 1, "warnings": 0, "info": 0}
  },
  "id": 1
}
```

**Push flow:**

```
User runs: wk push

1. Push command resolves files to push (via sync engine status)
2. Before engine.Push(), dispatches pre-push hooks
3. Finds recipe-lint plugin has pre-push hook → method "lint.pre_push"
4. Sends file list via JSON-RPC
5. Plugin lints all .recipe.json files in the push set
6. Returns diagnostics
7. If errors: print diagnostics, block push (exit 1)
8. If warnings only: print to stderr, continue push
9. --skip-lint bypasses hook dispatch entirely
```

### `--json` Output (Agent-Native)

Agents consume lint results as structured JSON:

```json
{
  "file": "submit_maintenance_request.recipe.json",
  "diagnostics": [
    {
      "level": "error",
      "message": "Condition lhs uses formula mode...",
      "source": {"json_pointer": "/code/block/0/block/2"},
      "rule_id": "DP_LHS_NO_FORMULA",
      "tier": 1
    }
  ],
  "summary": {
    "errors": 1,
    "warnings": 0,
    "info": 0
  }
}
```

---

## Rule Suppression

Rule suppression ships in Phase 1. Without it, the `wk push` lint gate will drive teams to `--skip-lint` on the first false positive.

### `.wklintrc.json` — project-level configuration

Located at the project root (next to `wk.toml`). Loaded by `pkg/lint/config.go`.

```json
{
  "version": "0.1.0",
  "rules": {
    "UUID_DESCRIPTIVE": "off",
    "DP_INTERPOLATION_SINGLE": "warn"
  },
  "ignore_files": [
    "legacy/*.recipe.json"
  ]
}
```

**Schema:**

| Field | Type | Description |
|---|---|---|
| `version` | string (semver) | Config schema version. Loader rejects configs with a major version it doesn't support. |
| `rules` | `map[string]string` | Rule ID → severity override. Values: `"off"`, `"info"`, `"warn"`, `"error"`. Absent rules use default severity. |
| `ignore_files` | `[]string` | Glob patterns for files to skip entirely. Relative to project root. |

### No inline suppression (yet)

Inline suppression (e.g., a `_lint_ignore` field in the recipe JSON) is deferred. Recipe JSON is a Workato platform format — adding non-standard fields risks compatibility issues. If inline suppression is needed later, it should use a sidecar file (e.g., `.wklint-ignore` next to the recipe) rather than polluting the recipe JSON.

### `--skip-lint` telemetry

When `--skip-lint` is used on `wk push`, emit a `[debug]` line (visible with `--verbose`) logging the skip. This enables future analysis of how often teams bypass the gate.

---

## Where Things Live

```
wk-cli-beta/
├── pkg/
│   ├── lint/
│   │   ├── lint.go                  # Orchestrator: LintRecipe() entry point
│   │   ├── diagnostic.go            # LintDiagnostic, SourceRef, severity constants
│   │   ├── tier0_schema.go          # JSON structure validation
│   │   ├── tier1_steps.go           # Step-level rules
│   │   ├── tier1_datapills.go       # Datapill walker (Phase 2)
│   │   ├── tier1_eis.go             # EIS mirror checking (Phase 3)
│   │   ├── tier2_structure.go       # IGM graph assertions (Phase 3)
│   │   ├── tier3_dataflow.go        # Datapill resolution (Phase 4)
│   │   ├── config.go                # .wklintrc.json loading + rule suppression
│   │   ├── rules.go                 # Rule registry + connector-specific rule loading
│   │   ├── rules_test.go
│   │   └── testdata/
│   │       ├── fixtures/            # Golden recipe files (see Test Fixtures)
│   │       └── malformed/           # Negative test cases
│   ├── recipe/
│   │   ├── parse.go                 # Recipe JSON → typed structs
│   │   └── walk.go                  # Generic tree walker with path tracking
│   └── igm/                         # (Phase 3) Go port of IGM transformer
│       ├── transformer.go
│       ├── types.go
│       └── transformer_test.go
├── plugins/
│   └── recipe-lint/
│       ├── main.go                  # JSON-RPC server, imports pkg/lint
│       ├── plugin.toml              # Manifest: wk lint command + pre-push hook
│       └── Makefile                 # Build the plugin binary
├── internal/
│   └── plugin/
│       ├── hooks.go                 # NEW: pre-push/post-pull hook dispatch
│       └── manifest.go              # MODIFIED: add Hooks field
```

### What Lives Where

| Artifact | Location | Language | Reason |
|----------|----------|----------|--------|
| Linter core (Tiers 0-1) | `wk-cli-beta/pkg/lint/` | Go | Importable library, used by plugin binary |
| Linter structure rules (Tier 2) | `wk-cli-beta/pkg/lint/` | Go | Consumes Go IGM, same library |
| Linter data flow rules (Tier 3) | `wk-cli-beta/pkg/lint/` | Go | Consumes Go IGM alias map, same library |
| Plugin binary | `wk-cli-beta/plugins/recipe-lint/` | Go | JSON-RPC server, imports `pkg/lint` |
| Plugin manifest | `plugins/recipe-lint/plugin.toml` | TOML | Declares commands + hooks |
| IGM transformer (Go port) | `wk-cli-beta/pkg/igm/` | Go | Linter Tiers 2-3 + future `wk igm` command |
| IGM transformer (original) | `recipe-visualizer/core/` | TypeScript | VS Code extension visualization |
| Connector-specific rule data | `recipe-skills/skills/*/` | JSON | Loaded by Go linter at runtime |
| Prose validation checklists | `recipe-skills/skills/*/` | Markdown | Human documentation (not executed) |
| Installed plugin (user) | `~/.wk/plugins/recipe-lint/` | Binary | Installed via `wk plugins install` |

### Two IGM Implementations, One Spec

The IGM v1.2 spec (`igm-schema-and-mapping-rules-v1.2-merged.md`) is the shared contract. Both the TypeScript and Go implementations must produce conformant graphs for the same input recipes. Conformance is validated by running both implementations against the same fixture set (`dewy-resort/workato/recipes/orchestrator-recipes/`) and comparing output snapshots.

The TypeScript implementation continues to serve the VS Code extension. The Go implementation serves the CLI linter and any future `wk igm <recipe>` command. They are siblings, not a fork — changes to the spec are implemented in both.

---

## Connector-Specific Extensibility

Connector-specific lint knowledge lives in JSON data files inside each skill directory. The Go linter loads these at runtime from a `--skills-path` argument or a well-known location.

```json
// recipe-skills/skills/salesforce-recipes/lint-rules.json
{
  "version": "0.1.0",
  "connector": "salesforce",
  "connector_internals": ["sobject_name", "limit"],
  "action_rules": [
    {
      "rule_id": "SF_DPS_SOBJECT",
      "action_names": ["search_sobjects", "upsert_sobject", "update_sobject"],
      "require_fields": ["sobject_name"],
      "require_in": ["input", "dynamicPickListSelection"],
      "message": "Salesforce action missing sobject_name in {missing_location}"
    },
    {
      "rule_id": "SF_SOQL_NO_EIS",
      "action_names": ["search_sobjects_soql"],
      "eis_must_be_empty": true,
      "message": "SOQL search action should have empty extended_input_schema"
    },
    {
      "rule_id": "SF_LIMIT_TYPE",
      "action_names": ["search_sobjects"],
      "field_type_checks": {
        "limit": {"type": "integer", "parse_output": "integer_conversion"}
      },
      "message": "Salesforce limit must use integer type with integer_conversion (not number, which produces LIMIT 50.0)"
    }
  ]
}
```

### `lint-rules.json` schema version: `0.1.0`

The `version` field is required and uses semver to signal schema stability.

**Loader behavior:**

| Condition | Behavior |
|---|---|
| `version` field absent | Reject with error: "lint-rules.json missing required 'version' field" |
| `version` major > supported | Reject with error: "lint-rules.json version X.Y.Z not supported (max: 0.x.x)" |
| Unknown fields present | Ignore (forward-compatible) |
| `version` minor > supported | Load with warning: "lint-rules.json version X.Y.Z newer than supported — some rules may be ignored" |

**Go types:**

```go
type ConnectorRules struct {
    Version            string       `json:"version"`
    Connector          string       `json:"connector"`
    ConnectorInternals []string     `json:"connector_internals"`
    ValidActionNames   []string     `json:"valid_action_names,omitempty"`
    ActionRules        []ActionRule `json:"action_rules"`
}

type ActionRule struct {
    RuleID          string                `json:"rule_id"`
    ActionNames     []string              `json:"action_names"`
    RequireFields   []string              `json:"require_fields,omitempty"`
    RequireIn       []string              `json:"require_in,omitempty"`
    EISMustBeEmpty  bool                  `json:"eis_must_be_empty,omitempty"`
    CaseSensitive   bool                  `json:"case_sensitive,omitempty"`
    FieldTypeChecks map[string]FieldCheck `json:"field_type_checks,omitempty"`
    Message         string                `json:"message"`
}

func LoadConnectorRules(skillsPath string) (map[string]*ConnectorRules, error) {
    rules := make(map[string]*ConnectorRules)
    // Walk skillsPath, find */lint-rules.json, unmarshal each
    return rules, nil
}
```

Adding a new connector's lint rules means adding a `lint-rules.json` file — no Go code changes, no recompilation, no new CLI release. The plugin bundles its own `rules/` directory and can also load from `--skills-path`.

### `valid_action_names` — Provider Action Validation (Amendment, March 2026)

The `valid_action_names` field on `ConnectorRules` lists the allowed action names for a provider. When present and non-empty, the `ACTION_NAME_VALID` rule checks that every step using that provider has a `name` matching one of the allowed values. If absent or empty, the rule is skipped for that provider.

Example for the `rest` connector:

```json
{
  "version": "0.1.0",
  "connector": "rest",
  "valid_action_names": ["make_request_v2"],
  "connector_internals": [],
  "action_rules": []
}
```

**Decision: static file, not API.** Provider-to-action mappings are shipped as static JSON data in `lint-rules.json` files. They are not fetched from the Workato API at lint time. This is consistent with the zero-network-dependency principle: "No runtime dependencies beyond the `wk` binary and the installed plugin." Mappings are updated by updating the connector's `lint-rules.json` file.

---

## Rule Evolution: No DSL — Go is the Rule Language

The linter will never have a rule DSL. The evolution path is A (compiled rules) → C (plugin rules in Go), deliberately skipping B (declarative JSON rules for core logic).

### What stays in JSON

Connector-specific *data* stays in `lint-rules.json` files. These are not rules — they are data that parameterize existing Go rules. "Salesforce's `sobject_name` is a connector-internal field" is data. "Check that every `input` field has a matching EIS entry" is a rule. The distinction:

| Artifact | Language | Changes when... | Example |
|---|---|---|---|
| Rule logic | Go (in `pkg/lint/`) | Recipe format changes, new validation patterns discovered | `EIS_MIRRORS_INPUT` check |
| Connector data | JSON (`lint-rules.json`) | New connector supported, connector API changes | Salesforce `connector_internals` list |
| Rule configuration | JSON (`.wklintrc.json`) | Project preferences change | "Disable `UUID_DESCRIPTIVE` in this repo" |

### Why not a DSL (Option B)

Option B (declarative rule language) appears to offer the same benefit as Option C (external rules without recompilation) at lower cost. It doesn't. The cost is hidden:

1. **Every DSL becomes Turing-complete or frustrating.** The first request will be "match this pattern but only if the parent block is a try." The second will be "check this field but only when the connector is Salesforce AND the action is `search_sobjects`." Within six months, the JSON schema has conditionals, negation, and path expressions — a bad programming language.

2. **DSL rules are harder to test than Go rules.** A Go rule is a function with inputs and outputs — standard `go test`. A DSL rule requires a DSL evaluator, and bugs can live in the evaluator or the rule definition. Two failure modes instead of one.

3. **DSL rules can't call the recipe walker.** The datapill linter's power comes from `DatapillContext` — knowing that a string is inside `conditions[].lhs` and therefore subject to different rules. A JSON DSL can't express "walk all string values, track field path context, apply rule only in condition LHS positions." The walker is Go code. Rules that need the walker must be Go code.

### The intended evolution

```
Phase 1-2:  All rules in pkg/lint/, compiled into recipe-lint plugin
            New rules → plugin update, not CLI release
            Solves the release coupling problem

Phase 3+:   pkg/lint exports LintRule interface
            External rule plugins implement the interface
            recipe-lint plugin can load external rule plugins
            Solves the custom rule problem

Never:      JSON rule DSL for core logic
            Wrong abstraction level
```

The `LintRule` interface (future):

```go
type LintRule interface {
    ID() string
    Tier() int
    Check(recipe *Recipe, ctx LintContext) []LintDiagnostic
}
```

External rule plugins are separate plugin binaries that implement this interface. The `pkg/lint` library provides the recipe walker, diagnostic types, and datapill context — external rule authors write rules against these types in Go. No DSL, no JSON rule language. Go is the rule language.

---

## Relationship to IGM

The linter is **not** a replacement for the IGM transformer. They have different jobs:

| Concern | IGM Transformer | Recipe Linter |
|---------|----------------|---------------|
| Primary output | Graph for visualization | Diagnostics for validation |
| Runs when | Recipe opened/changed in editor | Recipe generated, pre-push, CI |
| Node/edge construction | Yes | No (consumes IGM output for Tiers 2-3) |
| Datapill field-level parsing | No (node-level only) | Yes (string-level within fields) |
| Connector-specific rules | No | Yes (via JSON rule files) |
| Schema comparison (EIS) | No | Yes |

Tiers 0 and 1 run independently of the IGM — no transformer dependency, fast, available immediately. Tiers 2 and 3 consume the Go IGM graph as input, available after the transformer is ported.

---

## Test Fixture Requirements

Phase 1 requires four recipe JSON fixtures before implementation begins. Three are selected from `dewy-resort/workato/recipes/orchestrator-recipes/`; one is hand-crafted.

### Required fixtures

| Fixture | Source | Rules Exercised | File |
|---|---|---|---|
| API endpoint recipe with try/catch | dewy-resort | `RESPONSE_CODES_DEFINED`, `PICK_LIST_MATCH`, `CATCH_*`, `ALL_PATHS_RETURN`, `TERMINAL_COVERAGE`, `SUCCESS_BEFORE_CATCH`, `return_response` EIS rules | `pkg/lint/testdata/fixtures/api_endpoint_try_catch.recipe.json` |
| Simple connector recipe (linear) | dewy-resort | `STEP_NUMBERING`, `UUID_*`, `CONFIG_*`, `EIS_MIRRORS_INPUT`, connector-specific rules | `pkg/lint/testdata/fixtures/simple_connector.recipe.json` |
| Recipe with if/else branching | dewy-resort | `IF_NO_PROVIDER`, `ELSE_NO_PROVIDER`, `ELSE_LAST_IN_IF`, `NO_ELSIF`, `DP_LHS_NO_FORMULA` | `pkg/lint/testdata/fixtures/if_else_branching.recipe.json` |
| Deliberately malformed recipe | Hand-crafted | All Tier 0 rules (negative cases): missing `code`, `code` as array, missing `uuid`, bad `config` shape | `pkg/lint/testdata/malformed/tier0_failures.recipe.json` |

### Selection criteria for dewy-resort fixtures

- Choose recipes that are known-good (they've been deployed and tested in workshops)
- Each fixture should be annotated with a comment header documenting which fields the linter inspects (not in the JSON itself — in a companion `.md` or in the test file)
- Pin fixtures by copying them into `pkg/lint/testdata/`, not by referencing external paths. The test suite must be self-contained.

### Additional fixtures (Phase 2+)

- Phase 2 adds a datapill-heavy fixture with formula mode, interpolation mode, and condition `lhs` fields
- Phase 3 adds fixtures from a second repo (not dewy-resort) to validate regex and rule coverage against a different recipe authoring style

---

## Implementation Sequencing

### Phase 1: Tier 0 + Tier 1a + Plugin Infrastructure — Schema + Step Rules + Plugin Shell

**When:** CLI Phase 2 (weeks 7-10), alongside `wk recipes` full CRUD.

JSON structure validation, plus step-level rules (numbering, UUID, config, if/else/catch structure). Deterministic, no IGM, catches the most common agent mistakes. Ships with the plugin binary, manifest, pre-push hook, and rule suppression.

**Deliverables:**

| Deliverable | Description |
|---|---|
| `pkg/lint/` | `LintRecipe(recipe []byte, filename string, opts LintOptions) []LintDiagnostic` |
| `pkg/lint/config.go` | `.wklintrc.json` loading + rule suppression |
| `pkg/lint/rules.go` | Rule registry + `lint-rules.json` version-aware loading (`0.1.0`) |
| `pkg/recipe/parse.go` | Typed recipe JSON parsing |
| `plugins/recipe-lint/` | Plugin binary (JSON-RPC server importing `pkg/lint`) + `plugin.toml` manifest |
| `internal/plugin/hooks.go` | Pre-push hook dispatch (~40-60 lines) |
| `internal/plugin/manifest.go` | Modified: `Hooks` field on `Manifest` struct |
| `internal/commands/sync.go` | Modified: pre-push hook invocation + `--skip-lint` flag |
| Test fixtures | 4 fixtures: 3 dewy-resort + 1 hand-crafted negative |
| Tier 0 + Tier 1a rules | All rules in these tiers implemented and tested |

**What pulls forward from ROADMAP.md:**

- **P4.1 (Plugin hooks)** moves to Phase 1. Scope: `pre-push` hook only (not `post-pull`). ~40-60 lines in `internal/plugin/hooks.go` + `Hooks` field on `Manifest`.
- **Plugin command flag passing** is deferred (Option A: plugin parses its own args from the string array).

### Phase 2: Tier 1b — Datapill Linter

**When:** CLI Phase 2-3 (weeks 9-12). Highest false-negative impact.

The dedicated datapill walker with context-aware validation. The `DP_LHS_NO_FORMULA` rule alone justifies this phase.

**Deliverables:**
- `pkg/lint/tier1_datapills.go` with `lintDatapills()` and `walkAllStrings()`
- `pkg/recipe/walk.go` for generic JSON tree walking with path tracking
- Datapill regex validation report against two corpora

### Phase 3: Tier 1c + Tier 2 — EIS + IGM Structural Rules

**When:** CLI Phase 3-4 (weeks 11-18). Requires IGM port.

EIS mirror checking (tree comparison, connector-internal awareness) plus IGM-based structural assertions. Ships alongside or just after the Go IGM transformer port.

**Deliverables:**
- `pkg/lint/tier1_eis.go` with EIS mirror checking
- `pkg/lint/tier2_structure.go` consuming `*igm.Graph`
- `pkg/igm/` — Go port of IGM transformer (shared with future `wk igm` command)
- Connector-specific `lint-rules.json` loading from `--skills-path`
- Conformance test suite: Go IGM vs TypeScript IGM on shared fixtures

### Phase 4: Tier 3 — Data Flow Validation

**When:** CLI Phase 4+ (weeks 15+). Requires IGM alias map.

Datapill reference resolution using the IGM alias map. The alias map already exists in the TypeScript transformer; the Go port carries it forward.

**Deliverables:**
- `pkg/lint/tier3_dataflow.go` consuming alias map from `pkg/igm/`
- `DP_STEP_REACHABLE` rule (graph reachability analysis)

---

## Recipe-Visualizer Integration (Reverse Direction)

The VS Code extension keeps its TypeScript transformer for in-editor visualization. Rather than maintaining two linter implementations, the extension can optionally shell out to `wk lint --json` if the `wk` binary is on the user's PATH — getting full lint diagnostics in the editor with zero TypeScript linter code. If `wk` is not available, the extension falls back to IGM-only diagnostics (which already cover some structural issues).

For connector-specific rules, both the Go linter and the TypeScript extension can load the same `lint-rules.json` files from the skills directory. The rule data is the shared artifact; the execution engines are language-specific.

---

## What This Doesn't Solve

**Semantic correctness.** The linter can check that a datapill reference resolves, but not that it references the *right* field for the business logic. "You referenced `search_contact.Email` but should have referenced `search_contact.Id`" is a judgment call, not a lint rule.

**Deploy-readiness metadata.** The v5 experiment found that conditions B/C/E produced "deploy-ready" recipe function call schemas with extended metadata. The linter can check that `zip_name` exists (structural), but not that the extended metadata is complete (because "complete" is defined by the target Workato environment, not by the recipe JSON alone).

**Pattern quality.** "Is this retry-safe?" and "Should this use compound conditionals?" are design judgment, not lint violations. These remain in the domain of the skills documentation and agent reasoning.
