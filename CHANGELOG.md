# Changelog

All notable changes to the recipe-lint plugin will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
