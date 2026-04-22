# Rule Authoring Guide

This guide covers how to configure the linter, work with profiles, and author custom lint rules for your team's recipe conventions.

## Configuration

Project-level configuration lives in `.wklintrc.json`, placed at your project root (next to `wk.toml`). You can also specify a path with `--config-path`.

```json
{
  "version": "0.1.0",
  "profile": "strict",
  "rules": {
    "UUID_DESCRIPTIVE": "off",
    "DP_INTERPOLATION_SINGLE": "warn"
  },
  "ignore_files": [
    "legacy/*.recipe.json"
  ]
}
```

| Field | Type | Description |
|-------|------|-------------|
| `version` | string (semver) | Config schema version. Required. |
| `profile` | string | Profile name (`standard` or `strict`). Optional; defaults to `standard`. |
| `rules` | map | Rule ID to severity override. Values: `"off"`, `"info"`, `"warn"`, `"error"`. |
| `ignore_files` | string array | Glob patterns for files to skip entirely. Relative to project root. |

### Severity Resolution

Severity is resolved in three layers. Each layer overrides the previous:

1. **Hardcoded default** (defined in the rule source code)
2. **Profile** (`standard.json` or `strict.json`)
3. **`.wklintrc.json`** (your project overrides)

For example, `FORMULA_METHOD_INVALID` defaults to `warn`, the `strict` profile escalates it to `error`, and your `.wklintrc.json` could set it back to `"off"` if needed.

This resolution applies identically to built-in rules and custom rules. If you author a custom rule with `"rule_id": "MY_RULE"`, you can override its severity in `.wklintrc.json` or a profile just like any built-in rule.

---

## Profiles

Profiles are named severity presets. A profile sets the severity for a collection of rules, and your `.wklintrc.json` can override individual rules on top.

### Selecting a Profile

```bash
wk lint recipe.json --profile strict
```

Or in `.wklintrc.json`:

```json
{
  "profile": "strict"
}
```

The `--profile` CLI flag takes precedence over the `.wklintrc.json` setting.

### Built-in Profiles

Two profiles ship with the plugin:

#### `standard` (default)

Baseline profile. Most rules emit warnings; schema violations and structural invariants are errors.

| Rule ID | Severity |
|---------|----------|
| `INVALID_JSON` | error |
| `SCHEMA_MISSING_CODE` | error |
| `SCHEMA_MISSING_CONFIG` | error |
| `ACTION_NAME_VALID` | warning |
| `FORMULA_METHOD_INVALID` | warning |
| `FORMULA_FORBIDDEN_PATTERN` | warning |
| `DP_VALID_JSON` | error |
| `DP_LHS_NO_FORMULA` | warning |
| `DP_INTERPOLATION_SINGLE` | warning |
| `DP_FORMULA_CONCAT` | warning |
| `DP_NO_OUTER_PARENS` | info |
| `DP_NO_BODY_NATIVE` | warning |
| `DP_CATCH_PROVIDER` | warning |
| `EIS_MIRRORS_INPUT` | warning |
| `EIS_NESTED_MATCH` | warning |
| `EIS_NAME_MATCH` | warning |
| `EIS_NO_CONNECTOR_INTERNAL` | warning |
| `EIS_OUTPUT_MIRRORS_INPUT` | info |
| `CATCH_LAST_IN_TRY` | error |
| `ELSE_LAST_IN_IF` | error |
| `SUCCESS_BEFORE_CATCH` | warning |
| `TERMINAL_COVERAGE` | warning |
| `ALL_PATHS_RETURN` | warning |
| `CATCH_RETURNS_ALL_FIELDS` | warning |
| `RECIPE_CALL_ZIP_NAME` | warning |
| `DP_LINE_RESOLVES` | warning |
| `DP_PROVIDER_MATCHES` | warning |
| `DP_STEP_REACHABLE` | warning |
| `DP_TRIGGER_PATH` | info |

Rules not listed in a profile use their hardcoded default severity (shown in the Rule Reference tables in the README).

#### `strict`

Extends `standard`. Escalates key validation rules to errors, suitable for recipes headed to production.

