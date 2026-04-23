# Lint Profile System

**Author:** Zayne Turner
**Status:** Implemented
**Date:** March 9, 2026
**References:** ADR 0001 (Tiered Lint Architecture)

---

## Context

The linter has a flat severity model: each rule has a hardcoded default severity, and `.wklintrc.json` provides per-rule overrides. This works for individual projects, but different organizational contexts need fundamentally different lint postures — a team building internal integrations wants a relaxed baseline, while a team publishing shared connector recipes needs strict validation of every EIS field and datapill reference.

Achieving this today requires manually enumerating 10-20 rule overrides in every project's `.wklintrc.json`. This creates three problems:

1. **Boilerplate.** Every new project copies the same block of overrides. Drift between copies is inevitable.
2. **No vocabulary.** There's no way to say "use the strict posture" — only "set these 15 rules to these 15 values." Teams can't communicate lint posture in a single term.
3. **No upgrade path.** When new rules are added to the linter, existing `.wklintrc.json` files don't pick up appropriate severity for the new rules. A "strict" project stays strict only for the rules it knew about at config time.

Profiles add a named layer of severity mappings between hardcoded defaults and per-project overrides, enabling teams to adopt a lint posture with one line.

---

## Decision

### 1. Profiles describe lint posture, not organizational context

Profile names describe the quality bar they enforce, not who uses them. Names like `inner_source` or `open_source` leak organizational structure into tooling — they couple the profile name to an org chart that changes, and they conflate "who" with "how strict."

The ADR defines the naming convention: profile names are lowercase, underscore-separated, and describe the validation posture (e.g., `standard`, `strict`). Specific profile names and their rule-severity mappings are deferred to implementation — this ADR defines the mechanism, not the content.

### 2. Profiles are always external JSON data, never compiled into the binary

Both Workato-shipped and customer-defined profiles are JSON files loaded from disk at lint time. Disk-based profiles remain the primary mechanism and can be updated independently of the binary.

