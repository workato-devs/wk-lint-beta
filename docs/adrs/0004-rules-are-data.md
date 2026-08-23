# Rules Are Data, Not Code

**Author(s):** Zayne Turner, Claude [role: assistant; harness: Claude Code; model: Opus 4.8]
**Amended-by:** Codex [role: assistant; harness: Codex; model: GPT-5] — August 2026
**Amended-by:** Claude [role: assistant; harness: Claude Code; model: Opus 4.8], dir. Zayne Turner — June 2026

**Status:** Accepted
**Date:** June 17, 2026
**Implemented:** Partially — see Consequences
**References:** ADR 0001 (Tiered Validation Architecture); Labs repo `labs/docs/adrs/LABS-0001-skills-lint-rules-as-data.md` (skills-side rules contract)

**Amendments:**
- August 2026 — Added connector-declared action output schemas to the cross-repo `lint-rules.json` contract; retired the data-defined `TRY_NO_AS` false positive
- June 2026 — Recorded the `lint-rules.json` contract's downstream consumer (the agent-skills repo) and pointed to the Labs-repo ADR that owns that cross-repo decision.
- June 2026 — Added the `check_return_response_schema` builtin (issue #17), taking the builtin count from 27 to 28; recorded that issue #18 is addressed as recipe-skills allowlist data (no rule in this binary).
- June 2026 — Added the `DP_PATH_RESOLVES` rule (issue #22), backed by Go via the existing `check_dataflow` builtin (no new `RegisterBuiltin`; registered-builtin count stays 28). Grows the Go-backed rule-ID surface — debt counted deliberately.

---

## Context

This decision originated in the **original product requirements**, well before ADR-0001 was
written, but it was never captured as its own decision record. That omission is the reason this ADR
exists now, and it is the honest subject of the record below.

The PRD set a specific, load-bearing bar:

> **Changing the rules a customer runs must not require cutting a new Go binary.**

The intent was operational: connector teams and customers should be able to add, tune, and ship
lint rules as *data* — on their own cadence — without a release of `recipe-lint` itself. A rule was
meant to be a JSON artifact, not a code change.

Because this bar was never written down as an ADR, it was not visible as a hard constraint during
scoping. The early scoping work (done by an agent) interpreted the absence of a stated requirement
as license to treat "rules are data" as an aspiration rather than a contract, and leaned on a "no
DSL" argument to justify implementing non-trivial rules directly in Go. That interpretation went
unchallenged precisely because there was no decision record to check it against — the exact failure
mode the ADR convention exists to prevent (ADR-0000: *verify before relying*).

## Decision

**Every rule — built-in or custom — has a JSON definition.** Customers and connector teams see a
single uniform rule catalog and author rules as data.

Two mechanisms back this:

1. **A declarative assertion vocabulary**, interpreted at runtime in `pkg/lint/eval.go`:
   `field_exists`, `field_absent`, `field_matches`, `field_equals`, `step_count`, `eis_empty`,
   `eis_field_type`, `all_of`, `any_of`, `not`. Rules expressible in this vocabulary require **no
   new binary** — they load from embedded `builtin_rules.json`, from skills `lint-rules.json`
   (`--skills-path`), or from project `.wklint/rules/*.json`.

2. **A `builtin` assertion** that delegates to a Go function registered in the binary
   (`pkg/lint/builtin_registry.go`). This is the escape hatch for logic the declarative vocabulary
   cannot yet express (control-flow analysis over the IGM graph, datapill tracing, etc.).

The principle is that the declarative path is the default and the `builtin` path is the exception.

## Alternatives considered

- **A full rule DSL.** Rejected ("no DSL"). A bespoke expression language is a large surface to
  design, document, version, and secure, and most rules don't need it. *This rejection is sound on
  its own merits — the failure was not the "no DSL" conclusion, but using it as cover to skip the
  declarative-data layer entirely and route complex rules straight into Go.*
- **All rules as Go code.** Rejected: directly violates the PRD bar; every connector rule change
  would force a `recipe-lint` release.

## Consequences

**Honest status of the PRD bar: partially met, and that gap traces directly to this ADR's absence.**

- Rules expressible in the declarative vocabulary meet the bar fully: they ship as data, no binary.
- Any rule needing a `builtin` assertion does **not** meet the bar — it requires a Go function
  compiled into the binary. As of this writing there are **27 registered builtins**
  (`grep -rh 'RegisterBuiltin("' pkg/lint/*.go | grep -v '__tier0__'`, excluding the no-op
  placeholder), and they back the bulk of the non-trivial checks. In practice, "add a real rule"
  still often means "cut a new binary," which is the outcome the PRD set out to avoid.
- The drift was gradual and unchallenged because there was no record stating the bar. Had this ADR
  existed, the volume of `builtin`-backed rules would have been visible as debt against an explicit
  constraint, not an unexamined default.

**Follow-on work (deliberately surfaced, not yet decided):**

- Decide whether closing the gap means *expanding the declarative vocabulary* (more assertion types
  so fewer rules need Go) or *accepting a bounded set of builtins as legitimate* and documenting
  which rule classes are permitted to be code. Either is a real decision and should get its own ADR
  or an amendment here — not another silent default.
- Track the builtin count as a debt metric against the PRD bar.

> **Amendment (June 2026): the `lint-rules.json` contract has a downstream consumer — the agent-skills repo.**
> The original record above treats "skills `lint-rules.json` (`--skills-path`)" only as one of the
> *sources* a rule can load from. That understates the relationship. The agent-skills repo declares,
> per skill, which lint rules apply to it (e.g. `skills/workato-recipes/variable/lint-rules.json`) —
> a decision that repo took on given this linter's existence, to make its pre-commit checks easier.
> That makes the skills repo the **living proof of this ADR's bar**: rules shipping as data, on a
> separate repo's own cadence, with no `recipe-lint` binary cut.
>
> The consequence for anyone amending this ADR: the declarative assertion vocabulary and the
> `lint-rules.json` shape are a **contract with a standing downstream consumer.** Changing or
> removing assertion types, or altering how `--skills-path` files are read, has a blast radius
> beyond this repo. The cross-repo decision itself — what the skills repo declares and why — is owned
> and recorded in the Labs repo (`labs/docs/adrs/LABS-0001-skills-lint-rules-as-data.md`), not here;
> this ADR governs only the linter-side schema it consumes.

> **Amendment (June 2026): builtin count is now 28 (issue #17); issue #18 is being addressed as skills data, not in this binary.**
> The Consequences section above states "27 registered builtins" as of its writing. Issue #17 moved
> that number to **28** — verify with
> `grep -rh 'RegisterBuiltin("' pkg/lint/*.go | grep -v '__tier0__' | wc -l`.
>
> - **Issue #17** (`return_response` EIS↔EOS schema parity) added **one** builtin,
>   `check_return_response_schema` (backing `RETURN_RESPONSE_SCHEMA_PARITY`,
>   `RETURN_RESPONSE_SCHEMA_CONSISTENT`, `RETURN_RESPONSE_INPUT_MIRROR`). It is a legitimate use of
>   the escape hatch: structural deep-comparison of two schemas by name/type/nesting, and cross-step
>   set-identity, are not expressible in the current assertion vocabulary. This is debt against the
>   PRD bar, counted here deliberately.
> - **Issue #18** (`provider: workato` / `set_variable[s]` is UI-unsupported) added **nothing to this
>   repo** — by design. A first pass shipped a bespoke `WORKATO_PROVIDER_UNSUPPORTED` rule here that
>   used a forced-failure `where`/`field_absent` trick to catch two hardcoded names; it was reverted
>   as heterogeneous and too narrow. The homogeneous fix is **skills data**: declare the `workato`
>   provider's `valid_action_names` in a recipe-skills `lint-rules.json`, and the existing
>   `ACTION_NAME_VALID` rule (`tier1_steps.go`) flags every invalid `workato` action with no
>   `recipe-lint` change. That is the PRD bar working as intended — a real rule shipping as data on
>   the skills repo's cadence — and is tracked by recipe-skills#4. The lesson:
>   when an "invalid action for a provider" gap appears, the answer is connector allowlist data, not
>   a new rule in this binary.

> **Amendment (June 2026): `DP_PATH_RESOLVES` (issue #22) is a new Go-backed rule — debt against the bar, but the registered-builtin count is unchanged.**
> Issue #22 adds `DP_PATH_RESOLVES`: a datapill's `path` must resolve to a field declared in
> the referenced step's `extended_output_schema`. Resolving a path against a nested EOS tree
> (descending `properties`, skipping array indices, stopping at open containers) is not
> expressible in the current declarative assertion vocabulary, so it is a legitimate use of
> the `builtin` escape hatch — and therefore debt against the PRD bar, counted here.
>
> - **Debt-metric precision.** This rule reuses the **existing** `check_dataflow` builtin (it
>   adds a Go function, `checkDPPathResolves`, routed through that builtin, not a new
>   registration). So the **registered-builtin count stays 28** — verify with
>   `grep -rh 'RegisterBuiltin("' pkg/lint/*.go | grep -v '__tier0__' | wc -l`. What grows is
>   the count of **Go-backed rule IDs** / the code-based rule surface. Don't read the
>   unchanged `RegisterBuiltin` count as "no new code debt" — the two metrics differ.
> - **Debt was deliberately bounded.** v1 is **recipe-EOS-only** and conservative: it validates
>   only where the recipe materializes a schema and stops-and-accepts on absent/open/dynamic
>   schemas (no false positives on raw-JSON outputs). The wider goal — validating `path`
>   against **connector-declared** output schemas (from `--skills-path` `lint-rules.json`,
>   `ConnRules`) for steps with no recipe EOS — is deferred to a tracked v2 follow-up. That v2
>   is a candidate for the "expand the declarative vocabulary vs. accept bounded builtins as
>   legitimate" decision this ADR already flagged under Follow-on work, since connector-schema
>   data is exactly the "rules as data" path.

> **Amendment (August 2026): connector output contracts complete the bounded `DP_PATH_RESOLVES` v2 path.**
> Live Workato REST connector metadata and stopped canonical recipe exports established two
> simultaneous facts: the connector exposes stable transport outputs such as `status_code`, and a
> recipe export can contain a non-empty but partial EOS that omits those connector-owned roots.
> Therefore the earlier fallback-only proposal (use connector data only when EOS is absent) was
> insufficient. The additive cross-repo contract now supports per-action `static` or `dynamic`
> schemas plus explicit `intrinsic_fields`. Recipe EOS remains authoritative for roots it declares;
> audited intrinsic roots may augment it; static connector schemas close absent EOS; dynamic
> schemas validate known intrinsics while accepting unknown runtime output conservatively.
> `ConnectorRules` stores each action declaration as raw JSON so one malformed or unknown action
> does not disable sibling connector data. Triggers remain unchanged. The rule continues to use
> the existing `check_dataflow` builtin, so registered-builtin count does not grow.
> The same additive contract includes `action_internals` for connector-owned input fields that are
> specific to one action; provider-wide `connector_internals` remains compatible. This prevents an
> input exemption proven for one action from silently weakening EIS checks on sibling actions or
> triggers.
>
> The same evidence retired `TRY_NO_AS`. It was declarative data, but it asserted a platform
> invariant contradicted by current canonical exports, which legitimately contain generated `try`
> aliases. Removing a false rule is consistent with the rules-as-data contract; connector/runtime
> evidence, not a stale preference, controls the catalog.

<!-- When this evolves, add a dated amendment in place; do not rewrite the above. -->
