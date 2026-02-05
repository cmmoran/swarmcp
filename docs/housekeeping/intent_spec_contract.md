# Service Intent / Spec Contract

## 1. Purpose and Scope

This document defines the canonical service intent/spec contract that apply, status, and stack deploy MUST agree on.

- Intent: the normalized, canonical representation of desired service behavior derived from configuration, templates, and computed defaults.
- Spec: the concrete Swarm/Compose representation produced from intent for deployment or comparison.

This contract governs:
- Which fields are included in intent/spec.
- Which fields are user-defined vs derived.
- Equality semantics for drift detection.

This contract does NOT govern:
- Configuration loading, overlays, or source resolution.
- UI/CLI prompting behavior.
- Runtime orchestration order or operational workflows.

## 2. Canonical Intent/Spec Fields

The following fields are part of the contract and MUST be represented consistently by apply, status, and stack deploy.

### 2.1 Core Service Fields
- image (user-defined)
- command (user-defined)
- args (user-defined)
- workdir (user-defined)
- env (user-defined, key/value)
- ports (user-defined)
- mode and replicas (user-defined, with defaults)
- healthcheck (user-defined)

These fields MUST participate in drift detection.

### 2.2 Placement and Constraints
- placement.constraints (user-defined)

This field MUST participate in drift detection.

### 2.3 Labels
- Managed labels (derived)
- User labels (user-defined)

Labels MUST participate in drift detection, after filtering and rendering rules are applied.

### 2.4 Mounts
- Config mounts (derived from config/secret refs and rendered defs)
- Secret mounts (derived from config/secret refs and rendered defs)
- Volume mounts (derived from volume refs and volume defaults)

Mounts MUST participate in drift detection using the equality semantics defined in Section 4.

### 2.5 Networks
- Derived service networks

Networks MUST participate in drift detection using the equality semantics defined in Section 5.

### 2.6 Policies
- restart_policy
- update_config
- rollback_config

Policies MUST participate in drift detection using the equality semantics in Section 6.

## 3. Labels and Identity Semantics

### 3.1 Managed vs User Labels
- Managed labels are derived and MUST always be set by apply and status; stack deploy MUST preserve managed labels when rendering compose specs.
- User labels are defined in configuration and MUST be merged with managed labels.

### 3.2 Reserved Prefixes
- The prefix swarmcp.io/ is reserved.
- User labels MUST NOT use the reserved prefix; any such label MUST be rejected.

### 3.3 Filtering and Rendering
- User label keys MAY include template tokens and MUST be rendered before merge.
- Managed labels MUST NOT be overridden by user labels.
- Label sets MUST be compared after managed label injection and filtering.

### 3.4 Identity Composition
- Service identity MUST be derived from project, stack, partition, and service name.
- The logical name and managed labels MUST reflect this identity consistently across apply, status, and stack deploy.

## 4. Mount Semantics

### 4.1 Config and Secret Mounts
- Mounts are derived from rendered config/secret definitions and service references.
- The canonical mount identity is name, target, uid, gid, and mode.
- Ordering MUST NOT affect equality.

### 4.2 Volume Mounts
- Volume mounts are derived from service volume refs and project defaults.
- The canonical mount identity is type, source, target, and readonly.
- Ordering MUST NOT affect equality.

### 4.3 Inclusion Rules
- Only mounts referenced by the service intent are included.
- Implicit or inferred mounts MUST be included if enabled by rendering/inference rules.

## 5. Network Semantics

- Service networks are derived; explicit service-level networks are not part of intent.
- The canonical network identity is the resolved network name.
- Ordering MUST NOT affect equality.

## 6. Restart / Update / Rollback Semantics

### 6.1 Restart Policy
- restart_policy fields (condition, delay, max_attempts, window) are part of intent/spec.
- Omitted fields MUST remain omitted and MUST NOT be defaulted silently in intent/spec.
- Equality MUST compare each field value after normalization.

### 6.2 Update and Rollback Policies
- update_config and rollback_config fields (parallelism, delay, failure_action, monitor, max_failure_ratio, order) are part of intent/spec.
- Omitted fields MUST remain omitted and MUST NOT be defaulted silently in intent/spec.
- Equality MUST compare each field value after normalization.

### 6.3 Normalization Relationship
- Normalization behavior is defined in docs/policy_behavior_matrix.md and is authoritative.
- Apply, status, and stack deploy MUST reflect normalized values consistently.

## 7. Drift and Unmanaged Fields

### 7.1 Drift Definition
- Drift is any difference between current and desired intent for fields listed in Section 2, after normalization and filtering.

### 7.2 Ignored Fields
- Fields outside this contract are ignored for intent drift.
- Unmanaged Swarm spec fields MAY be reported separately but MUST NOT affect intent drift.

### 7.3 Apply vs Status Expectations
- Apply MUST produce specs consistent with intent.
- Status MUST compare current state to intent using the equality semantics defined here.
