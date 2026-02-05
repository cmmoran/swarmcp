# Housekeeping & Refactor Control Plan
-
- The service “build pipeline” (render → resolve mounts/labels/constraints → build spec) is implemented multiple times, creating parallel flows that can drift. (`internal/apply/services.go`, `internal/apply/status.go`, `internal/apply/stack_deploy.go`)
- `internal/config` mixes schema/validation with IO-heavy source loading and git logic, which blurs package boundaries and complicates reuse. (`internal/config/load.go`, `internal/config/git_sources.go`, `internal/config/overlays.go`)
- `internal/apply` acts as both domain orchestrator and spec formatter for Swarm and Compose, leading to mixed responsibilities. (`internal/apply/services.go`, `internal/apply/stack_deploy.go`, `internal/apply/status.go`)
- CLI commands own significant business logic (prune/preserve/confirmation behavior), duplicating patterns across commands. (`cmd/apply.go`, `cmd/diff.go`, `cmd/status.go`)

**Notable Smells & Duplication**
- Apply pipeline duplication across `internal/apply/services.go`, `internal/apply/status.go`, and `internal/apply/stack_deploy.go`: each builds rendered services, derives mounts/networks/labels/constraints, and computes intent/specs with slight differences.
- Policy validation + conversion duplicated between `internal/config/*_policy.go` and `internal/apply/*_policy.go` (restart/update/rollback): config validates + apply re-validates/normalizes, increasing divergence risk.
- Label/identity logic spread across `internal/render/render.go` (label schema), `internal/apply/services.go` (label filtering + reserved prefixes), and `internal/apply/plan.go`/`status.go` (managed checks). This is a hidden coupling that can diverge if labels change.
- Prune/preserve/confirm logic duplicated in CLI commands instead of centralized policy utilities. (`cmd/apply.go`, `cmd/diff.go`, `cmd/status.go`)
- Config loading + merging + source IO tightly coupled inside `internal/config`, rather than separated into “schema” vs “source loading” modules. (`internal/config/load.go`, `internal/config/imports.go`, `internal/config/git_sources.go`)

**Risk Assessment**
- Parallel service build paths will drift, leading to different behavior between apply, status, and stack deploy.
- Duplicate validation/conversion logic makes policy semantics brittle; updates must be made in multiple places.
- Label-based coupling risks silent breakage if label schema or filtering rules change.
- The config package’s mixed responsibilities will make future extensions (e.g., new sources or schema changes) harder to reason about and test.

**Targeted Improvement Opportunities**
- Centralize the service “render → intent → spec” pipeline into one internal package/function used by apply, status, and stack deploy.
- Consolidate update/restart policy normalization and validation into one package-level API and reuse it across config + apply.
- Define a single label/identity utility that owns managed-label checks, filtering, and formatting.
- Split `internal/config` into a lightweight schema/validation package and a separate source-loading package.

**Things That Are Surprisingly Well Done**
- The overlay and merge layering in `internal/config/overlays.go` is structured and explicit about precedence, which reduces ambiguity.
- The use of dedicated small packages (`internal/sliceutil`, `internal/yamlutil`, `internal/mergeutil`) shows intentional factoring of low-level helpers.

## Phase Status

- Phase 1: Convergence
    - Status: COMPLETE
    - Allowed:
        - Documentation
        - Test planning
    - Forbidden:
        - Refactors
        - Shared pipeline extraction
        - Package moves
        - Runtime behavior changes

- Phase 2: Consolidation
    - Status: COMPLETE

- Phase 3: Structural Cleanup
    - Status: SKIPPED (no safe cleanup identified)

## Phase 1 Artifacts Checklist

Phase 1 is complete when all items below are checked.

- [x] docs/housekeeping/intent_spec_contract.md
- [x] docs/housekeeping/policy_behavior_matrix.md
- [x] docs/housekeeping/label_identity_rules.md
- [x] docs/housekeeping/convergence_test_plan.md

## Phase 2: Consolidation

1. Phase Gate Confirmation
   - Phase 2 is ACTIVE per housekeeping.md.
2. Phase 2 Execution Plan
   - Establish a single authoritative service intent/spec builder used by apply, status, and stack deploy; do not change field semantics or defaulting; relies on docs/housekeeping/intent_spec_contract.md.
   - Replace ad‑hoc policy normalization/validation in apply with a single normalization API; do not change accepted inputs or error behavior; relies on docs/housekeeping/policy_behavior_matrix.md.
   - Consolidate label/identity checks and filtering into one utility API; do not change label keys, reserved prefixes, or drift rules; relies on docs/housekeeping/label_identity_rules.md.
   - Centralize prune/preserve/confirm behavior in a shared CLI policy helper; do not change prompts, defaults, or flags; relies on docs/housekeeping/convergence_test_plan.md to confirm identical outputs.
   - Run the convergence test plan after each consolidation step; do not proceed if any contract deviations are observed; relies on docs/housekeeping/convergence_test_plan.md.
3. Consolidation Map
   - Pipeline convergence: internal/apply/services.go, internal/apply/status.go, internal/apply/stack_deploy.go → single shared intent/spec builder.
   - Policy normalization: internal/config/*_policy.go and internal/apply/*_policy.go → single normalization/validation pathway.
   - Labels/identity: internal/render/render.go, internal/apply/services.go, internal/apply/plan.go, internal/apply/status.go → one utility for managed labels, filtering, and drift rules.
   - CLI behavior: cmd/apply.go, cmd/diff.go, cmd/status.go → shared prune/preserve/confirm policy helper.

4. Risks & Safeguards
   - Risk: Apply/status/stack deploy outputs diverge after consolidation; safeguard with fixtures from docs/housekeeping/convergence_test_plan.md.
   - Risk: Policy validation edge cases change; safeguard with the matrix in docs/housekeeping/policy_behavior_matrix.md.
   - Risk: Label filtering or managed checks drift; safeguard with docs/housekeeping/label_identity_rules.md and explicit label fixtures.
   - Risk: CLI behavior changes unintentionally; safeguard with prompt behavior tests and output comparisons from the convergence plan.

5. Phase 2 Readiness Checklist

- [x] docs/housekeeping/intent_spec_contract.md is complete and approved
- [x] docs/housekeeping/policy_behavior_matrix.md is complete and approved
- [x] docs/housekeeping/label_identity_rules.md is complete and approved
- [x] docs/housekeeping/convergence_test_plan.md is complete and approved
- [x] Baseline convergence tests are executable and passing on current code
