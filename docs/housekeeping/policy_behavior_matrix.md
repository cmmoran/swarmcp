# Policy Behavior Matrix

## 1. Purpose and Scope

This matrix defines canonical normalization and validation behavior for restart, update, and rollback policies. It is authoritative for apply, status, and stack deploy behavior.

Relationship to docs/intent_spec_contract.md:
- This document defines normalization and validation rules for policy fields that are part of intent/spec.
- Field inclusion and drift participation are defined in docs/intent_spec_contract.md.

Out of scope:
- Configuration loading and overlay resolution.
- Template rendering mechanics beyond the requirement that inputs are already rendered strings/values.
- Runtime orchestration behavior.

## 2. Shared Assumptions

- Inputs originate from config and rendered values and are evaluated before intent/spec comparison.
- “Omitted” means the field is not present; omitted fields MUST remain omitted.
- “Explicit zero-value” means a field is present with a value that is zero-like (e.g., 0, 0s); explicit zero-values MUST be preserved.
- Validation errors MUST be surfaced consistently across apply, status, and stack deploy.
- Normalization MUST be applied consistently and only where specified below.

## 3. Restart Policy Behavior

Fields: condition, delay, max_attempts, window.

- condition
  - Accepted values: none, on-failure, any (case-insensitive).
  - Invalid values: empty string; any other value.
  - Normalization: MUST trim whitespace and lowercase.
  - Equality: compare normalized string values.

- delay
  - Accepted values: any valid duration string (e.g., 5s, 1m, 0s).
  - Invalid values: empty string; invalid duration format.
  - Normalization: MUST trim whitespace; duration parsing MUST be used for validation.
  - Equality: compare parsed duration values.

- max_attempts
  - Accepted values: integer >= 0.
  - Invalid values: integer < 0.
  - Normalization: none.
  - Equality: compare integer values.

- window
  - Accepted values: any valid duration string (e.g., 10s, 1m, 0s).
  - Invalid values: empty string; invalid duration format.
  - Normalization: MUST trim whitespace; duration parsing MUST be used for validation.
  - Equality: compare parsed duration values.

## 4. Update Policy Behavior

Fields: parallelism, delay, failure_action, monitor, max_failure_ratio, order.

- parallelism
  - Accepted values: integer >= 0.
  - Invalid values: integer < 0.
  - Normalization: none.
  - Equality: compare integer values.

- delay
  - Accepted values: any valid duration string (e.g., 5s, 1m, 0s).
  - Invalid values: empty string; invalid duration format.
  - Normalization: MUST trim whitespace; duration parsing MUST be used for validation.
  - Equality: compare parsed duration values.

- failure_action
  - Accepted values: pause, continue, rollback (case-insensitive).
  - Invalid values: empty string; any other value.
  - Normalization: MUST trim whitespace and lowercase.
  - Equality: compare normalized string values.

- monitor
  - Accepted values: any valid duration string (e.g., 30s, 2m, 0s).
  - Invalid values: empty string; invalid duration format.
  - Normalization: MUST trim whitespace; duration parsing MUST be used for validation.
  - Equality: compare parsed duration values.

- max_failure_ratio
  - Accepted values: float between 0 and 1 inclusive.
  - Invalid values: < 0 or > 1.
  - Normalization: none.
  - Equality: compare numeric values.

- order
  - Accepted values: stop-first, start-first (case-insensitive).
  - Invalid values: empty string; any other value.
  - Normalization: MUST trim whitespace and lowercase.
  - Equality: compare normalized string values.

## 5. Rollback Policy Behavior

Rollback policy uses the same fields and rules as update policy:
- parallelism, delay, failure_action, monitor, max_failure_ratio, order
- Accepted values, invalid values, normalization, and equality MUST be identical to Update Policy Behavior.
- No asymmetry is permitted between update and rollback handling.

## 6. Omission vs Explicit Values

- Omitted fields MUST remain omitted in intent/spec and MUST NOT be defaulted.
- Explicit zero-values (e.g., 0, 0s, 0.0) MUST be preserved and compared as explicit values.
- Empty strings MUST be treated as invalid for string-typed fields and MUST produce errors.

## 7. Error Handling Expectations

- Invalid enum values (condition, failure_action, order) MUST produce errors.
- Empty strings for string-typed fields MUST produce errors.
- Invalid duration strings MUST produce errors.
- Out-of-range numeric values (max_attempts < 0, parallelism < 0, max_failure_ratio outside [0,1]) MUST produce errors.
- These error conditions MUST be enforced consistently across apply, status, and stack deploy.

## 8. Relationship to Drift Detection

- Normalized values defined in this matrix MUST be the values used for equality checks.
- Drift semantics and field inclusion are defined in docs/intent_spec_contract.md and are authoritative.
