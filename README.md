# wk lint

Recipe validation plugin for the `wk` CLI. Validates Workato recipe JSON files against 52 built-in rules across four tiers of analysis, from basic JSON schema checks through cross-step data flow validation. Runs as a standalone command or as an automatic pre-push gate on `wk push`.

Teams can extend the linter with [custom rules defined in JSON](docs/rule-authoring.md) — no Go code required.

## Installation

Requires the [`wk` CLI](https://github.com/workato-devs/wk-cli-beta).

### 1. Download and install

Download the archive for your platform from the [releases page](https://github.com/workato-devs/wk-lint-beta/releases).

| OS | Architecture | Archive |
|----|-------------|---------|
| macOS | Apple Silicon (M1+) | `wk-lint_VERSION_Darwin_arm64.tar.gz` |
| macOS | Intel | `wk-lint_VERSION_Darwin_x86_64.tar.gz` |
| Linux | x86_64 | `wk-lint_VERSION_Linux_x86_64.tar.gz` |
| Linux | ARM64 | `wk-lint_VERSION_Linux_arm64.tar.gz` |
| Windows | x86_64 | `wk-lint_VERSION_Windows_x86_64.zip` |
| Windows | ARM64 | `wk-lint_VERSION_Windows_arm64.zip` |

Extract the binary and move it onto your PATH.

**macOS / Linux:**

```bash
tar -xzf wk-lint_<version>_<OS>_<arch>.tar.gz
sudo mv recipe-lint /usr/local/bin/
```

**Windows (PowerShell):**

```powershell
Expand-Archive wk-lint_<version>_Windows_<arch>.zip -DestinationPath .
Move-Item recipe-lint.exe "$env:LOCALAPPDATA\Microsoft\WindowsApps\"
```

### 2. Verify

```bash
which recipe-lint   # macOS / Linux
where recipe-lint   # Windows
```

### 3. Register as a `wk` plugin

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
