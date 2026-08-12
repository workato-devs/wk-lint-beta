# Rule Reference

All built-in rules organized by tier. Each rule has a default severity that can be overridden via [profiles and configuration](rule-authoring.md).

## Tier 0: Schema Validation

Checks that the file is syntactically valid recipe JSON with the expected shape.

| Rule ID | Description | Default |
|---------|-------------|---------|
| `INVALID_JSON` | File is not valid JSON | error |
| `CODE_WRAPPED_IN_RECIPE` | Top-level JSON should not be wrapped in a `"recipe"` key | error |
| `MISSING_TOP_LEVEL_KEYS` | Missing a required top-level key (`name`, `version`, `code`, `config`, etc.) | error |
| `CODE_NOT_OBJECT` | `"code"` field must be a JSON object, not an array | error |
| `CONFIG_INVALID` | `"config"` array structure is malformed (not an array, missing `keyword`, wrong `keyword` value) | error |
| `STEP_MISSING_KEYWORD` | A step block is missing the `"keyword"` field | error |
| `STEP_MISSING_NUMBER` | A step block is missing the `"number"` field | error |
| `NUMBER_NOT_INTEGER` | Step `"number"` must be a JSON number, not a string | error |
| `STEP_MISSING_UUID` | A step block is missing the `"uuid"` field | error |
| `UUID_TOO_LONG` | UUID exceeds 36 characters | error |

## Tier 1: Intra-Step Validation

Checks each step for internal correctness without needing to know about other steps.

### Step-Level Rules

| Rule ID | Description | Default |
|---------|-------------|---------|
| `STEP_NUMBERING` | Action `number` fields must be sequential (0, 1, 2, ...) | warn |
| `UUID_UNIQUE` | All `uuid` values must be unique within the recipe | error |
| `UUID_DESCRIPTIVE` | UUID looks like a standard UUID v4; consider using a descriptive name | warn |
| `TRIGGER_NUMBER_ZERO` | Trigger step must have `number: 0` | error |
| `FILENAME_MATCH` | Recipe `name` field does not match the filename | warn |
| `CONFIG_NO_WORKATO` | Config array should not list `"workato"` as a provider | warn |
| `CONFIG_PROVIDER_MATCH` | Every `provider` used in action steps must have a matching `config` entry | warn |
| `IF_NO_PROVIDER` | `"if"` steps should not have a `provider` field | warn |
| `ELSE_NO_PROVIDER` | `"else"` steps should not have a `provider` field | warn |
| `CATCH_PROVIDER_NULL` | `"catch"` steps should have `"provider": null` | warn |
| `CATCH_HAS_AS` | `"catch"` steps must have a non-empty `"as"` field | warn |
| `CATCH_HAS_RETRY` | `"catch"` step input should include `max_retry_count` | info |
| `TRY_NO_AS` | `"try"` steps should have an empty `"as"` field | warn |
| `REPEAT_NO_PROVIDER` | `"repeat"` (loop) steps should not have a `provider` field | warn |
| `WHILE_CONDITION_NO_PROVIDER` | `"while_condition"` steps should not have a `provider`, `name`, or `as` field | warn |
| `NO_ELSIF` | `"elsif"` keyword is not allowed; use nested if/else instead | error |
| `ACTION_NAME_VALID` | Action `name` must be in the allowed set for its `provider` (via connector `lint-rules.json`) | error |
| `ACTION_RULES` | A connector-specific `action_rules` constraint (legacy v0.1.0 `lint-rules.json`) is violated | warn |
| `RESPONSE_CODES_DEFINED` | API platform triggers should define response codes in input | info |

### Datapill Rules

| Rule ID | Description | Default |
|---------|-------------|---------|
| `DP_LHS_NO_FORMULA` | Condition `lhs` (left-hand side) should use datapill interpolation, not formula mode | warn |
| `DP_VALID_JSON` | `_dp()` payload must be parseable JSON | error |
| `DP_INTERPOLATION_SINGLE` | A single datapill into a **string** field should use `#{_dp(...)}` interpolation, not `=_dp(...)` formula mode. Skipped when the target field is declared `array`, `object`, `number`, `integer` or `boolean` — interpolation always yields a string, so formula mode is what preserves the pill's type | warn |
| `DP_FORMULA_CONCAT` | Multiple datapills should use formula mode with `+` concatenation, not `#{}` interpolation | warn |
| `DP_NO_OUTER_PARENS` | Formula expressions should not be wrapped in unnecessary outer parentheses | info |
| `DP_NO_BODY_NATIVE` | Datapill paths for native connectors should not include `["body"]` | warn |
| `DP_CATCH_PROVIDER` | Datapills referencing catch data should use `"provider":"catch"`, not `null` | warn |

### Extended Input Schema (EIS) Rules

