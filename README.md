# wk lint

> 🟢 **Beta — Design Partner Program**

Recipe validation plugin for the `wk` CLI. Validates Workato recipe JSON files against 52 built-in rules across four tiers of analysis, from basic JSON schema checks through cross-step data flow validation. Runs as a standalone command or as an automatic pre-push gate on `wk push`.

Teams can extend the linter with [custom rules defined in JSON](docs/rule-authoring.md) — no Go code required.

## Installation

Install the `wk` CLI and recipe linter together using the [Workato Labs install guide](https://github.com/workato-devs/labs#install-the-toolkit) (Homebrew, Scoop, or manual).

After installing, register the plugin:

```bash
wk plugins install recipe-lint
wk lint --help
```

## Usage

```bash
# Lint a single recipe
wk lint recipe.json

# Lint all recipes in a directory
wk lint ./recipes/

# Run only schema and step-level rules (Tiers 0-1, fast)
wk lint recipe.json --tiers 0,1

# Use the strict profile
wk lint recipe.json --profile strict

# Load connector-specific rules from a skills directory
wk lint recipe.json --skills-path ./skills/
```

### Flags

| Flag | Type | Description |
|------|------|-------------|
| `--skills-path` | string | Path to connector skills directory (loads `lint-rules.json` files) |
| `--config-path` | string | Path to `.wklintrc.json` configuration file |
| `--tiers` | int array | Tier levels to run (e.g., `0,1`). Default: all |
| `--profile` | string | Lint profile to use (overrides `.wklintrc.json` setting) |

Also runs automatically as a pre-push gate on `wk push` — errors block the push, warnings display but don't block, `--skip-lint` to bypass.

## Tiers

Rules are organized into four tiers based on the analysis context they require.

| Tier | What it checks | Tool |
|------|---------------|------|
| **0** | JSON structure — required keys, field types, step shape | Struct validation |
| **1** | Each step independently — numbering, UUIDs, datapill syntax, EIS, formulas | Tree walker |
| **2** | Relationships between steps — control flow ordering, terminal coverage, path completeness | Control flow graph |
| **3** | Cross-step data flow — datapill reference resolution, provider matching, reachability | Graph + alias map |

If the control flow graph fails to build, Tiers 2-3 are skipped with a warning. Tiers 0-1 always run.

See the [Rule Reference](docs/rule-reference.md) for every rule ID, description, and default severity.

## Custom Rules

Teams can author custom lint rules as JSON — structural constraints, naming conventions, field requirements, architectural patterns. Rules run through the same tier engine as built-in rules.

```json
{
  "version": "0.2.0",
  "rules": [{
    "rule_id": "MAX_ONE_ACTION",
    "tier": 1, "level": "error",
    "message": "Recipe must have at most one action step",
    "scope": "recipe",
    "assert": { "step_count": { "where": { "keyword": "action" }, "max": 1 } }
  }]
}
```

Rules are discovered from two locations:
- **Connector skills**: `lint-rules.json` files loaded via `--skills-path`
- **Project rules**: `.wklint/rules/*.json` auto-discovered from the project root

See the [Rule Authoring Guide](docs/rule-authoring.md) for the full matcher reference, profiles, configuration, and examples.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | No errors (warnings are OK) |
| 1 | One or more lint errors found |
| 2 | Invalid input (file not found, not valid JSON) |

## Docs

- [Rule Reference](docs/rule-reference.md) — every built-in rule ID, description, and default severity
- [Rule Authoring Guide](docs/rule-authoring.md) — profiles, `.wklintrc.json` configuration, custom rule authoring
- [Architecture Decisions](docs/adr/) — tiered architecture, formula validation, profile system design
- [Linter Workflow Guide](https://github.com/workato-devs/recipe-skills/blob/main/docs/cli-guidance.md) — skills + linter integration patterns