| Rule ID | Escalated to |
|---------|-------------|
| `ACTION_NAME_VALID` | error |
| `FORMULA_METHOD_INVALID` | error |
| `FORMULA_FORBIDDEN_PATTERN` | error |
| `DP_VALID_JSON` | error |
| `DP_LHS_NO_FORMULA` | error |
| `EIS_MIRRORS_INPUT` | error |
| `EIS_NAME_MATCH` | error |
| `ALL_PATHS_RETURN` | error |
| `TERMINAL_COVERAGE` | error |
| `CATCH_RETURNS_ALL_FIELDS` | error |
| `RECIPE_CALL_ZIP_NAME` | error |
| `DP_LINE_RESOLVES` | error |
| `DP_PROVIDER_MATCHES` | error |
| `DP_STEP_REACHABLE` | error |

### Custom Profiles

Create your own profiles by adding JSON files to `.wklint/profiles/` in your project root.

```
my-project/
├── .wklint/
│   └── profiles/
│       └── team_api.json
├── .wklintrc.json        # "profile": "team_api"
└── recipes/
```

Example `team_api.json`:

```json
{
  "name": "team_api",
  "description": "API team profile -- strict on data flow, relaxed on formatting",
  "extends": "standard",
  "rules": {
    "DP_LINE_RESOLVES": "error",
    "DP_PROVIDER_MATCHES": "error",
    "DP_STEP_REACHABLE": "error",
    "UUID_DESCRIPTIVE": "off"
  }
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Must match the filename (without `.json`). Lowercase, underscores only. |
| `description` | string | no | Human-readable description. |
| `extends` | string | no | Name of a parent profile to inherit from. Omit for no parent. |
| `rules` | map | yes | Rule ID to severity. Only rules that differ from the parent need to be listed. |

Inheritance rules:
- Single-parent only (no multiple inheritance).
- Maximum chain depth of 5.
- Child rules override parent rules of the same ID.
- Project profiles (`.wklint/profiles/`) override plugin-bundled profiles of the same name.

---

## Custom Rules

Custom rules let you extend the linter with your own validation logic — structural constraints, naming conventions, required fields, architectural patterns — defined entirely in JSON. Rules run through the same tier engine as built-in rules.

There are two authoring surfaces:

| Who | Where | Format |
|-----|-------|--------|
| Connector skill authors | `skills/{connector}/lint-rules.json` (loaded via `--skills-path`) | v0.1.0 connector data and/or v0.2.0 declarative rules |
| Project teams | `.wklint/rules/*.json` (auto-discovered from project root) | v0.2.0 declarative rules |

### Declarative Rules (v0.2.0)

The v0.2.0 schema lets you define rules as composable matchers. Each rule has a scope (recipe-level or step-level), an optional `where` clause to select which steps it applies to, and an `assert` clause that must hold true.

```json
{
  "version": "0.2.0",
  "rules": [
    {
      "rule_id": "MAX_ONE_ACTION",
      "tier": 1,
      "level": "error",
      "message": "Recipe must have at most one action step",
      "scope": "recipe",
      "assert": {
        "step_count": { "where": { "keyword": "action" }, "max": 1 }
      }
    },
    {
      "rule_id": "UUID_PREFIX_SF",
      "tier": 1,
      "level": "warn",
      "message": "Salesforce step UUID must start with 'sf-'",
      "scope": "step",
      "where": { "provider": "salesforce" },
      "assert": {
        "field_matches": { "path": "uuid", "pattern": "^sf-" }
      }
    },
    {
      "rule_id": "SEARCH_NEEDS_LIMIT",
      "tier": 1,
      "level": "warn",
      "message": "Search action missing limit in input",
      "scope": "step",
      "where": { "provider": "salesforce", "action_name": "search_sobjects" },
      "assert": {
        "field_exists": { "path": "input.limit" }
      }
    }
  ]
}
```

#### Rule Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `rule_id` | string | yes | Unique identifier (appears in lint output; can be overridden in profiles/config). |
| `tier` | int | yes | Which tier this rule runs at (1, 2, or 3). |
| `level` | string | yes | Default severity: `"error"`, `"warn"`, or `"info"`. |
| `message` | string | yes | Diagnostic message when the assertion fails. |
| `scope` | string | yes | `"recipe"` (evaluated once per recipe) or `"step"` (evaluated per matching step). |
| `where` | object | no | Step selector — only steps matching all criteria are checked. Omit to match all steps. |
| `assert` | object | yes | The assertion that must hold true. If it fails, a diagnostic is emitted. |

#### Step Selector (`where`)

The `where` clause filters which steps a step-scoped rule applies to. All specified fields must match (AND logic). Each field accepts a single string or an array of strings (OR within a field).

| Field | Type | Description |
|-------|------|-------------|
| `keyword` | string or string[] | Step keyword: `"trigger"`, `"action"`, `"if"`, `"else"`, `"try"`, `"catch"` |
| `provider` | string or string[] | Provider name (e.g., `"salesforce"`, `"rest"`) |
| `action_name` | string or string[] | Action name (e.g., `"search_sobjects"`) |

Examples:
- `{ "provider": "salesforce" }` — all Salesforce steps
- `{ "keyword": ["action", "trigger"] }` — actions and triggers
- `{ "provider": "salesforce", "action_name": "search_sobjects" }` — only Salesforce search

#### Assertion Matchers

Each assertion is an object with exactly one matcher key. Matchers can be composed with `all_of` and `any_of`.

##### `field_exists`

Asserts a field path exists on the step.

```json
{ "field_exists": { "path": "input.sobject_name" } }
```

##### `field_absent`

Asserts a field path does not exist.

```json
{ "field_absent": { "path": "input.deprecated_field" } }
```

##### `field_matches`

Asserts a field value matches a regex pattern.

```json
{ "field_matches": { "path": "uuid", "pattern": "^sf-" } }
```

##### `field_equals`

Asserts a field value equals a literal.

```json
{ "field_equals": { "path": "keyword", "value": "action" } }
```

##### `step_count`

Asserts the number of steps matching a selector. Recipe-scoped only.

```json
{ "step_count": { "where": { "keyword": "action" }, "max": 5 } }
```

| Field | Type | Description |
|-------|------|-------------|
| `where` | object | Step selector (same schema as rule-level `where`). Omit to count all steps. |
| `min` | int | Minimum count (inclusive). |
| `max` | int | Maximum count (inclusive). |
| `exact` | int | Exact count required. |

##### `eis_empty`

Asserts the step's `extended_input_schema` is null, absent, or an empty array.

```json
{ "eis_empty": true }
```

##### `eis_field_type`

Asserts an EIS field has the expected type and/or parse_output.

```json
{ "eis_field_type": { "name": "limit", "type": "integer", "parse_output": "integer_conversion" } }
```

##### `all_of`

All sub-assertions must pass (logical AND).

```json
{
  "all_of": [
    { "field_exists": { "path": "input.email" } },
    { "field_matches": { "path": "uuid", "pattern": "^notify-" } }
  ]
}
```

##### `any_of`

At least one sub-assertion must pass (logical OR).

```json
{
  "any_of": [
    { "field_exists": { "path": "input.id" } },
    { "field_exists": { "path": "input.external_id" } }
  ]
}
```

#### Field Paths

Field paths use dot notation to navigate into step data:

| Path prefix | Resolves to |
|-------------|-------------|
| `uuid` | Step UUID |
| `name` | Action name |
| `keyword` | Step keyword |
| `provider` | Provider name |
| `as` | Step alias |
| `input.{field}` | Key inside the step's `input` JSON |
| `extended_input_schema` | The step's EIS (raw) |
| `dynamicPickListSelection.{field}` | Key inside DPS JSON |

Nested paths navigate into JSON objects: `input.address.street` looks up `input` → `address` → `street`.

### Connector Rules (v0.1.0)

The v0.1.0 format is the original connector-specific schema. It remains fully supported — existing `lint-rules.json` files work without changes.

```json
{
  "version": "0.1.0",
  "connector": "salesforce",
  "connector_internals": ["sobject_name", "limit"],
  "valid_action_names": ["search_sobjects", "upsert_sobject"],
  "action_rules": [
    {
      "rule_id": "SF_DPS_SOBJECT",
      "action_names": ["search_sobjects", "upsert_sobject"],
      "require_fields": ["sobject_name"],
      "require_in": ["input", "dynamicPickListSelection"],
      "message": "Salesforce action missing sobject_name in {missing_location}"
    }
  ]
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `version` | string | yes | Schema version (`"0.1.0"`). |
| `connector` | string | yes | Provider name as it appears in recipe `provider` fields. |
| `connector_internals` | string[] | yes | Fields excluded from EIS checks (`EIS_NO_CONNECTOR_INTERNAL`). Use `[]` if none. |
| `valid_action_names` | string[] | no | Allowed action names for `ACTION_NAME_VALID`. Omit or empty to skip. |
| `action_rules` | array | yes | Connector-specific check rules. Use `[]` if none. |

**Action rule fields:**

| Field | Type | Description |
|-------|------|-------------|
| `rule_id` | string | Unique identifier. |
| `action_names` | string[] | Which actions this rule applies to. |
| `require_fields` | string[] | Fields that must be present. |
| `require_in` | string[] | Locations to check (e.g., `["input"]`). Defaults to `["input"]`. |
| `eis_must_be_empty` | bool | If true, EIS must be empty for this action. |
| `field_type_checks` | map | Field name to `{"type": "...", "parse_output": "..."}`. |
| `message` | string | Diagnostic message. Supports `{field_name}` and `{missing_location}` placeholders. |

### Mixed Format (v0.2.0 with connector data)

Connector skill authors can combine both formats in a single file — v0.1.0 connector data fields alongside v0.2.0 declarative rules:

```json
{
  "version": "0.2.0",
  "connector": "salesforce",
  "connector_internals": ["sobject_name"],
  "valid_action_names": ["search_sobjects", "upsert_sobject"],
  "action_rules": [
    {
      "rule_id": "SF_DPS_SOBJECT",
      "action_names": ["search_sobjects"],
      "require_fields": ["sobject_name"],
      "require_in": ["input"],
      "message": "Missing sobject_name in {missing_location}"
    }
  ],
  "rules": [
    {
      "rule_id": "SF_UUID_PREFIX",
      "tier": 1,
      "level": "warn",
      "message": "Salesforce step UUID should start with 'sf-'",
      "scope": "step",
      "where": { "provider": "salesforce" },
      "assert": { "field_matches": { "path": "uuid", "pattern": "^sf-" } }
    }
  ]
}
```

### Loading Rules

```bash
# Load connector rules from a skills directory
wk lint recipe.json --skills-path ./skills/

# Project rules are auto-discovered from .wklint/rules/
wk lint recipe.json --config-path .wklintrc.json
```

The linter discovers rules from two paths:
1. `--skills-path` — walks the directory recursively, loading all `lint-rules.json` files
2. Project root (derived from `--config-path`) — loads all `*.json` files from `.wklint/rules/`

Both paths are optional. Without them, only built-in rules run.

### Testing Rules Against a Recipe Corpus

You can validate your custom rules against a directory of real recipes using the corpus test suite. This runs every recipe through the full linter — including your custom rules — and reports diagnostics per file.

Set environment variables and run:

```bash
RECIPE_CORPUS_DIR=./recipes \
LINT_SKILLS_PATH=./skills \
LINT_CONFIG_PATH=.wklintrc.json \
  go test ./pkg/lint/ -run Corpus -v
```

| Variable | Required | Description |
|----------|----------|-------------|
| `RECIPE_CORPUS_DIR` | yes | Path to a directory of `.recipe.json` files |
| `LINT_SKILLS_PATH` | no | Path to your skills directory (connector `lint-rules.json` files) |
| `LINT_CONFIG_PATH` | no | Path to your `.wklintrc.json` (loads profile, overrides, and `.wklint/rules/`) |

The test walks `RECIPE_CORPUS_DIR` recursively, runs all four tiers on each `.json` file, and logs diagnostic counts by tier and rule ID. Files that aren't valid recipes are skipped gracefully.

Without `LINT_SKILLS_PATH` or `LINT_CONFIG_PATH`, the corpus test runs built-in rules only. See `.env.example` for a template.
