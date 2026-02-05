# REFACTOR_PLAYBOOK.md

A **generic, phase-gated process** for executing high-risk refactors safely.

This playbook is designed to constrain automation (e.g., Codex, Codex CLI) so that
structural changes can be made **without changing behavior**.

This document defines **process**, not domain semantics.

---

## Core guarantees

- **Behavior is defined before code is moved**
- **Correctness is enforced by tests, not assurances**
- **Phase transitions are human-controlled**
- **Automation executes, never decides**
- **Cleanup is optional and abortable**

---

## Roles

- **Humans**
    - Define correctness
    - Advance phase status
    - Approve artifacts

- **Automation**
    - Executes tasks *only when authorized*
    - Refuses work when phase gates are closed
    - Reports changes and test results

---

## Control structure

Each refactor MUST define a **control directory** containing:

- a **control document** (phase status, constraints, checklists)
- one or more **semantic artifacts** (domain-specific definitions)
- a **test plan** that enforces semantic equivalence

The exact names and contents of these artifacts are project-specific.

Automation MUST treat the control directory as authoritative.

---

## Phase model

### Phase 0 — Observation

**Goal:** understand the system without changing it.

Allowed
- reading code
- documenting duplication and risks
- proposing a phased plan

Forbidden
- refactors
- cleanup
- structural changes

Deliverables
- observations
- duplication inventory
- risk assessment

---

### Phase 1 — Semantic definition

**Goal:** define what “correct behavior” means.

This phase produces **documents and test plans**, not code changes.

Allowed
- documentation
- test design

Forbidden
- refactors
- extraction or consolidation
- behavior changes
- structural reorganization

Exit criteria
- correctness is explicitly defined
- equality rules are unambiguous
- test plan can detect divergence

---

### Phase 2 — Consolidation

**Goal:** remove duplication without changing behavior.

Rules
- one step at a time
- minimal diffs
- no behavior, defaults, or validation changes
- tests run after every step
- stop immediately on failure

Phase 2 is **not complete** until:
- equivalence tests exist
- equivalence tests pass

---

### Phase 3 — Cleanup (optional)

**Goal:** improve clarity without affecting behavior.

This phase is optional and frequently skipped.

Allowed
- dead code removal
- renaming unexported/private identifiers
- comment clarification

Forbidden
- new abstractions
- logic changes
- package or API redesign
- performance optimization

Valid outcome:
- “No safe cleanup identified”

---

## Phase gating

Phase status MUST be explicit and human-controlled, for example:

- BLOCKED
- READY
- ACTIVE
- COMPLETE
- SKIPPED

Automation MUST refuse to proceed if the phase state does not authorize work.

---

## Test enforcement

Refactors are accepted only when tests prove:

- equivalent behavior across execution paths
- identical handling of edge cases
- preserved error and validation behavior

Tests replace trust.

---

## Automation contract

When using automation:

1. **Read control documents**
2. **Confirm phase authorization**
3. **Execute a single scoped task**
4. **Run tests**
5. **Report results**
6. **Stop**

Automation MUST NOT:
- advance phases
- weaken constraints
- redefine correctness

---

## When to stop

Stop immediately if:
- behavior cannot be proven equivalent
- tests fail or are insufficient
- automation proposes “improvements”
- scope begins to expand

Stopping is a valid and correct outcome.

---

## Reuse guidance

This playbook is reusable.

To apply it to a new refactor:
1. Copy this file
2. Create a project-specific control directory
3. Instantiate semantic artifacts for that system
4. Execute phases deliberately