**Amendment (Issue #8):** Built-in profiles (`standard`, `strict`) are also embedded in the binary via `go:embed` as a fallback. Without this, profiles are not discoverable on release binaries because the `profiles/` directory was not included in GoReleaser archives — making the entire profile system non-functional on distributed builds. The precedence chain is: embedded defaults < plugin-bundled disk profiles < project-level disk profiles. Disk profiles of the same name fully replace the embedded version, preserving the decoupling principle for updates.

This is consistent with ADR 0001's treatment of connector-specific rules: `lint-rules.json` files are external data loaded at runtime. Profiles follow the same pattern, with the embedded layer providing a guaranteed baseline.

### 3. Two-tier profile discovery

Profiles are discovered from two locations, in precedence order:

1. **Project-level** — `.wklint/profiles/*.json`
   - Version-controlled alongside the project
   - Visible to newcomers (they can see what profiles exist)
   - Team-specific customization without affecting other projects

2. **Plugin-bundled** — `~/.wk/plugins/recipe-lint/profiles/`
   - Shipped alongside the plugin binary as separate JSON files
   - Updated via `wk plugins update recipe-lint`
   - Provides Workato-shipped defaults (e.g., `standard.json`, `strict.json`)

Project profiles take precedence over plugin-bundled profiles of the same name. This lets teams fork a shipped profile by placing a modified copy in `.wklint/profiles/`.

```
# Project-level profiles
my-project/
├── .wklint/
│   └── profiles/
│       └── team_api.json       # custom profile for this project
├── .wklintrc.json              # references: "profile": "team_api"
└── recipes/

# Plugin-bundled profiles
~/.wk/plugins/recipe-lint/
├── recipe-lint                 # plugin binary
└── profiles/
    ├── standard.json           # shipped by Workato
    └── strict.json             # shipped by Workato
```

### 4. Customer profiles use the same mechanism as shipped profiles

There is no special syntax for custom profiles. A profile is a JSON file in one of the discovery directories. Customers create their own by adding a file to `.wklint/profiles/`. This eliminates the need for inline profile definition in `.wklintrc.json` and keeps a single, consistent profile format.

### 5. Profile schema

A profile is a JSON file with the following schema:

```json
{
  "name": "strict",
  "description": "Strict validation for published recipes",
  "extends": "",
  "rules": {
    "UUID_DESCRIPTIVE": "error",
    "DP_INTERPOLATION_SINGLE": "error",
    "FORMULA_METHOD_INVALID": "error"
  }
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Profile identifier. Must match the filename (without `.json`). Lowercase, underscores only. |
| `description` | string | no | Human-readable description of the profile's purpose. |
| `extends` | string | no | Name of a parent profile. Empty string or absent means no parent. |
| `rules` | `map[string]string` | yes | Rule ID → severity. Values: `"off"`, `"info"`, `"warn"`, `"error"`. Only rules that differ from the parent (or hardcoded defaults) need to be listed. |

Go types:

```go
// ProfileDef is the on-disk representation of a lint profile.
type ProfileDef struct {
    Name        string            `json:"name"`
    Description string            `json:"description,omitempty"`
    Extends     string            `json:"extends,omitempty"`
    Rules       map[string]string `json:"rules"`
}

// ResolvedProfile is a fully flattened profile after inheritance resolution.
type ResolvedProfile struct {
    Name  string            // leaf profile name
    Chain []string          // e.g., ["strict", "standard"] — leaf first
    Rules map[string]string // merged rule severities, leaf wins
}
```

### 6. Profile selection

Profile selection uses the `"profile"` field in `.wklintrc.json`, overridable by the `--profile` CLI flag:

```json
{
  "version": "0.1.0",
  "profile": "strict",
  "rules": {
    "FILENAME_MATCH": "off"
  }
}
```

| Source | Precedence |
|---|---|
| `--profile` CLI flag | Highest — overrides `.wklintrc.json` |
| `"profile"` field in `.wklintrc.json` | Default |
| No profile specified | No profile layer applied (identical to today) |

The `--profile` flag is added to `plugin.toml`:

```toml
[[commands.flags]]
name = "profile"
description = "Lint profile to use (overrides .wklintrc.json)"
type = "string"
```

### 7. Layered severity resolution

Severity for a rule is resolved through three layers, each overriding the previous:

```
hardcoded default → profile chain → .wklintrc.json rules
```

The resolution algorithm:

```
function resolveSeverity(ruleID):
    severity = hardcodedDefault(ruleID)

    if profile is active:
        resolved = resolveProfileChain(profileName)
        if ruleID in resolved.Rules:
            severity = resolved.Rules[ruleID]

    if ruleID in wklintrc.rules:
        severity = wklintrc.rules[ruleID]

    return severity
```

This means:
- A profile sets the baseline posture for the project
- `.wklintrc.json` `rules` overrides provide per-project exceptions on top of the profile
- If no profile is active, behavior is identical to today (hardcoded + `.wklintrc.json`)

### 8. Single-parent inheritance with depth limit

Profiles support single-parent inheritance via the `extends` field. A profile inherits all rule severities from its parent, then applies its own overrides on top.

```json
// standard.json
{
  "name": "standard",
  "rules": {
    "UUID_DESCRIPTIVE": "warn",
    "DP_INTERPOLATION_SINGLE": "warn"
  }
}

// strict.json — extends standard, escalates to error
{
  "name": "strict",
  "extends": "standard",
  "rules": {
    "UUID_DESCRIPTIVE": "error",
    "DP_INTERPOLATION_SINGLE": "error",
    "FORMULA_METHOD_INVALID": "error"
  }
}
```

Resolving `strict`: start with hardcoded defaults, apply `standard` rules, then apply `strict` rules. The chain is `[strict, standard]`.

**Constraints:**

- **Max depth: 5.** An inheritance chain longer than 5 is a configuration error. The loader rejects it with a clear error message naming the chain.
- **Cycle detection.** The loader tracks visited profile names during chain resolution. If a name appears twice, it rejects with an error naming the cycle (e.g., `"profile cycle detected: a → b → a"`).
- **Single parent only.** No multiple inheritance. A profile has zero or one parent. This keeps resolution deterministic and the mental model simple.

Resolution pseudocode:

```
function resolveProfileChain(name, discovered, visited):
    if name in visited:
        error("profile cycle detected: " + join(visited, " → ") + " → " + name)
    if len(visited) >= 5:
        error("profile chain too deep (max 5): " + join(visited, " → "))

    visited.append(name)
    def = discovered[name]
    if def is nil:
        error("profile not found: " + name)

    // Recurse to parent first
    var parentRules map[string]string
    if def.Extends != "":
        parent = resolveProfileChain(def.Extends, discovered, visited)
        parentRules = parent.Rules
    else:
        parentRules = {}

    // Merge: child overrides parent
    merged = copy(parentRules)
    for ruleID, severity in def.Rules:
        merged[ruleID] = severity

    return ResolvedProfile{
        Name:  name,
        Chain: visited,
        Rules: merged,
    }
```

### 9. Backwards compatibility

No `profile` field in `.wklintrc.json` = no profile layer = severity resolution is `hardcoded default → .wklintrc.json rules`, identical to today's behavior. Existing configurations work without modification.

The `Profile` field on `LintConfig` is a pointer or empty string — zero value means "no profile." The `--profile` flag defaults to empty.

---

## Implementation

### Changes to existing code (implemented)

| File | Change |
|---|---|
| `pkg/lint/config.go` | Added `Profile string` field to `LintConfig`. |
| `pkg/lint/profiles.go` | **New file.** `ProfileDef` and `ResolvedProfile` types. Profile discovery (`discoverProfiles`), loading (`loadProfilesFromDir`), chain resolution (`resolveProfileChain`) with cycle detection and max depth 5. |
| `pkg/lint/profiles_test.go` | **New file.** 9 test cases: no-parent, single/multi-level inheritance, cycle detection, depth limit, missing parent, empty rules, project-overrides-plugin, name-must-match-filename. |
| `pkg/lint/lint.go` | Added `Profile` and `PluginDir` fields to `LintOptions`. Profile resolution between config loading and tier execution (CLI flag > config file > none). Replaced `applyCfgOverrides` with `applyOverrides` (profile layer first, then config layer). |
| `cmd/recipe-lint/main.go` | Added `Profile` and `PluginDir` fields to `lintRunParams` and `prePushParams`. Passed through to `LintOptions`. |
| `plugin.toml` | Added `--profile` flag definition under `[[commands.flags]]`. |
| `profiles/standard.json` | **New file.** Baseline profile — schema rules as errors, action/formula rules as warnings. |
| `profiles/strict.json` | **New file.** Extends `standard`, escalates action/formula rules to errors. |

### Modified severity resolution in `lint.go`

The current `applyCfgOverrides` function applies `.wklintrc.json` rule overrides directly to diagnostics after they're emitted. With profiles, the resolution order becomes:

```go
// applyOverrides replaces applyCfgOverrides. It applies profile severities
// first, then .wklintrc.json overrides on top.
func applyOverrides(diags []LintDiagnostic, profile *ResolvedProfile, cfg *LintConfig) []LintDiagnostic {
    for i := range diags {
        ruleID := diags[i].RuleID
        severity := diags[i].Level // hardcoded default (set by the rule)

        // Layer 2: profile
        if profile != nil {
            if s, ok := profile.Rules[ruleID]; ok {
                severity = s
            }
        }

        // Layer 3: .wklintrc.json
        if cfg != nil {
            if s, ok := cfg.Rules[ruleID]; ok {
                severity = s
            }
        }

        diags[i].Level = severity
    }
    return diags
}
```

### Profile discovery implementation

```go
// discoverProfiles loads all profile definitions from both discovery locations.
// Project-level profiles override plugin-bundled profiles of the same name.
func discoverProfiles(projectRoot, pluginDir string) (map[string]*ProfileDef, error) {
    profiles := make(map[string]*ProfileDef)

    // Load plugin-bundled profiles first (lower precedence)
    pluginProfileDir := filepath.Join(pluginDir, "profiles")
    if err := loadProfilesFromDir(pluginProfileDir, profiles); err != nil {
        return nil, err
    }

    // Load project-level profiles (higher precedence, overwrites)
    projectProfileDir := filepath.Join(projectRoot, ".wklint", "profiles")
    if err := loadProfilesFromDir(projectProfileDir, profiles); err != nil {
        return nil, err
    }

    return profiles, nil
}

func loadProfilesFromDir(dir string, into map[string]*ProfileDef) error {
    entries, err := os.ReadDir(dir)
    if err != nil {
        if os.IsNotExist(err) {
            return nil // directory doesn't exist — not an error
        }
        return err
    }
    for _, entry := range entries {
        if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
            continue
        }
        data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
        if err != nil {
            return err
        }
        var def ProfileDef
        if err := json.Unmarshal(data, &def); err != nil {
            return fmt.Errorf("invalid profile %s: %w", entry.Name(), err)
        }
        into[def.Name] = &def
    }
    return nil
}
```

### Implementation sequencing (completed)

All steps implemented in a single pass:

1. **Types and loading** — `ProfileDef`, `ResolvedProfile`, `discoverProfiles`, `resolveProfileChain` in `pkg/lint/profiles.go`. Unit tests in `pkg/lint/profiles_test.go`.
2. **Lint pipeline** — `Profile` added to `LintConfig` and `LintOptions`. `applyCfgOverrides` replaced with `applyOverrides`. All existing tests pass (no profile = no change).
3. **CLI integration** — `--profile` flag added to `plugin.toml` and both `lintRunParams`/`prePushParams`.
4. **Initial profiles** — `profiles/standard.json` and `profiles/strict.json` shipped at repo root (installed alongside plugin binary).

---

## Consequences

- **One-line posture adoption.** Teams set `"profile": "strict"` instead of enumerating 15 rule overrides. New rules added to the linter automatically inherit the profile's severity.
- **Profile updates decouple from binary releases.** Because profiles are external JSON, adjusting a shipped profile is a `wk plugins update` — no new binary required. Consistent with ADR 0001's principle of external data files.
- **Customer extensibility without special syntax.** Custom profiles use the same format and discovery as shipped profiles. No new concepts to learn.
- **Inheritance keeps profiles DRY.** `strict` extends `standard` rather than duplicating all of `standard`'s mappings. The depth limit (5) and cycle detection prevent pathological chains.
- **Backwards compatible.** Existing `.wklintrc.json` files without a `profile` field behave identically to today. No migration required.
- **Discovery complexity.** Two discovery locations (project + plugin-bundled) add a "where did this profile come from?" question. Mitigated by the simple precedence rule: project wins. Future: `wk lint --explain-profile` could show the resolved chain and source locations.
- **No inline profiles.** Profiles cannot be defined inline in `.wklintrc.json`. This is deliberate — it forces profiles to be reusable, named, and discoverable — but it means creating a custom profile requires creating a file in `.wklint/profiles/`, not just editing `.wklintrc.json`.
