# Label and Identity Rules

## 1. Purpose and Scope

This document defines canonical rules for labels and service identity that apply, status, and stack deploy MUST follow identically.

Relationship to docs/intent_spec_contract.md:
- This document defines label and identity semantics referenced by the intent/spec contract.
- Field inclusion and drift participation are defined in docs/intent_spec_contract.md and are authoritative.

Out of scope:
- Configuration loading, overlays, and template sources.
- Non-service label usage (e.g., node labels) unless explicitly stated.
- Runtime orchestration behavior.

## 2. Label Categories

- Managed labels: labels owned by swarmcp and derived from service identity or system metadata.
- User-defined labels: labels provided by configuration and rendered templates.
- Derived/computed labels: any labels computed from identity or runtime context; these are managed labels for contract purposes.

## 3. Managed Label Rules

- Managed labels are part of the canonical label set and MUST be set by apply and status.
- Stack deploy MUST preserve managed labels when rendering compose specs.
- Managed labels MUST NOT be overridden or mutated by user-defined labels.
- Managed labels MUST participate in drift detection.

The managed label set MUST include at least the following keys:
- swarmcp.io/managed
- swarmcp.io/name
- swarmcp.io/hash
- swarmcp.io/project
- swarmcp.io/partition
- swarmcp.io/stack
- swarmcp.io/service

## 4. Reserved Prefix Rules

- Reserved prefixes: swarmcp.io/
- User-defined labels MUST NOT use reserved prefixes.
- Any user-defined label key that uses a reserved prefix MUST be rejected with an error.
- Reserved-prefix enforcement MUST be consistent across apply, status, and stack deploy.

## 5. Label Rendering and Filtering

- User-defined label keys and values MAY contain template tokens and MUST be rendered before validation and merge.
- Label keys MUST be validated after rendering.
- Managed labels MUST be injected after user-defined label rendering and MUST take precedence.
- Filtering MUST remove any user-defined labels that violate reserved-prefix rules.
- Ordering MUST NOT affect equality; label sets MUST be compared by key/value after merge.

## 6. Service Identity Composition

- Service identity MUST be derived from: project, stack, partition, service.
- The canonical identity MUST be reflected in managed labels consistently across apply, status, and stack deploy.
- Any logical name label MUST encode the same identity components and MUST be consistent across all flows.

## 7. Drift Detection Semantics

- Label drift is any difference between desired and current label sets after rendering, filtering, and managed-label injection.
- Differences in managed labels constitute drift.
- Differences in user-defined labels constitute drift after merge rules are applied.
- Labels outside the canonical set are ignored only if explicitly excluded by filtering rules.

## 8. Error Handling Expectations

- Reserved-prefix violations MUST error.
- Empty label keys after rendering MUST error.
- Conflicts where user-defined labels attempt to override managed labels MUST be rejected.
- These errors MUST be enforced consistently across apply, status, and stack deploy.
