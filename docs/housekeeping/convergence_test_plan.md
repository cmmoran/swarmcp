# Convergence Test Plan

## 1. Purpose and Scope

Convergence tests MUST assert that apply, status, and stack deploy produce identical intent/spec behavior for the same inputs.

Out of scope:
- Performance testing and profiling.
- End-to-end runtime behavior in Docker Swarm.
- UI/CLI prompt behavior.

Relationship to Phase 1 semantic contracts:
- docs/intent_spec_contract.md defines the field contract and drift participation.
- docs/policy_behavior_matrix.md defines policy normalization and validation rules.
- docs/label_identity_rules.md defines label and identity behavior.

## 2. Convergence Definition

- “Identical behavior” means semantic equivalence of intent/spec outputs across apply, status, and stack deploy.
- Byte-for-byte equality is NOT required unless explicitly stated for a snapshot.
- Field-level comparison MUST follow the ordering and normalization rules in the Phase 1 documents.

## 3. Test Axes and Coverage Matrix

The test matrix MUST vary these dimensions independently:

### 3.1 Minimal Configurations
- Single stack, single service, no overlays.
- Only required fields present.
- No mounts, no policies, no custom labels.

### 3.2 Typical Configurations
- Multiple stacks with mixed shared/partitioned modes.
- Configs/secrets/volumes present.
- Restart/update/rollback policies present.
- User labels with template tokens.

### 3.3 Edge and Boundary Configurations
- Empty env/labels/ports lists.
- Zero-value policies (e.g., delay=0s, parallelism=0, max_failure_ratio=0).
- Large but valid label sets.
- Network ephemerals and derived networks.

Each axis MUST explicitly vary:
- Labels (managed vs user-defined)
- Mounts (configs/secrets/volumes)
- Networks (derived)
- Policies (restart/update/rollback)
- Identity (project/stack/partition/service)

## 4. Input Fixtures

Fixtures MUST include:
- Config inputs covering the axes above.
- Values files and overlays for at least one fixture.
- Source variants: local paths and rendered template sources; git sources MAY be included if deterministic.

Determinism requirements:
- All fixtures MUST be deterministic and stable across runs.
- Dynamic time-, hash-, or order-dependent values MUST be controlled or normalized.

## 5. Output Snapshots

Each fixture MUST produce snapshots for:
- Intent snapshots (canonical intent fields only).
- Spec snapshots (Swarm/Compose fields included in contract).
- Normalized policy representations for restart/update/rollback.
- Label sets after rendering, filtering, and managed-label injection.

## 6. Equality Semantics

- Ordering MUST be ignored for mounts, networks, and unordered label maps.
- Fields not included in docs/intent_spec_contract.md MUST be ignored for convergence checks.
- Policy comparisons MUST use normalized values per docs/policy_behavior_matrix.md.
- Labels MUST be compared after rendering, filtering, and managed-label injection per docs/label_identity_rules.md.

## 7. Failure Classification

Failures MUST be classified as one of:
- Contract violation: output differs from docs/intent_spec_contract.md.
- Normalization mismatch: output violates docs/policy_behavior_matrix.md.
- Drift detection regression: intent/spec equality deviates from contract rules.
- Test infrastructure failure: fixture or harness failure unrelated to semantics.

## 8. Execution Expectations

- Convergence tests MUST run on every change that touches apply, status, or stack deploy logic.
- Phase 2 MUST NOT begin unless all convergence tests pass.
- A single contract-violation failure MUST block Phase 2 progression.
- Partial failures MUST be treated as blocking until resolved or reclassified.