| Rule ID | Description | Default |
|---------|-------------|---------|
| `EIS_MIRRORS_INPUT` | Every field in `input` must have a corresponding `extended_input_schema` entry | warn |
| `EIS_NESTED_MATCH` | Nested objects in `input` must have matching nested `properties` in EIS | warn |
| `EIS_NAME_MATCH` | EIS field names must match `input` field names exactly | warn |
| `EIS_NO_CONNECTOR_INTERNAL` | Connector-internal fields (e.g., `sobject_name`) should not appear in EIS | warn |
| `EIS_OUTPUT_MIRRORS_INPUT` | `extended_output_schema` should mirror `extended_input_schema` for return actions | info |
| `RETURN_RESPONSE_SCHEMA_PARITY` | A `return_response` block's `extended_input_schema` and `extended_output_schema` must be structurally identical (field names, types, nesting) | error |
| `RETURN_RESPONSE_SCHEMA_CONSISTENT` | All `return_response` blocks in the recipe must share an identical EIS and EOS | warn |
| `RETURN_RESPONSE_INPUT_MIRROR` | Every `input.response` field must be defined under the `return_response` EIS `response` field's properties | warn |

### Formula Method Rules

| Rule ID | Description | Default |
|---------|-------------|---------|
| `FORMULA_FORBIDDEN_PATTERN` | Known-bad formula pattern detected (with specific replacement suggestion) | warn |
| `FORMULA_METHOD_INVALID` | Method name is not in the Workato formula allowlist (~120 methods) | warn |

### Action Authoring Rules

Connector- and action-specific footguns where syntactically valid input is silently dropped or rejected on import.

| Rule ID | Description | Default |
|---------|-------------|---------|
| `UPDATE_VARS_RAW_FORM` | `update_variables` uses the structured `variables:[{variable,value}]` form, which is silently dropped on import; use `input_mode:"raw"` with flat per-variable keys | error |
| `BODY_FORMULA_MODE` | A `body` field uses formula mode (`=` prefix), which may be silently stripped; use `#{}` interpolation | warn |
| `BUTTON_PARAMS_FORMAT` | Button `params` use URL-encoded format, which Workbot silently drops; use space-separated `key: value` | warn |
| `STOP_MISSING_REASON` | A `stop` action is missing the `stop_reason` field (required for recipe activation) | warn |
| `STOP_ERROR_IN_GENIE` | `stop` with `stop_with_error:"true"` is rejected in genie skill recipes; use `workflow_return_result` with `success:false` | error |
| `GENIE_SKILL_DESCRIPTION_EMPTY` | A genie skill trigger has an empty `description`; the UI will show a blank skill entry | warn |

## Tier 2: Inter-Step Structure

Checks relationships between steps by building a control flow graph of the recipe. Requires the graph to build successfully (see `IGM_BUILD_FAILED`).

| Rule ID | Description | Default |
|---------|-------------|---------|
| `CATCH_LAST_IN_TRY` | Catch block must be the last child in its try block | error |
| `ELSE_LAST_IN_IF` | Else block must be the last child in its if block | error |
| `REPEAT_HAS_WHILE_CONDITION` | Every `repeat` block must contain a `while_condition` child (its absence silently fails UI reconstruction after push) | error |
| `WHILE_CONDITION_LAST_IN_REPEAT` | `while_condition` must be the last child in its repeat block | error |
| `SUCCESS_BEFORE_CATCH` | Success `return_response` should be in the try body, not in catch | warn |
| `TERMINAL_COVERAGE` | Every HTTP status code declared in trigger responses must have a `return_response` | warn |
| `ALL_PATHS_RETURN` | Every control flow path must terminate with a `return_response` | warn |
| `CATCH_RETURNS_ALL_FIELDS` | Catch `return_response` must provide all fields defined in the trigger response schema | warn |
| `RECIPE_CALL_ZIP_NAME` | Recipe function `call` actions must include `zip_name` in `flow_id` | warn |
| `IGM_BUILD_FAILED` | Control flow graph construction failed; Tiers 2-3 skipped (shows the build error) | warn |

## Tier 3: Cross-Step Data Flow

Resolves datapill references across steps using the control flow graph and step alias map.

| Rule ID | Description | Default |
|---------|-------------|---------|
| `DP_LINE_RESOLVES` | Datapill `line` value must match an `as` alias on a step in the recipe | warn |
| `DP_PROVIDER_MATCHES` | Datapill `provider` must match the resolved step's actual provider | warn |
| `DP_STEP_REACHABLE` | The step referenced by a datapill must be reachable in the control flow graph | warn |
| `DP_TRIGGER_PATH` | API endpoint datapill paths should start with `"request"` | info |
| `DP_PATH_RESOLVES` | Datapill `path` must resolve to a field declared in the referenced step's `extended_output_schema` (recipe-EOS-only; dynamic/open schemas are skipped) | warn |

## Custom Rule Loading

Emitted during rule file discovery, not tied to a specific tier.

| Rule ID | Description | Default |
|---------|-------------|---------|
| `CUSTOM_RULE_INVALID` | A custom rule file could not be parsed or a rule failed validation (shows file path and error) | warn |
