# Changelog

All notable changes to the recipe-lint plugin will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `DP_BARE_UNWRAPPED` (Tier 1, error): a `_dp('{...}')` value that is neither wrapped
  in `#{...}` nor prefixed with `=` is never resolved — the platform emits the literal
  `_dp('{...}')` characters as the field value. Recipes shipped this way push, activate
  and run green while returning pill text to the caller, and no rule flagged it. The
  check runs per string value and skips occurrences already inside a `#{...}` wrapper,
  so a mixed concatenation flags only the unwrapped pill.

- `DP_INTERPOLATION_TYPED` (Tier 1, warning): the inverse of the fix below. A lone datapill
  wrapped in `#{...}` on a field declared `array`, `object`, `number`, `integer` or `boolean`
  is stringified, because interpolation mode always yields a string. The recipe pushes, lints
  clean and runs green; only the caller sees the wrong type. Fires only when the value is
  exactly the wrapped pill — surrounding literal text makes the field a string by
  construction, and formula mode is not the fix for that. Reuses the declared-type resolution
  introduced for `DP_INTERPOLATION_SINGLE` (`extended_input_schema`, falling back to the
  trigger's `result_schema_json` for a return step's `input.result` fields).

### Fixed

- `DP_INTERPOLATION_SINGLE` is now aware of the target field's declared type. It
  previously fired on any single-pill formula-mode value and told the author to drop
  the leading `=`; on a field declared `array`, `object`, `number`, `integer` or
  `boolean` that advice stringifies a correctly-typed value, because interpolation
  mode always yields a string. The rule now resolves the field type from the step's
  `extended_input_schema` — falling back to the trigger's `result_schema_json` for a
  return step's `input.result` fields — and skips non-string targets. Behavior is
  unchanged for string fields and for fields no schema declares. The suggested fix
  names the type constraint.

## [1.1.0] -- 2026-08-01

### Added

- Plugin-owned text rendering (#26): `lint.render` turns an existing `lint.run`
  result into stable, human-readable per-file diagnostics and summaries. Compatible
  `wk` hosts call it only for text output; `--json` continues to emit the canonical
  `lint.run` result and the renderer cannot alter the command exit code.
- Trigger-level `filter` (#28): the "only continue if..." condition is now a recognized field
  on trigger steps and covered by the same Tier 1 datapill checks (`DP_VALID_JSON`,
  `DP_LHS_NO_FORMULA`, `DP_INTERPOLATION_SINGLE`, etc.) already applied to an if-block's
  `input.conditions[].lhs`. Previously this key was entirely unparsed and invisible to
  every rule, including the general datapill walk. `filter.{field}` is also now a valid
  custom-rule field path (see docs/rule-authoring.md#field-paths).

### Fixed

- `RETURN_RESPONSE_SCHEMA_CONSISTENT` suggested fix (#25): the previous text told
  authors to unify the schema across `return_response` blocks but omitted that any
  field a given block does not populate needs `"optional": true`. Followed verbatim
  it produced a recipe that lints clean but cannot activate (`success:false`,
  "can't be blank" per missing field). The fix text now names that requirement.

## [1.0.0] -- 2026-06-28

### Added

- Datapill path resolution (#22): `DP_PATH_RESOLVES` (Tier 3) flags a datapill whose `path`
  points at a field not declared in the referenced step's `extended_output_schema` — the
  "made-up field" failure mode that previously passed lint and only surfaced in the Workato
  UI as "Please replace invalid datapill(s)". Conservative and recipe-EOS-only: it validates
  only where the recipe materializes a schema and skips absent/open/dynamic schemas to avoid
  false positives on raw-JSON outputs. `warn` in `standard`, `error` in `strict`. Validating
  against connector-declared schemas (for steps with no recipe EOS) is a tracked follow-up.
- Loop structural validation for `repeat`/`while_condition` (#13): `REPEAT_NO_PROVIDER`
  and `WHILE_CONDITION_NO_PROVIDER` (Tier 1), `REPEAT_HAS_WHILE_CONDITION` and
  `WHILE_CONDITION_LAST_IN_REPEAT` (Tier 2). A malformed loop missing its
  `while_condition` child — which silently fails UI reconstruction after push —
  is now caught.
- `inside` containment clause for custom-rule step selectors (#14): match a step
  by where it sits in the recipe tree (e.g. `{ "provider": "logger", "inside":
  { "keyword": "repeat" } }`). Works in both rule-level `where` and
  `step_count.where`. Capped at one level; nested `inside` is rejected as
  `CUSTOM_RULE_INVALID`.
- Recipe tree-ancestry layer (`pkg/recipe`): `Parent`/`Ancestors`/`Children`/
  `StepByPointer` lookups derived from step JSON pointers, shared by the loop
  structural checks and the `inside` selector.

### Fixed

- `CATCH_LAST_IN_TRY` and `ELSE_LAST_IN_IF` previously never fired because the
  IGM-based implementation inspected the wrong graph children. They were migrated
  to the recipe tree layer and now correctly flag out-of-order catch/else blocks.

## [0.1.0] -- 2026-04-13

### Added

- Tier 0 schema validation (10 rules): JSON validity, required top-level keys,
  code/config structure, step field presence, UUID length checks
- Tier 1 step-level validation (16 rules): sequential numbering, UUID
  uniqueness and format, trigger numbering, filename matching, config provider
  checks, control flow keyword validation (if/else/catch/try), action name
  validation against connector allowlists
- Tier 1 datapill validation (7 rules): formula vs interpolation mode detection,
  JSON payload parsing, concatenation pattern checks, native connector body
  path validation, catch provider binding
- Tier 1 formula method validation (2 rules): allowlist-based validation against
  ~120 Workato formula methods with specific suggestions for common mistakes
- Tier 1 EIS validation (5 rules): extended_input_schema mirror checks against
  input fields, nested object matching, connector-internal field exclusion,
  output schema mirroring
- Tier 2 inter-step structure validation (7 rules): catch/else ordering within
  parent blocks, success response placement relative to catch, terminal response
  code coverage, control flow path termination, catch field completeness, recipe
  call zip_name presence
- Tier 3 cross-step data flow validation (4 rules): datapill alias resolution
  against step aliases, provider matching between datapill and resolved step,
  BFS-based reachability analysis, API endpoint trigger path validation
- Go port of IGM transformer (`pkg/igm/`) for Tier 2-3 graph analysis, with
  build-tag-gated snapshot conformance testing against the TypeScript
  implementation
- Profile system with two shipped profiles: `standard` (29 rules, baseline
  severities) and `strict` (extends standard, escalates 14 rules to errors),
  supporting single-parent inheritance
- `.wklintrc.json` project configuration: per-rule severity overrides
  (`off`/`info`/`warn`/`error`), file ignore patterns via globs, profile
  selection
- Connector-specific rule loading from `--skills-path` via `lint-rules.json`
  files (valid action names, connector-internal fields, custom action rules)
- Pre-push hook integration: `wk push` automatically lints `.recipe.json` files;
  errors block the push, warnings display but allow it, `--skip-lint` bypasses
- JSON-RPC plugin binary (`cmd/recipe-lint`) with `lint.run` and `lint.pre_push`
  methods
- CLI flags: `--skills-path`, `--config-path`, `--tiers`, `--profile`

### Fixed

- Pre-push hook type mismatch: hook was receiving empty file paths due to type
  mismatch with the CLI wire format
