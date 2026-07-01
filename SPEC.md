# SwarmCP Redux Specification (Draft)

## Table of Contents
- [Goals](#goals)
- [Concepts and Naming](#concepts-and-naming)
- [Recommended Project Model](#recommended-project-model)
- [Configuration Sources](#configuration-sources)
- [Config File Layering](#config-file-layering)
- [External Imports (Stacks and Services)](#external-imports-stacks-and-services)
- [Values Store (values#/path)](#values-store-valuespath)
- [Template Resolution and Scope](#template-resolution-and-scope)
- [Overlays (Deployment + Partition)](#overlays-deployment--partition)
- [Configs and Secrets (Swarm)](#configs-and-secrets-swarm)
- [Labeling](#labeling)
- [Service Intent (Milestone 2)](#service-intent-milestone-2)
- [Networks and Volumes](#networks-and-volumes)
- [Secrets Engines](#secrets-engines)
- [Change Detection and Apply Flow](#change-detection-and-apply-flow)
- [Service Dependencies and Update Policy](#service-dependencies-and-update-policy)
- [Rollback Policy](#rollback-policy)
- [State and Cache](#state-and-cache)
- [Execution Targeting (Deployment + Partition + Stack)](#execution-targeting-deployment--partition--stack)
- [CLI (Cobra)](#cli-cobra)
- [Resolve (Resolved Config Model)](#resolve-resolved-config-model)
- [Explain (Config Provenance)](#explain-config-provenance)
- [YAML Schema](#yaml-schema)
- [Examples](#examples)
- [Appendix A. Implementation Notes (Non-Normative)](#appendix-a-implementation-notes-non-normative)
- [Appendix B. Draft Design Areas (Non-Normative)](#appendix-b-draft-design-areas-non-normative)

## Goals
- Provision and manage Docker Swarm resources from YAML configuration and templates.
- Provide atomic, healthcheck-gated service updates.
- Detect drift and reconcile configs, secrets, services, networks, and volumes.
- Support reusable templates and external secrets engines.

## Concepts and Naming
- **Project**: top-level grouping; declares partitions and shared/partitioned stacks.
- **Deployment**: runtime target selection for deployment overlays, context selection, and node targeting. Deployment is not part of stack instance naming.
- **Partition**: environment-like duplication axis for partitioned stacks (e.g., `dev`, `qa`, `uat`).
- **Reserved partition name**: `_` (used as the shared-stack placeholder).
- **Stack modes**:
  - **shared**: one per project. Name: `<project>_<stack>`.
  - **partitioned**: one per partition. Name: `<project>_<partition>_<stack>`.
- **Reserved stack**: `core`.
- **Stack**: logical topology key under `stacks:`. `--stack` selects these logical keys; it does not add a new naming axis.

Target-axis model:
- SwarmCP has three targeting axes: deployment, partition, and stack.
- `deployment` selects an execution target. It changes overlay selection, context resolution, values scope, and node targeting, but it does not create additional stack instances in Swarm naming.
- `partition` selects stack instances only for stacks with `mode: partitioned`.
- `stack` selects logical stack definitions under `stacks:`.
- The effective runtime workset is the intersection of selected deployments, selected stacks, and selected partition instances for partitioned stacks.
- Shared stacks are partition-agnostic. A partition selector narrows partitioned stacks only; it must not create per-partition variants of shared stacks.
- Inclusion rules must preserve this model:
  - stack-level `included_in` may decide whether a logical stack participates in a runtime target
  - service-level `included_in` may only further narrow membership inside an already-included stack
  - service-level inclusion must not widen a stack beyond the stack's own inclusion scope
- Inclusion should be documented deployment-first:
  - `deployments` is the primary and most common `included_in` dimension
  - `partitions` is an advanced narrowing dimension for cases that truly need partition-specific presence
- Deployments may optionally restrict which partitions are valid for that deployment via `project.deployment_targets.<name>.partitions`.
- If a deployment target does not declare `partitions`, all `project.partitions` remain eligible for that deployment.

Project-context model:
- `ProjectContext` is a single effective target context, not a multi-target execution plan.
- A project context carries:
  - one loaded config after `--config` and `--release-config` layering
  - one effective deployment selection
  - zero or one effective partition selector
  - zero or one effective stack selector
  - one resolved Swarm context name
  - one values scope used to load values and templates
- Runtime commands may iterate many effective targets, but they do so by creating multiple single-target project contexts rather than by storing repeated selector sets inside one project context.
- Repeated selector sets belong to runtime-target orchestration, not to `ProjectContext` itself.

Design consequence:
- Deployment is a runtime axis, not an identity axis. Two deployments may target different contexts or node sets while still referring to the same logical and physical shared stack names.
- Partition is both a selection axis and, for partitioned stacks, an identity axis because it changes stack instance naming.
- Stack is a logical selection axis only.

## Recommended Project Model
SwarmCP should be documented around four artifacts with distinct intent:

- `project.yaml`: desired topology, imports, structural defaults, and project/team policy
- `values/*.yaml`: authored render input artifacts
- `release.yaml`: a release config passed with `--release-config` that selects deploy-time refs and service intent for existing project entries
- runtime flags: execution scope for commands such as `plan`, `diff`, `status`, and `apply`

Recommended workflow:
1. Author and review `project.yaml` infrequently when topology or project policy changes.
2. Author and review `values/*.yaml` when render input artifacts change.
3. For each deployable release, generate or author a small `release.yaml` that pins refs/images and optional service rollout intent, then pass it with `--release-config`.

The mental model is:

- `project.yaml` answers "what can exist?": nodes, deployments, partitions, stack existence, service existence, import URL/path, networks, volumes, configs, secrets, inclusion rules, and default release policy.
- Imported stack or service files own reusable service structure.
- `values/*.yaml` answers "what data feeds rendering?"; values are authored once and must not be duplicated inline in a release config.
- `release.yaml` answers "which exact deploy-time versions are selected?"; it identifies source refs, image digests/tags, git-backed values refs, and optional service rollout intent for existing project entries.
- Runtime flags answer "where and how broadly should this release be evaluated or applied?"

A release version identifies the full deployable render contract, not only the app code artifact. A release version may advance for code-only, values-only, or mixed changes.

### Release Configs

A release config is deploy-time intent, not a general-purpose YAML patch. It should be readable as a deployment decision and should stay small enough for review.

Release config root fields:
- `project.values[]`: selected values source refs by existing values source name. A release entry may set only `name` and `ref`; the base `project.values` declaration owns `url` and `path`.
- `stacks.<name>.source.ref`: selected stack import ref for an existing stack source.
- `stacks.<name>.services.<service>.image`: selected service image tag or digest.
- `stacks.<name>.services.<service>.replicas`: selected service replica count.
- `stacks.<name>.services.<service>.env` and `labels`: selected scalar service env/label overrides.
- `stacks.<name>.services.<service>.update_config` and `rollback_config`: selected rollout policy fields.

Example release config:
```yaml
project:
  values:
    - name: platform
      ref: values-prod-2026.05.29-2
stacks:
  participant:
    source:
      ref: v0.74.1
    services:
      participant:
        image: registry.example.com/app@sha256:7f3b...
        replicas: 3
```

Release config validation rules:
- The release config may only select refs/images and constrained service deploy intent.
- A release config may reference only stacks that exist in the base project config.
- A release config may reference only services that exist in the resolved stack after source refs and imports have been applied.
- `stacks.<name>.source.ref` may only select the ref for an existing `stacks.<name>.source`; it may not change `url`, `path`, or `overrides_path`.
- A release config may not add, remove, or modify stacks, services, or lifecycle jobs.
- A release config may not redefine concrete values or change values source paths. It may only select the ref for an existing git-backed `project.values` entry by name.
- If any selected source ref, image, values source ref, or deploy intent changes, a new externally tracked release version should be produced.

Disallowed in release configs:
- `project.nodes`
- `project.partitions`
- `project.deployment_targets`
- adding or removing stacks
- adding or removing services
- adding, removing, or modifying service lifecycle jobs
- `stacks.<name>.services.<svc>.included_in`
- broad structural network, volume, config, secret, or import rewrites
- concrete values data under `project.values` or any other release field

If a release config uses immutable refs, image digests, and immutable values source refs, it serves as a practical lock artifact. SwarmCP does not need a separate lockfile product to achieve basic reproducibility.

### Saved Plan Artifacts

The preferred Terraform-style execution flow is:

```bash
swarmcp plan --out release.plan.yaml ...
swarmcp show release.plan.yaml
swarmcp apply release.plan.yaml
```

`plan --out` generates an applyable `swarmcp.plan.v1` artifact. The saved plan is generated output, not another broad human-authored configuration layer. It captures:
- target metadata: project, deployment, optional partition/stack selectors, and Docker context
- input provenance: SHA-256 fingerprints for project config files, release overlays, values files, and local secrets files when a file-backed secrets store is used
- current-state assumptions: resources that were absent, resources selected for delete by ID, and stack services selected for deploy by ID/version
- exact Swarm reconciliation intent: configs, secrets, networks, stack deploy payloads, delete/prune intent, and skipped delete counts
- secret handling mode: `payload`, `reference`, or `mixed`
- secret source metadata for reference-mode secrets

`swarmcp show <plan-file>` validates and summarizes the saved plan without connecting to Docker. It is the review step for generated plans.

`swarmcp apply <plan-file>` consumes the saved plan as the execution artifact. It must not re-render the current workspace. It validates the plan API version, secret mode, replay source shape, target context, and recorded current-state assumptions before external secret replay or Swarm mutation. Passing `--context` to apply a saved plan to a different Docker context is rejected unless `--allow-context-override` is set.

Saved-plan assumption validation is part of exact-plan safety. If a resource that was absent at plan time now exists, a delete target disappeared or was replaced, a delete target became mounted by a service, or a stack service selected for deployment changed ID/version, `apply <plan-file>` must fail before applying the plan.

Secret plan semantics:
- Default saved plans should avoid storing secret payloads when a secret can be replayed from source metadata.
- Vault/OpenBao KV secret dependencies record provider, address, auth method/path/role/audience, mount, path, key, optional KV version, and the SHA-256 hash of the resolved value.
- When a KV version is available, `apply <plan-file>` must request that version and verify the resolved hash before creating the Swarm secret.
- File-backed secret dependencies record provider `file`, key, and the SHA-256 hash of the resolved value. The plan input list records the secrets file path and SHA-256 fingerprint; `apply <plan-file>` must require that same file to exist with the same fingerprint before reading the key and verifying the resolved value hash.
- If a created Swarm secret is composed from multiple secret values or otherwise cannot be replayed from one source, `plan --out` refuses to write the plan unless `--include-secret-payloads` is explicitly set.
- Payload-mode plans are allowed as an explicit development/operator escape hatch, not the preferred production release artifact.

### Planned Release Version Policies

Release version policy is future project/team governance. When implemented, it should live in `project.yaml` under `project.release_policies`. A concrete release config could store the selected policy name, the resolved release version, and resolved version parameters; it should not store the policy definition unless it intentionally snapshots an external policy for audit.

The authoring experience should prefer presets over raw parameter definitions. Custom templates and parameter definitions are the advanced escape hatch.

Example project policy:
```yaml
project:
  release_policies:
    default:
      version:
        preset: deployment_stack_calver_sequence
        parameters:
          sequence:
            scope: [deployment, stack, calver]
            width: 3
```

Equivalent expanded policy:
```yaml
project:
  release_policies:
    default:
      version:
        scheme: template
        template: "{{ .deployment }}-{{ .stack }}-{{ .calver }}.{{ .sequence }}"
        parameters:
          deployment:
            source: target.deployment
          stack:
            source: target.stack
          calver:
            source: clock.date
            type: calver
            format: YYYY.MM.DD
          sequence:
            type: counter
            scope: [deployment, stack, calver]
            width: 3
```

Built-in version presets should include:
- `semver`
- `semver_prerelease`
- `calver`
- `calver_sequence`
- `branch_calver_sequence`
- `deployment_stack_calver_sequence`
- `semver_calver_build`

Parameter behavior:
- Most parameters are supplied or derived; they do not auto-increment.
- Only parameters with `type: counter` auto-increment.
- Counter parameters must declare a `scope`. Valid scope elements include `global`, `branch`, `deployment`, `partition`, `stack`, `calver`, `semver`, and `artifact_tuple`.
- Counter allocation must be deterministic within its declared scope. For example, a `scope: [deployment, stack, calver]` counter restarts for each deployment, stack, and calendar version.
- The resolved release version must be persisted in the release config or adjacent release artifact. Commands must not silently recompute a different version for an existing release artifact.

Supported parameter sources:
- `input.<name>`: provided by CLI, CI, or user prompt.
- `git.branch`: current branch name.
- `git.ref`: current checked-out ref.
- `git.sha`: current commit SHA.
- `clock.date`: current date using the policy format.
- `target.deployment`, `target.partition`, `target.stack`: selected release target fields.
- `artifact.<kind>...`: selected artifact identity, such as a stack source ref, image digest, or values ref.
- `release.sequence`: allocated sequence/counter value.

Supported parameter types:
- `string`
- `git_ref`
- `semver`
- `semver_prerelease`
- `calver`
- `integer`
- `counter`

Supported parameter transforms:
- `normalize: slug` for branch/ref-like values.
- `format` for date/calver and optional string formatting.
- `prefix` or template helpers for optional prerelease/build fragments.
- `omit_if_empty: true` for optional parameters.
- `width` for zero-padded integer and counter values.

Schema and discovery requirements:
- Release policy YAML should be covered by JSON Schema so YAML language servers can provide completion, enum values, and descriptions.
- SwarmCP should expose CLI discovery for presets and expanded policy shape, such as `swarmcp release version presets`, `swarmcp release version explain <preset>`, `swarmcp release version init --preset <name>`, and `swarmcp release version preview`.

### Release Overlay Resolution Model

Release configs are constrained deployment intent, not general YAML patches. This distinction is important because SwarmCP supports imported stacks and services. Imported stacks make the configuration more powerful, but they also mean some service definitions are not visible in the project wrapper until import resolution has happened.

SwarmCP therefore applies release overlays in two conceptual phases:

1. **Import selection phase**
   - `stacks.<name>.source.ref` is applied before import resolution.
   - This selects the version of an imported stack or service repository that SwarmCP should fetch and expand.
   - The stack named by `stacks.<name>` must already exist in the base project config.
   - The stack must already have a `source` object in the base project config. A release overlay may change the ref of an import, but it may not create an import.
   - `source.url`, `source.path`, and `source.overrides_path` remain structural authorship. They are not deploy-time pins and are rejected in release overlays.

2. **Deploy intent phase**
   - Service fields such as `image`, `replicas`, `env`, labels, and update policy are validated and applied after imports are expanded.
   - This means release overlays may pin a service inside an imported stack:

     ```yaml
     stacks:
       participant:
         source:
           ref: v0.74.1
         services:
           participant:
             image: registry.example.com/app@sha256:7f3b...
     ```

   - The example above is valid when the base project defines `stacks.participant.source`, and the imported stack at `v0.74.1` contains a service named `participant`.
   - The same overlay is invalid if the imported stack does not contain `services.participant`.
   - Service pins are never allowed to create new services. They can only modify the small allowlisted deploy-intent fields of a service that exists after import expansion.

This two-phase model is deliberate. It lets release configs express the deployable tuple of "stack source ref plus service image digest plus values source ref" while preserving the safety property that release configs cannot redefine topology or duplicate values.

When in doubt, ask whether a field changes *what exists*, *what data was authored*, or *which version of an existing artifact should run*. If it changes what exists, it belongs in project or imported stack configuration. If it authors concrete render data, it belongs in values. If it chooses the version or runtime intent for an already-existing artifact, it may belong in a release config.

## Configuration Sources
- **Primary source**: project repository (YAML + templates).
- **Optional overrides**: service-level repos for config/secret sources.
- **Templates**: Go templates + Sprig. Sources ending in `.tmpl` are rendered as templates; other files are included as-is.

Sources can be scoped at project, stack, partition, or service level to define a base for config/secret `source:` paths:
```yaml
project:
  sources:
    url: ssh://git@github.com/org/stack-library.git
    ref: v1.2.3
    path: configs
```
Behavior:
- If `sources.url` is empty, `sources.path` is resolved relative to the file that defined it.
- If `sources.url` is set, the repo is fetched and `sources.path` is resolved within the checked-out tree.
- A `sources` definition only affects config/secret sources defined at the same scope.
- Path escape is forbidden: resolved paths must remain inside the source tree that defined them (symlink-aware), otherwise it is an error.
- Absolute `sources.path` values are allowed and treated as their own source roots (symlink-aware), similar to a checked-out repo root.

Local overrides continue to work by redefining `sources` at a narrower scope.

## Config File Layering
`--config` is repeatable. Repeatable `--config` is the implementation mechanism for composing structural project configuration. It is not the primary mental model for releases and it is not intended as a general patch system.

`--release-config` is also repeatable. It is the recommended way to apply deploy-time release intent, because SwarmCP validates those files against the release-config constraints above.

Canonical example:
```bash
swarmcp plan \
  --config project.yaml \
  --values values/prod.yaml \
  --release-config releases/prod-2026-03-10.yaml
```

```bash
swarmcp apply \
  --config project.yaml \
  --values values/prod.yaml \
  --release-config releases/prod-2026-03-10.yaml
```

Example release config:
```yaml
stacks:
  core:
    source:
      ref: 2026.03.10-1842
    services:
      api:
        image: ghcr.io/acme/api@sha256:7f3b...
        replicas: 3
```

Layering rules:
- Merge order is left to right.
- The first `--config` is the base config.
- Each later `--config` overlays the accumulated result.
- The final merged document is the input to import resolution, validation, render, plan, diff, status, and apply.
- If `.swarmcp.project` is used, it behaves as the implicit first config when no explicit `--config` flags are passed.

Path and identity rules:
- The first config file is the base identity for relative path resolution.
- The first config file is also the base identity for inferred values/secrets lookup and local state/cache naming.
- Later config files are partial overlays only; they do not establish independent path roots.

Field policy:
- Named maps merge by key.
- Scalar fields replace earlier values.
- Lists replace earlier values; they do not append.
- Singular objects such as `source`, `restart_policy`, `update_config`, `rollback_config`, `healthcheck`, and `secrets_engine` replace as whole objects and are never field-merged.
- Final merged configs must still satisfy the same validation rules as single-file configs.

Repeated `--config` remains available for constrained config layering in general. The recommended project model for deploy-time pins is `project.yaml` plus authored `values/*.yaml` plus a deploy-specific release config passed via `--release-config`.

Release overlay intent rules:
- Release configs may change version-selection fields and a small set of deploy-intent fields.
- Release configs must not change project topology.
- Release configs should stay small enough to review as deployment intent rather than as structural authoring.
- When stronger reproducibility is required, release configs should use immutable refs, image digests, and immutable values source refs.
- Release validation applies only when a file is passed via `--release-config`; plain repeated `--config` layering retains the broader config-layering rules below.
- Release configs are not ordinary repeated config overlays. They are interpreted through the two-phase release model:
  - `source.ref` is applied before imports so it can select the imported stack or service version.
  - service deploy-intent fields are validated and applied after imports so services inside imported stacks can be pinned safely.
- For imported stacks, service pins are checked against the imported stack at the selected ref, not against the pre-import wrapper in `project.yaml`.
- For non-imported stacks, service pins are checked against the services directly authored in the base project config.
- In both cases, the service must exist before the release service pin is applied.
- Legacy release overlays support these allowed fields:
  - `stacks.<name>.source.ref`
  - `stacks.<name>.services.<svc>.image`
  - `stacks.<name>.services.<svc>.replicas`
  - `stacks.<name>.services.<svc>.env.<key>`
  - `stacks.<name>.services.<svc>.labels.<key>`
  - `stacks.<name>.services.<svc>.update_config.{parallelism,delay,failure_action,monitor,max_failure_ratio,order}`
  - `stacks.<name>.services.<svc>.rollback_config.{parallelism,delay,failure_action,monitor,max_failure_ratio,order}`

Release review guidance:
- A release config should be readable as a deployment decision, not as a second project file.
- A reviewer should be able to answer:
  - which release version is being deployed,
  - which imported stack or service ref is selected,
  - which image digest or tag will run,
  - which named values artifact revision will feed rendering,
  - whether replicas or operational rollout knobs changed,
  - and which environment or partition will consume the release file.
- A reviewer should not need to inspect a release config for new networks, volumes, configs, secrets, import paths, concrete values, or inclusion rules; those fields are rejected by design.
- Prefer immutable image digests (`repo@sha256:...`) over mutable tags when the release file is intended to document an exact deployable artifact.
- Prefer immutable source refs, such as tags or commit SHAs, when the imported stack shape must remain reproducible.
- Prefer immutable values source refs, such as tags or commit SHAs, when rendered configs must remain reproducible.

Per-section policy:
- Root:
  - `project`: merge by the field rules below.
  - `stacks`: merge by stack name.
  - `overlays`: merge by deployment/partition/stack/service key using normal schema rules.
- `project`:
  - Merge: `contexts`, `deployment_targets`, `nodes`, `defaults`, `configs`, `secrets`, `sources`
  - Replace-only: `name`, `deployment`, `restart_policy`, `update_config`, `rollback_config`, `secrets_engine`, `preserve_unused_resources`, `partitions`, `deployments`
- `project.defaults`:
  - Merge: `networks`, `volumes`, `volumes.standards`
  - Replace-only: `networks.shared`, `networks.internal`, `networks.egress`, `networks.attachable`, `volumes.driver`, `volumes.base_path`, `volumes.layout`, `volumes.node_label_key`, `volumes.service_standard`, `volumes.service_target`
- `project.nodes.<name>`:
  - Merge: `labels`, `platform`
  - Replace-only: `roles`, `volumes`
- `project.deployment_targets.<name>`:
  - Merge: `include.labels`, `exclude.labels`, `overrides`
  - Replace-only: `partitions`, `include.names`, `exclude.names`
- `project.deployment_targets.<name>.overrides.<node>`:
  - Merge: `labels`, `platform`
  - Replace-only: `roles`, `volumes`
- `stacks.<name>`:
  - Merge: `partitions`, `sources`, `configs`, `secrets`, `volumes`, `services`, `overlays`
  - Replace-only: `source`, `mode`, `restart_policy`, `update_config`, `rollback_config`
  - Invalid in later config files: `overrides`
- `stacks.<name>.partitions.<name>`:
  - Merge: `sources`, `configs`, `secrets`
  - Replace-only: `restart_policy`, `update_config`, `rollback_config`
- `stacks.<name>.services.<name>`:
  - Merge: `env`, `labels`, `placement`, `sources`, `overlays`
  - Replace-only: `source`, `image`, `command`, `args`, `workdir`, `ports`, `mode`, `replicas`, `restart_policy`, `update_config`, `rollback_config`, `healthcheck`, `depends_on`, `jobs`, `egress`, `networks`, `network_ephemeral`, `configs`, `secrets`, `volumes`, `included_in`
  - Invalid in later config files: `overrides`

Import constraints under layering:
- `source` replaces as a whole object; `url`, `ref`, `path`, and `overrides_path` are never field-merged.
- A later config file may not set stack/service import `overrides`.
- Repeated `--config` layering does not permit new mixed local/import authoring states. The final merged stack/service must still obey the normal rule: sourced objects may only be modified through their own `overrides` in the defining file.
- `project.partitions` and `project.deployments` are allowed in overlays but are advanced, high-risk replace-only fields.
- The recommended release-config model is narrower than the full technical layering capability. Even where a field is technically mergeable, it should not be treated as release intent unless explicitly allowed by the release-config rules above.

Valid overlay example:
```yaml
# base: project.yaml
project:
  name: platform
stacks:
  core:
    services:
      api:
        image: ghcr.io/acme/api:main
        replicas: 2
        env:
          LOG_LEVEL: info
```

```yaml
# overlay: releases/qa.yaml
stacks:
  core:
    services:
      api:
        image: ghcr.io/acme/api:1.8.4
        replicas: 3
        env:
          LOG_LEVEL: debug
          FEATURE_FLAG_X: "true"
```

Result:
- `image` is replaced.
- `replicas` is replaced.
- `env` merges by key.

Replace-only list example:
```yaml
# base
stacks:
  core:
    services:
      api:
        networks: [frontend]
```

```yaml
# overlay
stacks:
  core:
    services:
      api:
        networks: [frontend, metrics]
```

Result:
- `networks` becomes `[frontend, metrics]`.
- The overlay does not append; it replaces the full list.

Rejected mixed-authoring example:
```yaml
# base
stacks:
  core:
    source:
      url: ssh://git@github.com/org/stack-library.git
      ref: main
      path: stacks/core.yaml
```

```yaml
# overlay
stacks:
  core:
    services:
      api:
        image: ghcr.io/acme/api:1.8.4
```

Result:
- Invalid. Repeated `--config` layering must not create a sourced stack with local fields outside that stack's own defining `overrides`.

Rejected import-override layering example:
```yaml
# overlay
stacks:
  core:
    overrides:
      services:
        api:
          image: ghcr.io/acme/api:1.8.4
```

Result:
- Invalid. Later config files may not set stack/service import `overrides`.

Acceptance criteria:
- Single-file `--config` behavior remains unchanged.
- Repeated `--config` merges left to right with later files overriding earlier files according to the field policy above.
- The first config remains the root for relative path resolution, inferred values/secrets discovery, and local state/cache identity.
- Invalid later-file `overrides` usage on sourced stacks/services is rejected with a validation error.
- Invalid mixed local/import authoring states produced by layering are rejected with a validation error.

Error cases:
- No explicit `--config` and no `.swarmcp.project`: fail with a clear config-required error.
- Any provided config file is unreadable: fail with the path and underlying read error.
- Any provided config file root is not a YAML mapping: fail with the path and a root-must-be-mapping error.
- A later config file sets stack/service import `overrides`: fail with a validation error naming the offending path.
- Layering produces an invalid sourced/local hybrid: fail with the existing sourced-vs-local validation error naming the offending stack/service.

## External Imports (Stacks and Services)
Stacks and services can be imported from external files (local or git) and overridden locally.

Import shape:
```yaml
stacks:
  core:
    source:
      url: ssh://git@github.com/org/stack-library.git
      ref: v1.2.3
      path: stacks/core.yaml
    overrides:
      services:
        ingress:
          replicas: 2
```

Service import shape:
```yaml
stacks:
  api:
    services:
      worker:
        source:
          url: ssh://git@github.com/org/stack-library.git
          ref: main
          path: services/worker.yaml
        overrides:
          env:
            LOG_LEVEL: info
```

Import rules:
- When `source` is set on a stack or service, the referenced file provides the base definition.
- Local changes must be specified under `overrides`. Other local fields at that level are rejected.
- Merge order: local overrides -> remote overrides (if configured) -> remote base.
- Maps deep-merge; lists replace by default. List merge controls use the same suffixes as values (`field+`, `field~`).
- `source.path` is resolved relative to the imported file when it references additional `source:` files inside it.
- Path escape is forbidden for imports and nested sources: resolved paths must stay within the imported tree (symlink-aware), otherwise it is an error.
- Absolute `source.path` values are allowed and treated as their own source roots (symlink-aware), similar to a checked-out repo root.
- Imported stacks/services use their own base directories for resolving nested `sources`, even if parent scopes define `sources`.

Remote overrides are configured with `source.overrides_path` (optional); when set, the overrides file is loaded from the same repo/local path as `source.path`.

`source` uses the same shape as `sources` with one extra field:
- `url` (optional): git repo URL. When omitted, `path` is local.
- `ref` (optional): tag, branch, or commit SHA.
- `path` (required): path to the stack/service YAML inside the repo or local filesystem.
- `overrides_path` (optional): path to a stack/service override file in the same repo or local filesystem.

Fetch and cache (draft):
- On-demand fetch into a local cache directory; re-fetch when not offline.
- `--offline` disables fetch and uses only cached copies.
- Resolved commit SHAs are recorded in cache metadata for reproducibility.


## Values Store (`values#/path`)
Values files can be loaded at runtime and merged into a single in-memory document.
Configs/secrets can then point to fragments inside that document using `values#/...`.

Usage:
- Pass one or more `--values <path>` flags (order matters).
- Each values file may be a template (`.tmpl`); templates render before merge.
- Merge behavior is replace-on-conflict (map keys override; arrays replace).

Example:
```yaml
project:
  configs:
    domain:
      source: values#/global/domain
```

Source forms:
- File path (optionally `.tmpl`)
- Values fragment (`values#/...`)
- Inline template (`inline:{{ ... }}` or a block with `inline:` prefix; content is trimmed)

Implicit values lookup (when the path does not start with `#/global`, `#/deployments`, `#/partitions`, or `#/stacks`):
1) `#/stacks/<stack>/partitions/<partition>/<path>` (if stack + partition)
2) `#/stacks/<stack>/<path>` (if stack)
3) `#/partitions/<partition>/<path>` (if partition)
4) `#/deployments/<deployment>/<path>` (if deployment)
5) `#/global/<path>`
6) `#/<path>`

## Template Resolution and Scope
Resolution is hierarchical and stops at the first match:
1) service level
2) partition level
3) stack level
4) project level

Overlays apply after base definitions. Precedence is narrow-to-broad (service -> partition -> stack -> project).
Sealed overlays are applied last within their scope and override non-sealed values. If multiple sealed values
conflict, the broadest scope wins (project > stack > partition > service).

Service-level definitions live under `services.<service>.configs` and `services.<service>.secrets` when the entry includes `source`.

Configs defined at higher levels are available to lower levels. Cross-scope access to configs is allowed.
Secrets defined at higher levels are available to lower levels. Cross-scope access to secrets is denied by default.
External definitions are supported via higher-scope configs/secrets and referenced by name.

Resolution order is level-first, then type: service -> partition -> stack -> project, with configs resolved before secrets.

Inference (default on):
- Service template `config_ref` / `secret_ref` and `config_refs` / `secret_refs` calls infer required mounts and definitions when no explicit `configs` / `secrets` are listed.
- Inferred configs default to `source: values#/name` (lookup uses normal values scoping).
- Inferred secrets default to `source: inline: {{ secret_value "name" }}`.
- `config_value` falls back to `values#/name` when no config definition exists (uses normal values scoping when a values store is configured). When the values entry is a map or list, the fallback returns the structured value (so `config_value_index` and `config_value_get` work).
- Literal names/patterns are inferable. Dynamic name/pattern expressions are not inferable and are treated as dynamic references.
- Disable with `--no-infer` to require explicit `configs` / `secrets` declarations.

Template references:
- Config templates may reference other config templates.
- Secret templates may reference other secret templates and/or config templates.
- Secret templates may resolve either:
  - the secret value (fetched from a secrets engine), or
  - the secret reference path (the file path where Swarm mounts the secret).
- Config templates may resolve config values/refs and secret refs, but not secret values.

Template functions (draft):
- `secret_value "<name>"`
- `secret_ref "<name>"`
- `config_value "<name>"`
- `config_value_index "<name>" <index>`
- `config_value_get "<name>" "<key>"`
- `config_path "<path>"`
- `config_ref "<name>"`
- `config_refs "<glob-pattern>"`
- `secret_refs "<glob-pattern>"`
- `runtime_value "<template>"`
- `runtime_value "standard_volumes" ["standard=<name>[,<name>...]"] ["category=<name>[,<name>...]"] ["_format=csv|json|yaml"]`
- `external_ip`
- `escape_template "<expression-or-text>" ["<levels-or-open-delim>"] ["<open-delim>"] ["<close-delim>"]`
- `escape_swarm_template "<expression-or-text>" ["<levels-or-open-delim>"] ["<open-delim>"] ["<close-delim>"]`
- `swarm_network_cidrs [name]`

`config_value` and `config_ref` resolve names using the same scope hierarchy as config references. When inference is enabled, `config_value` falls back to `values#/name` if no config definition exists.
`config_refs` and `secret_refs` match logical names using glob patterns (`*`, `?`, `[class]`) against names visible in the current scope hierarchy (service -> partition -> stack -> project), then resolve each match using the same semantics as `config_ref` / `secret_ref`.
`config_refs` and `secret_refs` return deterministic sorted lists of resolved mount paths and return an empty list when there are no matches.
Invalid glob patterns are errors.
`config_value_index` and `config_value_get` help extract list and map values from config templates.
`config_path` resolves a top-level config/value name, then traverses nested maps/lists using explicit path syntax. Dot-separated segments and bracket segments are both supported and may be mixed. Numeric segments index into lists. For example, `config_path "runtime.urls.agency"`, `config_path "runtime[urls][agency]"`, and `config_path "runtime.urls[agency]"` all resolve `runtime -> urls -> agency`, while `config_path "runtime.trusted_issuers.0"`, `config_path "runtime[trusted_issuers][0]"`, and `config_path "runtime.trusted_issuers[0]"` all return the first list element.
`runtime_value` expands `{project}`, `{deployment}`, `{stack}`, `{partition}`, `{service}`, `{networks_shared}`, and `{network_ephemeral}` tokens in the provided string.
`runtime_value "standard_volumes"` returns service standard volume mounts (host:target) filtered by standard and/or category.
`_format` defaults to `csv` and outputs a comma-separated list of `host:target` pairs.
`{networks_shared}` expands to a comma-separated list of rendered shared networks for the current partition scope.
`{network_ephemeral}` expands to the service-scoped ephemeral network name when configured; otherwise it is empty.
`external_ip` fetches the current WAN IP from `https://ifconfig.me/ip` at render time (dev/operator use).
`escape_template` emits Go template expressions as literal strings for downstream renderers. If the input contains one or more `{{ ... }}` actions, each action is escaped independently and the surrounding text is preserved. If the input does not contain `{{ ... }}`, it is treated as a Go template action expression and wrapped before escaping (so `escape_template "foo"` yields a literal `{{ foo }}` downstream).
`levels` controls how many template passes the output survives: `levels=1` returns a literal Go template action that will execute on the next pass, while higher levels add additional escaping layers so the action survives multiple passes.
Example pass counts:
- `escape_template "{{ .Service.Name }}.{{ .Task.Slot }}" "1"` produces `{{ .Service.Name }}.{{ .Task.Slot }}` after 1 pass.
- `escape_template "{{ .Service.Name }}.{{ .Task.Slot }}" "2"` produces `{{ "{{ .Service.Name }}" }}.{{ "{{ .Task.Slot }}" }}` after 1 pass, then `{{ .Service.Name }}.{{ .Task.Slot }}` after 2 passes.
`escape_template` arguments: the first parameter is the input text/expression; the optional second parameter is either an integer `levels` or the open delimiter. When a `levels` is provided, the open/close delimiters shift to parameters 3 and 4; otherwise they are parameters 2 and 3. Delimiters default to `{{` and `}}`. The delimiter parameters define the Go template delimiters that will be used by downstream engines; those engines must be configured to the same delimiters for the escaped output to render correctly.
`escape_swarm_template` is an alias of `escape_template` and shares the same semantics.

YAML quoting guidance:
- Use backticks for the input expression when possible to avoid YAML escaping (`value: "{{ escape_template \`default (uuidv4) (.Header.Get \"X-Correlation-ID\")\` }}"`).
- If backticks are not viable, you can build the expression using Sprig helpers like `quote`:
  - Example: `value: '{{ escape_template (printf "default (uuidv4) (.Header.Get %s)" (quote "X-Correlation-ID")) }}'`
`swarm_network_cidrs` returns the CIDR subnets for a named Swarm network or all Swarm-scoped overlay networks when no name is provided.

Service labels are rendered as templates using the service scope.
Service definition string fields (env values, image, command/args, placement constraints, network names, volume targets, config/secret mount fields, restart/update policy string fields, etc.) first expand scope tokens such as `{project}`, `{deployment}`, `{stack}`, `{partition}`, and `{service}`, then render as templates using the service scope.
Environment map keys expand scope tokens but are not themselves rendered as templates; environment values follow the normal service string-field rendering rules.
Bare `{{ ... }}` scalars in `project.yaml` are accepted for compatibility. The loader normalizes them before/while parsing so unquoted template expressions continue to work, including a narrow fallback path for cases that do not parse as standard YAML scalars initially.

Tokens:
- `{partition}` is omitted for shared stacks unless noted otherwise.

Config/secret mount defaults and overrides:
- Config/secret definitions may set default mount fields: `target`, `uid`, `gid`, `mode`.
- Service references may override those defaults per use.
- Precedence: service reference override > definition defaults > engine defaults.
- Engine defaults: secrets mount to `/run/secrets/<name>`; configs mount to `/<name>`.

## Overlays (Deployment + Partition)
Overlays let you override config/secret definitions and selected service fields without changing the base structure.

Types:
- **Deployment overlay**: selected by `project.deployment`.
- **Partition overlay**: selected by `match.partition` against the active partition.
  - Partition overlays may be defined as a **map** keyed by partition name (exact match), or a **list** of rules
    with `match.partition` (preferred; supports exact, glob, or regexp).

Overlay precedence (default):
- base definitions are applied first, then overlays.
- unsealed overlay order is project deployment, project partition (rule order), stack deployment, stack partition (rule order), service deployment, service partition (rule order).
- sealed overlays apply last within their scope; deployment and partition order are preserved.
- if multiple sealed values conflict, the broadest scope wins (project > stack > service).
- `project.secrets_engine` is a singular replace-only object when supplied via overlays:
  - base `project.secrets_engine` applies first
  - `overlays.deployments.<deployment>.project.secrets_engine` replaces the whole object when present
  - matching `overlays.partitions.*.project.secrets_engine` replaces the whole object when present
  - partition overrides deployment when both are present
  - no field-level merge occurs across `secrets_engine` objects

Overlay scope (draft):
- Project, stack, and stack partition config/secret definitions.
- Project `secrets_engine`, which may be overridden by deployment and partition overlays as a whole object.
- Stack service overrides via overlays (`overlays.*.stacks.<stack>.services.<service>`); overlay services merge into the base service definition and do not support `source` or `overrides`.
- Stack-level overlay definitions are supported at `stacks.<stack>.overlays.<name>` with the same rules as `overlays.*.stacks.<stack>`.
- Stack/service templates may also define `overlays` (`stacks.<stack>.overlays` inside stack templates, `services.<service>.overlays` inside service templates).
- Service template overlays are service-scoped maps (no `services:` nesting); service partition overlays may be a map (exact match) or a list with `match.partition`.
- `sealed: true` may be set on project/stack/service overlay blocks; sealed fields override non-sealed values even if they are narrower in scope.

`included_in` placement rule:
- `included_in` is allowed on stacks and services anywhere stack/service definitions or stack/service overlays already accept stack/service fields, except release overlays.
- Allowed authoring locations include:
  - base stack definitions in `project.yaml` or any other normal config file at `stacks.<stack>.included_in`
  - base service definitions in `project.yaml` or any other normal config file at `stacks.<stack>.services.<service>.included_in`
  - normal repeated `--config` layered overlays for stacks at the same stack path above
  - normal repeated `--config` layered overlays at the same path above
  - project deployment/partition overlays at `overlays.*.stacks.<stack>.included_in`
  - project deployment/partition overlays at `overlays.*.stacks.<stack>.services.<service>.included_in`
  - stack deployment/partition overlays at `stacks.<stack>.overlays.*.included_in`
  - stack deployment/partition overlays at `stacks.<stack>.overlays.*.services.<service>.included_in`
  - service-scoped deployment/partition overlays at `stacks.<stack>.services.<service>.overlays.*.included_in`
- `included_in` is not allowed in files passed via `--release-config`.
- This distinction is normative: `included_in` changes runtime stack/service membership and topology, so it belongs to project/config overlays, not release overlays.

Overlay use case:
- "Values as configs": define base value configs, then override `source` per overlay.
  Templates can call `config_value` to resolve the active value.

Example (draft):
```yaml
stacks:
  core:
    configs:
      traefik:
        source: templates/configs/traefik.yml.tmpl
        target: /etc/traefik/traefik.yml
        uid: "0"
        gid: "0"
        mode: "0444"
    services:
      ingress:
        image: ghcr.io/cmmoran/traefik:3.6b
        configs:
          - name: traefik
            target: /etc/traefik/traefik.yml
            mode: "0440"
```

Partition overlay rule example:
```yaml
overlays:
  partitions:
    - name: prod-defaults
      match:
        partition: prod
      stacks:
        core:
          services:
            api:
              image: my-api:prod
    - name: canary-any
      match:
        partition: "canary-*"
      stacks:
        edge:
          services:
            web:
              replicas: 1
    - name: regexp-partitions
      match:
        partition:
          type: regexp
          pattern: "^prod-\\d+$"
      stacks:
        core:
          services:
            api:
              replicas: 2
```

Service template overlay example:
```yaml
services:
  api:
    image: ghcr.io/example/api:latest
    overlays:
      deployments:
        prod:
          replicas: 3
          labels:
            env: prod
      partitions:
        - name: dev
          match:
            partition: dev
          replicas: 1
        - name: canary
          match:
            partition: "canary-*"
          sealed: true
          env:
            FEATURE_FLAG: "true"
```

Partition contract inference (draft):
- Stack imports assume project partition names are canonical.
- The tool may infer required partition-scoped values/secrets/refs by tracing template references and scope usage.
- `runtime_value` calls that include `{partition}` mark a stack as partition-sensitive but do not imply required
  values/secrets paths.
- Dynamic/unresolvable partition references are errors by default.
- If a project overlay provides an explicit value for the same field path, the error is downgraded to a warning
  (templated values are allowed).

## Configs and Secrets (Swarm)
- Swarm configs/secrets are immutable.
- YAML names are **logical**; the tool creates **versioned physical names**.
- Physical names are short + hashed; metadata is stored in labels.
- Service updates replace config/secret references with new IDs.

Physical naming rule:
- `<logical>_<hash12>` using `sha256` (12 chars), truncated to 63 chars if needed.
- Project/partition/stack context is stored in labels, not the name.
- Logical names must be short enough to avoid truncation collisions (max 50 chars).

## Labeling
Each config/secret/service carries labels for:
- `swarmcp.io/managed=true`
- `swarmcp.io/name=<logical_name>`
- `swarmcp.io/hash=<content_hash>`
- `swarmcp.io/ref=<ref>` (required for repo-backed sources; optional for local runs)
- `swarmcp.io/project=<project>`
- `swarmcp.io/partition=<partition|none>`
- `swarmcp.io/stack=<stack>`
- `swarmcp.io/service=<service|none>`

Optional labels:
- `swarmcp.io/version=<tool_version>`
- `swarmcp.io/url=<url>`
- `swarmcp.io/path=<path>`

The `swarmcp.io/hash` label is the source of truth for config/secret content comparison; raw data is not inspected for diff/status.

## Service Intent (Milestone 2)
Services declare intent, not the full Swarm service spec.

Stacks may also declare runtime inclusion intent with `stacks.<stack>.included_in`. Stack inclusion is evaluated before any service-level inclusion.

Required:
- `image`

Optional:
- `command`, `args`, `workdir`
- `env` (key/value map)
- `ports` (`target`, `published`, `protocol`, `mode`)
- `mode` (`replicated` or `global`) and `replicas`
- `healthcheck`, `depends_on`, `egress`
- `jobs` (`before_update`, `after_update`, `on_rollback` lifecycle commands)
- `included_in` (runtime inclusion rules by deployment/partition/stack)
- `restart_policy` (`condition`, `delay`, `max_attempts`, `window`)
- `update_config` (`parallelism`, `delay`, `failure_action`, `monitor`, `max_failure_ratio`, `order`)
- `rollback_config` (`parallelism`, `delay`, `failure_action`, `monitor`, `max_failure_ratio`, `order`)
- `labels` (merged with managed labels; `swarmcp.io/*` reserved)
- `placement.constraints` (Swarm placement constraint expressions)
- `configs`, `secrets`, `sources`

Service inclusion rules:
- `included_in` is a service-level runtime selector matrix. It controls whether a service exists in the effective runtime model for a given `(deployment, partition, stack)` target.
- The default mental model is deployment membership: "this service exists in these deployments."
- In most projects, `deployments` should be the only `included_in` dimension used.
- `partitions` is optional and should be treated as an advanced narrowing feature, mainly for partitioned stacks that truly need partition-specific service presence.
- Service inclusion is always evaluated after stack inclusion. The effective rule is:
  - stack is included for the target, and
  - service is included for the target.
- A service-level rule may further narrow the stack's scope, but it must not widen it. If the stack is excluded, every service in that stack is excluded regardless of service-level `included_in`.
- `included_in` may be authored in base config at `stacks.<stack>.services.<service>.included_in`.
- `included_in` may also be authored in normal repeated `--config` overlays, using the same path above. In config layering it is replace-only, not deep-merged.
- `included_in` may also be authored in service override overlays:
  - `overlays.deployments.<deployment>.stacks.<stack>.services.<service>.included_in`
  - `overlays.partitions[*].stacks.<stack>.services.<service>.included_in`
  - `stacks.<stack>.overlays.deployments.<deployment>.services.<service>.included_in`
  - `stacks.<stack>.overlays.partitions[*].services.<service>.included_in`
  - `stacks.<stack>.services.<service>.overlays.deployments.<deployment>.included_in`
  - `stacks.<stack>.services.<service>.overlays.partitions[*].included_in`
- `included_in` must not be allowed in files passed via `--release-config`. Release overlays may not change service existence/topology.
- If `included_in` is omitted, the service is included everywhere.
- If `included_in` is present, the service is included only when at least one rule matches the active runtime target.
- Within one rule, all specified dimensions must match.
- Across rules, matching is OR.
- An omitted dimension inside a rule is a wildcard for that rule.
- On services inside `mode: shared` stacks, `deployments` remains the preferred dimension. Using `partitions` on shared-stack services is not part of the primary design goal and should generally be avoided unless future behavior is specified more explicitly.

Example:
```yaml
stacks:
  participant:
    services:
      api:
        image: ghcr.io/acme/api:latest
        included_in:
          - deployments: [dev]
          - deployments: [prod]
            partitions: [blue, green]
          - deployments: [ops]
            stacks: [participant]
```

Stack inclusion rules:
- `included_in` is also valid on `stacks.<stack>`. It controls whether that logical stack participates in the effective runtime model for a given `(deployment, partition, stack)` target.
- The default mental model is deployment membership: "this stack exists in these deployments."
- If stack-level `included_in` is omitted, the stack is included everywhere.
- If stack-level `included_in` is present, the stack is included only when at least one rule matches the active runtime target.
- Stack-level `included_in` uses the same rule shape as service-level `included_in`.
- For `mode: partitioned` stacks, `partitions` may be used when you truly need to narrow which partition instances of the stack exist for a given deployment.
- For `mode: shared` stacks, `deployments` is the intended dimension. Using `partitions` on shared-stack inclusion rules is outside the primary design goal and should generally be avoided unless future behavior is specified more explicitly.

Example:
```yaml
stacks:
  primary-ext:
    mode: shared
    included_in:
      - deployments: [prod, nonprod]
    services:
      grafana:
        image: grafana/grafana:latest
        included_in:
          - deployments: [prod, nonprod]
      loki:
        image: grafana/loki:latest
        included_in:
          - deployments: [prod, nonprod]
      drone:
        image: drone/drone:latest
        included_in:
          - deployments: [nonprod]
```

Service lifecycle jobs:
- `stacks.<stack>.services.<service>.jobs` defines ordered one-shot commands for that service.
- Jobs are not steady-state Swarm services and must not be included in normal `docker stack deploy` payloads.
- A job always runs the referencing service's rendered image. It cannot set its own image or source.
- A job must set `command`; `args` are optional.
- A job may override only execution parameters: `name`, `command`, `args`, `workdir`, `env`, `timeout`, and `cleanup`.
- Job `env` is merged over the referencing service's rendered environment for that job run only.
- Service configs, secrets, volumes, placement, networks, and environment are inherited from the referencing service's rendered intent unless a later spec explicitly adds narrow overrides.
- `before_update`: jobs that must complete before the service is updated or created.
- `after_update`: jobs that run after the service reaches the successful update criteria.
- `on_rollback`: jobs that run if the service update fails and rollback handling is selected.
- Job arrays are ordered and service-local. Cross-service, cross-stack, and cross-partition job references are out of scope.
- If a service is excluded by `included_in`, its lifecycle jobs are also excluded for that target.
- Jobs must complete successfully before the dependent execution step proceeds. Completion means the job task exits with code 0 unless a future job success policy states otherwise.
- Failed `before_update` jobs fail the plan application before the steady-state service update proceeds.
- Failed `after_update` jobs fail the plan application after the service update and may trigger rollback handling if policy selects rollback.
- Rollback jobs are only considered after a deployment failure path reaches rollback handling.
- Job cleanup policy is explicit with `cleanup: always`, `success`, or `never`; default is `success`.
- Job timeout is explicit with `timeout`; default is project/tool policy and should be short enough to fail operator workflows predictably.
- Jobs are deployment topology, not release intent, so they are allowed in normal project/config files and overlays, but they may not be added, removed, or modified in files passed with `--release-config`.

Example:
```yaml
stacks:
  participant:
    services:
      participant:
        image: ghcr.io/acme/participant:v2
        secrets:
          - name: db_config
        jobs:
          before_update:
            - name: migrate
              command: ["./participant", "migrate", "up"]
              timeout: 5m
              cleanup: success
          on_rollback:
            - name: rollback-db
              command: ["./participant", "migrate", "down"]
              timeout: 5m
              cleanup: always
```

Saved plan behavior for jobs:
- `plan --out` must serialize job execution steps as part of the exact apply intent.
- The plan must include each job's service image, command, args, workdir, env, order, phase, target scope, cleanup policy, timeout, inherited service runtime attachments, and any replay metadata needed for inherited secrets.
- `apply <plan-file>` must execute saved job steps from the plan and must not re-render job definitions from the current workspace.
- `swarmcp show <plan-file>` should summarize job steps and their phases without printing secret payloads.
- A plan that contains lifecycle jobs but cannot serialize the needed job execution spec is invalid.

Restart policy inheritance:
- `project.restart_policy` provides the default.
- `stacks.<stack>.restart_policy` overrides project.
- `stacks.<stack>.partitions.<partition>.restart_policy` overrides stack.
- `stacks.<stack>.services.<service>.restart_policy` overrides partition.

Restart policy fields:
- `condition`: `none`, `on-failure`, or `any`.
- `delay`: duration string (e.g. `5s`, `1m`).
- `max_attempts`: non-negative integer.
- `window`: duration string (e.g. `10s`, `1m`).

Example:
```yaml
project:
  restart_policy:
    condition: on-failure
    delay: 5s
    max_attempts: 1
    window: 2m
stacks:
  api:
    restart_policy:
      delay: 10s
    partitions:
      dev:
        restart_policy:
          max_attempts: 3
    services:
      web:
        restart_policy:
          condition: any
```

Update/rollback config inheritance:
- `project.update_config` / `project.rollback_config` provide defaults.
- `stacks.<stack>.update_config` / `stacks.<stack>.rollback_config` override project.
- `stacks.<stack>.partitions.<partition>.update_config` / `stacks.<stack>.partitions.<partition>.rollback_config` override stack.
- `stacks.<stack>.services.<service>.update_config` / `stacks.<stack>.services.<service>.rollback_config` override partition.

Update/rollback config fields:
- `parallelism`: non-negative integer (0 = unlimited parallelism).
- `delay`: duration string (e.g. `5s`, `1m`).
- `failure_action`: `pause`, `continue`, or `rollback`.
- `monitor`: duration string (e.g. `30s`, `2m`).
- `max_failure_ratio`: float between 0 and 1.
- `order`: `stop-first` or `start-first`.

Example:
```yaml
project:
  update_config:
    parallelism: 2
    delay: 10s
    failure_action: rollback
    monitor: 30s
    max_failure_ratio: 0.1
    order: start-first
```

Derived (not configurable at service scope yet):
- `networks`: computed from stack mode + partition + `egress`
- `labels`: managed labels are always set; user labels are deferred
- `resources`: deferred until a profile or policy mechanism exists

## Networks and Volumes
- Partition isolation is strict by default.
- Internal networks are partition-scoped:
  - `<project>_<partition>_internal`
  - shared stacks attach to all partition internal networks by default
  - partitioned services reach shared stacks via the partition internal network
- Egress pattern:
  - default external egress network: `<project>_egress` (`internal:false`)
  - services opt in with `egress: true`; they do not reference the network name directly
- Service-level networks are derived; `network_ephemeral` adds an attachable service-scoped network managed by the stack.
  - Name: `<project>_<stack>_svc_<service>` for shared stacks, `<project>_<partition>_<stack>_svc_<service>` for partitioned stacks.
- Network `attachable` defaults to `false` and is configurable at the project level.
- Stack-scoped networks follow stack mode and are instance-bound:
  - shared stacks get one network
  - partitioned stacks get one network per partition
- Stateful services must use volumes and will require node constraints.
- Warn when a shared stack attaches to many partition networks (threshold configurable).

## Secrets Engines
Secrets are resolved during plan/apply via pluggable providers and resolvers:
- `vault://path#key`
- `bao://path#key` or `openbao://path#key`
- later: `aws://...`, `gcp://...`

Authentication:
- Dev/operator: env vars allowed.
- Automation: zero-trust auth (OIDC/JWT/AppRole/etc).
- Interactive runs may renew tokens; automation requires fresh short-lived tokens.
- Env var file support: `*_FILE` variants are supported for Vault tokens/credentials.

Missing secrets:
- Default: **block apply**.
- Override: `--allow-missing-secrets` inserts placeholders.
  - Placeholders are clearly invalid and trigger warnings.

Caching:
- In-memory only by default.
- Optional `--cache-secrets` for dev/operator use.

Write capability:
- Tool detects write access but does not write during apply.
- Secret writes are explicit via a dedicated command (e.g., `secrets put`).

Provider-specific blocks (e.g., `vault`) are used for settings like KV mounts and path templates.

Planned commands:
- `secrets check`: report missing secrets required by templates.
- `secrets put`: write a secret value to the secrets file (if provided) or to the secrets engine.

## Change Detection and Apply Flow
1) Load YAML + templates.
2) Resolve configs/secrets with secrets engine.
3) Compute desired state and diff against swarm.
4) Apply changes:
   - create/update configs/secrets
   - deploy stacks via `docker stack deploy` (per stack instance)
   - ensure networks/volumes exist (non-destructive by default)
   - prune unused managed configs/secrets only when `--prune` is set
5) Verify healthchecks; rollback on failure.
   - Healthchecks are required unless `--skip-healthcheck` is provided.

Managed diff/status scope (current):
- Configs/secrets: presence and labels (including `swarmcp.io/hash`) for managed resources.
- Networks: missing overlay networks derived from the config.
- Services: image, command/args, workdir, env, ports, mode/replicas, healthcheck, network attachments, volume mounts, and config/secret mounts (placement/resources are not compared yet).

## Execution Targeting (Deployment + Partition + Stack)
Runtime commands support selector-based scope narrowing. Effective scope is the intersection of all provided selectors.

Selectors:
- `--deployment <name>`: selects a deployment execution target. This chooses deployment overlays, values scope, context resolution, and node targeting.
- `--partition <name>`: limits partitioned stack instances to one partition.
- `--stack <name>`: limits runtime scope to one logical stack key under `stacks:`.

Validation and resolution:
- `--stack` must reference an existing logical stack key; unknown stack is an error.
- `--partition` must be in `project.partitions`; unknown partition is an error.
- If both deployment and partition are selected and the deployment target declares `partitions`, the selected partition must be a member of that allowlist; otherwise the command fails.
- For runtime commands with multiple partitions selected under one deployment, the effective partition set is the intersection of:
  - the user-selected partitions, if any
  - the deployment target's declared `partitions`, if any
  - `project.partitions`
- For partitioned stacks:
  - no `--partition`: all project partitions in scope.
  - with `--partition`: only that partition in scope.
- For shared stacks, partition selector does not create per-partition instances and does not change the resolved shared-stack service/config/secret model.

Selector cardinality by command family:
- Runtime commands (`plan`, `diff`, `status`, `apply`) may accept multiple `--deployment`, `--partition`, and `--stack` values.
- For runtime commands, repeated selector flags act as set filters after de-duplication.
- Runtime evaluation order is:
1. Iterate each selected deployment as an independent execution target.
2. Within that deployment, compute the eligible partition set for that deployment.
3. Within that deployment, select matching logical stacks.
4. For each selected stack:
   - if `mode: shared`, evaluate exactly one shared instance with `partition=""`
   - if `mode: partitioned`, evaluate the selected eligible partitions for that deployment
- Introspection and single-target commands (`resolve`, `explain`, `validate`, `sources`, `bootstrap`, `secrets put`) accept at most one effective deployment, partition, and stack selector unless the command explicitly documents broader behavior.
- Commands that require one effective target must fail on repeated selector values instead of silently choosing one.

Project context versus runtime target:
- Commands built around a single resolved view use one project context directly.
- Runtime commands use a higher-level runtime-target loop that constructs one project context per effective deployment, then applies partition and stack filters inside that deployment run.
- The spec should treat this split as intentional:
  - `ProjectContext` answers "what is the current effective config + context?"
  - runtime targeting answers "which effective contexts and stack instances should this command execute against?"

Command semantics:
- `plan`: render/report only targeted stack instances and only their related configs/secrets/mounts/networks/bind paths.
- `diff`: compute and report drift only for targeted stack instances/resources.
- `status`: report only targeted stack instances/resources and their service health.
- `apply`: reconcile only targeted stack instances/resources.

Stack and service inclusion targeting:
- After deployment, partition, and stack selectors are resolved, each selected stack is filtered by its effective stack-level `included_in` rules for that runtime target.
- A stack excluded by stack-level `included_in` is treated as absent from that target's resolved runtime model.
- Only after a stack is included are its services filtered by their effective service-level `included_in` rules for that runtime target.
- A service excluded by `included_in` is treated as absent from that target's resolved runtime model.
- Service inclusion cannot cause an excluded stack to exist for a target.
- Excluded services are not rendered, deployed, diffed, status-checked, or considered for service-scoped bind/config/secret/volume planning for that target.
- `included_in` does not create new deployments or logical stacks.
- The primary use of `included_in` is deployment-based inclusion and exclusion.
- For partitioned stacks, `included_in.partitions` may optionally narrow which existing partition instances are in scope when that level of control is actually needed.

Shared-stack partition rule:
- Partition overlays, partition-scoped stack config/secret defs, and partition-specific service overlays are defined against a partition context.
- Those partition-scoped mechanisms apply to partitioned stack instances only.
- `resolve` and `explain` should not present a partition-specialized variant of a shared stack just because `--partition` was provided.
- If future behavior intentionally allows partition-conditioned shared stacks, that must be specified as a new feature because it changes the current stack identity model.

Deployment-partition compatibility:
- `project.partitions` declares the global partition vocabulary for the project.
- `project.deployment_targets.<name>.partitions` optionally declares which of those partitions are valid when that deployment is active.
- This compatibility rule is normative for command validation, values scope selection, and runtime targeting.
- A deployment must not name a partition that is absent from `project.partitions`.
- If `project.deployments` lists a deployment name and `project.deployment_targets.<name>` exists, the allowlist in that deployment target is the source of truth for deployment-partition compatibility.
- If a project uses deployments but omits `project.deployment_targets.<name>.partitions`, the deployment is treated as compatible with all project partitions for backward compatibility.

Prune behavior with stack targeting:
- `--prune-services`: may remove services only within targeted stack instances (`docker stack deploy --prune` limited to targeted deploys).
- `--prune` (configs/secrets): may remove only managed configs/secrets labeled for targeted stack scope; no cross-stack cleanup when `--stack` is set.

## Service Dependencies and Update Policy
- Dependencies are explicit via `depends_on`.
- One-shot lifecycle work is explicit via service-local `jobs` phases.
- `depends_on` is for steady-state service dependency and reachability only; it must not be used to imply migration completion, rollback hooks, or other one-shot execution.
- Updates run in topological order; independent services update in parallel batches.
- Auto-attach required networks for dependency reachability within the same partition (overrideable).
- Egress networks are never auto-attached; services must opt in with `egress: true`.
- Failure scope: rollback the failed service only.

## Rollback Policy
- Default health timeout: 2 minutes per service.
- Success requires all desired replicas to be healthy.
- Failure conditions: task healthcheck failures or no healthy replicas by timeout.
- Retry: none by default (fast fail).
- Rollback enabled by default; override with `--no-rollback`.
- In parallel batches, keep successful updates; rollback only failed services.

## State and Cache
- Source of truth: swarm state + labels.
- Local state cache written after plan/apply: `.swarmcp/<config-file-without-extension>.state` (JSON).
- Cache is informational today; plan/apply do not read it yet.
- Unmanaged resources trigger warnings by default; suppress with `--no-warn-unmanaged`.

## CLI (Cobra)
- `plan`: compute desired state and show changes.
- `diff`: show resource-level differences (missing/stale configs/secrets, mount drift, missing services).
- `apply`: reconcile to desired state.
  - `--prune`: remove unused managed configs/secrets.
  - `--preserve <n>`: keep the most recent `n` unused configs/secrets when pruning.
  - `--confirm`: enable confirmation prompts for prune operations.
  - `--output <auto|summary|stack|error-only>`: control deploy log rendering during apply; when explicitly set, it implies `--no-ui`.
- `status`: show managed resources, mount drift, and service health (desired/running task counts; desired=0 treated as disabled).
- `secrets check`: report missing secrets required by templates.
- `secrets put`: write a secret value to the secrets file or secrets engine.
- `bootstrap networks`: create required overlay networks for the project.
- `bootstrap labels`: apply auto volume labels to swarm nodes and write them back to the project file.
  - `--prune-auto-labels`: remove auto volume labels that are no longer required by the current execution.
- `validate`: schema + template validation.

Debug output:
- `plan --debug` prints derived physical names and labels for rendered configs/secrets.
- `plan --debug-content` prints rendered config/secret content.
- `plan --debug-content-max <bytes>` prints content with a max length (0 for unlimited).
- `plan` prints required bind paths for services with volume mounts (and eligible nodes when deployment targets/nodes are configured).

Values store:
- `--values <path>` (repeatable) loads one or more YAML/JSON files and merges them.
- Later files override earlier ones (map keys override, arrays replaced).
- Templates in values files (`.tmpl`) are rendered before merge.
- Values templates may use `runtime_value` tokens with project/deployment/partition scope.
- `--deployment <name>` overrides `project.deployment` at runtime.
- `--partition <name>` limits planning/validation to a single partition.
- `--stack <name>` limits runtime commands (`plan`/`diff`/`status`/`apply`) to a single logical stack.
- Map keys ending in `+` append to lists (e.g., `ports+`).
- Map keys ending in `~` perform keyed list merges using `_key` (default: `name`).
- For scalar lists, `~` treats the item value as the key; unmatched items are appended (duplicates preserved).
- Overlays deep-merge config/secret definitions; fields omitted in overlays inherit from base.

## Resolve (Resolved Config Model)
`resolve` prints the final resolved config model for the selected scope.

Example:
```bash
swarmcp resolve \
  --config project.yaml \
  --release-config releases/prod-2026-03-10.yaml \
  --deployment prod \
  --partition blue \
  --stack core
```

Purpose:
- `resolve` is a config introspection command.
- It answers what config model SwarmCP computed after layering and imports, what deployment/partition/stack scoped overlays contributed to the final result, and what subtree exists at a selected field path.
- It does not query Swarm, does not compute a reconciliation plan, and does not print rendered config/secret payload bodies or secret values.

Resolution pipeline:
1. Load repeated `--config` files left to right.
2. Load repeated `--release-config` files and validate each against the allowlisted release-config fields.
3. Apply selected `project.values` refs and `stacks.<name>.source.ref` fields to the pre-import config.
4. Resolve stack and service imports using the selected source refs.
5. Validate and apply release service image pins and deploy-intent fields against the post-import resolved services.
6. Resolve or load selected `project.values` sources for the release scope without allowing the release config to redefine concrete values or values paths.
7. Apply runtime `--deployment` selection.
8. Apply deployment, partition, stack, and service overlays for the active scope.
9. Print the resulting resolved config model or selected subtree.

The split between steps 3 and 5 is intentional. Source refs must be known before import expansion, while service pins inside imported stacks cannot be safely validated until the imported stack exists in the resolved model.

Output:
- Default output format is YAML.
- `--output json` prints JSON.
- Output must be machine-readable and must not include banners, summaries, or status lines.
- Output excludes rendered config/secret payload content, secret values, Swarm runtime state, and internal loader/runtime-only fields.

Flags:
- `--config <path>` repeatable
- `--release-config <path>` repeatable
- `--deployment <name>`
- `--partition <name>`
- `--stack <name>`
- `--offline`
- `--output yaml|json`
- `--path <field-path>`

Scope rules:
- `--deployment` accepts at most one value.
- `--partition` accepts at most one value.
- `--stack` accepts at most one value.
- If `--deployment` is omitted, `project.deployment` is used if set; otherwise deployment-specific overlays are not selected.
- If `--partition` is omitted, `resolve` may print all partitions in scope.
- If `--stack` is omitted, `resolve` may print all stacks in scope.
- `resolve` is allowed to print a broad model when selectors are omitted.

Path lookup:
- `--path` is evaluated against the exact resolved model that `resolve` would otherwise print.
- `resolve` does not perform additional implicit narrowing for path lookup.
- If the selected path points to a scalar, print only that scalar value.
- If the selected path points to a map/object or list, print only that subtree.
- If the selected path does not exist in the resolved model, fail with a field-path-not-found error.
- Field paths use dot notation and are rooted at the resolved model.

Acceptance criteria:
- `resolve` prints the resolved model after repeated `--config` layering, repeated `--release-config`, release source-ref selection, import resolution, release service-intent application, runtime deployment selection, and scoped overlay application.
- `resolve` does not query Swarm.
- `resolve --output json` produces valid JSON with no extra text.
- `resolve --path` prints only the selected subtree or scalar.
- Repeated `--deployment`, `--partition`, and `--stack` values are rejected.

Error cases:
- Missing project config: fail with the existing config-path error.
- Repeated `--deployment`: fail with a single-value selector error.
- Repeated `--partition`: fail with a single-value selector error.
- Repeated `--stack`: fail with a single-value selector error.
- Unknown selected deployment/partition/stack: fail with the existing selector validation error.
- Unknown field path passed via `--path`: fail with a field-path-not-found error.
- Invalid `--output` value: fail with an output-format error.

## Explain (Config Provenance)
`explain` reports why a resolved field has its final value.

Example:
```bash
swarmcp explain \
  --config project.yaml \
  --release-config releases/prod-2026-03-10.yaml \
  --deployment prod \
  --partition blue \
  --stack core \
  stacks.core.services.api.image
```

Purpose:
- `explain` is a provenance command built on the same resolved config pipeline as `resolve`.
- It answers what the final resolved value is at a field path, which layers contributed to that value, and which layer won after precedence was applied.
- `explain` is intended to explain the resolved config model, not rendered artifact content or runtime behavior.

Resolution pipeline:
1. Load repeated `--config` files left to right.
2. Load repeated `--release-config` files and validate each against the allowlisted release-config fields.
3. Apply selected `project.values` refs and `stacks.<name>.source.ref` fields to the pre-import config.
4. Resolve stack and service imports using the selected source refs.
5. Validate and apply release service image pins and deploy-intent fields against the post-import resolved services.
6. Resolve or load selected `project.values` sources for the release scope without allowing the release config to redefine concrete values or values paths.
7. Apply runtime `--deployment` selection.
8. Apply deployment, partition, stack, and service overlays for the active scope.
9. Evaluate the requested field path against the resulting resolved model.
10. Print the final value, contributing layers, and winning layer.

Flags:
- `--config <path>` repeatable
- `--release-config <path>` repeatable
- `--deployment <name>`
- `--partition <name>`
- `--stack <name>`
- `--offline`

Scope rules:
- `--deployment` accepts at most one value.
- `--partition` accepts at most one value.
- `--stack` accepts at most one value.
- `explain` must operate on one effective target context for the requested field.
- `explain` must not silently choose one of several valid target contexts.

Narrowing behavior:
- If the requested field is project-scoped and unaffected by partition or stack selection, omitted `--partition` or `--stack` values are allowed.
- If omitted selectors would leave multiple effective partition-, stack-, or service-overlay contexts that could affect the requested field, `explain` must fail and require the user to narrow the target.
- `swarmcp explain --config project.yaml project.name` may succeed without `--partition` or `--stack`.
- `swarmcp explain --config project.yaml stacks.core.services.api.image` may require `--partition` and/or `--stack` if overlays or partition-specific structure could affect the result.

Field-path syntax:
- Paths are rooted at the resolved config model.
- Dot notation is used for map/object traversal.

Examples:
- `project.name`
- `stacks.core.services.api.image`
- `stacks.core.partitions.dev.configs.app_yaml`

Output contract:
- Print the requested field path.
- Print the final resolved value.
- Print the contributing layers in precedence order.
- Identify the winning layer.

Layer types to report in v1:
- base config file
- later repeated `--config` overlay files
- repeated `--release-config` overlay files
- imported stack/service source file
- imported remote override documents
- local import override blocks
- project deployment overlay
- project partition overlay
- stack deployment overlay
- stack partition overlay
- service deployment overlay
- service partition overlay

Example output shape:
```text
explain OK
path: stacks.core.services.api.image
final: ghcr.io/acme/api:1.8.4

layers:
1. config project.yaml -> ghcr.io/acme/api:main
2. config releases/qa.yaml -> ghcr.io/acme/api:1.8.4

winner:
- config releases/qa.yaml
```

Acceptance criteria:
- `explain` resolves the same config model pipeline as `resolve`.
- `explain` prints the requested path, final value, contributing layers, and winning layer.
- Repeated `--config` files appear as distinct provenance layers when they affect the resolved field.
- Repeated `--deployment`, `--partition`, or `--stack` values are rejected.
- Ambiguous target contexts are rejected instead of being silently chosen.
- Requests for paths outside the resolved model fail with a clear error.

Error cases:
- Missing explain path argument: fail with a required-argument error.
- Repeated `--deployment`: fail with a single-value selector error.
- Repeated `--partition`: fail with a single-value selector error.
- Repeated `--stack`: fail with a single-value selector error.
- Unknown selected deployment/partition/stack: fail with the existing selector validation error.
- Unknown field path: fail with a field-not-found error naming the requested path.
- Path resolves through a non-traversable scalar/list step: fail with a clear invalid-path error naming the blocked segment.
- Omitted selectors leave multiple effective target contexts for the requested field: fail with an error that tells the user which selector to add.

## YAML Schema
The project authoring schema lives at `schemas/swarmcp-project.v1.schema.json`. The release-config authoring schema lives at `schemas/swarmcp-release.v1.schema.json`. They are intended for editor completion and basic shape validation. `swarmcp validate` remains the normative semantic validator.

Outline:
```yaml
project:
  name: <string>
  partitions: [<string>] # "_" reserved
  deployments: [<string>]
  deployment: <string> # selects overlays.deployments.<name>
  contexts:
    <deployment>: <string> # docker context name or endpoint
  deployment_targets:
    <name>:
      partitions: [<string>] # optional allowlist; values must exist in project.partitions
      include:
        names: [<string>]
        labels:
          <key>: <value>
      exclude:
        names: [<string>]
        labels:
          <key>: <value>
      overrides:
        <node_name>:
          roles: [manager|worker]
          labels:
            <key>: <value>
          volumes:
            - <name>
          platform:
            os: <string>
            arch: <string>
  preserve_unused_resources: <int> # default: 5
  defaults:
    networks:
      internal: <string> # supports <partition> token
      egress: <string>
      attachable: <bool>
      shared:
        - <string> # supports <project> and <partition> tokens
    volumes:
      driver: <string>
      base_path: <path>
      service_standard: <name> # reserved standard name for per-service bind mounts (default: service)
      service_target: <path> # default container target for service-standard volumes (default: /data)
      node_label_key: <string>
      standards:
        <name>:
          source: <path>
          target: <path>
          readonly: <bool>
          requires:
            roles: [manager|worker]
  nodes:
    <node_name>:
      roles: [manager|worker]
      labels:
        <key>: <value>
      volumes:
        - <name>
      platform:
        os: <string>
        arch: <string>
  sources:
    url: <string>
    ref: <string>
    path: <string>
  configs:
    <name>:
      source: <path>
      target: <path>
      uid: <string>
      gid: <string>
      mode: <string>
  secrets:
    <name>:
      source: <path>
      target: <path>
      uid: <string>
      gid: <string>
      mode: <string>
  secrets_engine:
    provider: vault|bao|openbao
    addr: <url>
    auth:
      method: oidc|approle|jwt|kubernetes|tls
      path: <string>
      role: <string>
      audience: <string>
    vault:
      mount: <string>
      path_template: "{project}/{partition}/{stack}/{service}" # <partition> segment omitted for shared stacks

overlays:
  deployments:
    <name>:
      project:
        sealed: <bool>
        secrets_engine:
          provider: vault|bao|openbao
          addr: <url>
          auth:
            method: oidc|approle|jwt|kubernetes|tls
            path: <string>
            role: <string>
            audience: <string>
          vault:
            mount: <string>
            path_template: "{project}/{partition}/{stack}/{service}"
          # replaces base project.secrets_engine as a whole object when present
        sources:
          url: <string>
          ref: <string>
          path: <string>
        configs:
          <name>:
            source: <path>
            target: <path>
            uid: <string>
            gid: <string>
            mode: <string>
        secrets:
          <name>:
            source: <path>
            target: <path>
            uid: <string>
            gid: <string>
            mode: <string>
      stacks:
        <stack>:
          sealed: <bool>
          included_in:
            - deployments: [<deployment>]
              partitions: [<partition>]
              stacks: [<stack>]
          sources:
            url: <string>
            ref: <string>
            path: <string>
          services:
            <service>:
              # overrides only; no source/overrides in overlays
              sealed: <bool>
              image: <string>
              included_in:
                - deployments: [<deployment>]
                  partitions: [<partition>]
                  stacks: [<stack>]
              env:
                <key>: <value>
              labels:
                <key>: <value>
              replicas: <int>
              mode: replicated|global
          configs:
            <name>:
              source: <path>
              target: <path>
              uid: <string>
              gid: <string>
              mode: <string>
          secrets:
            <name>:
              source: <path>
              target: <path>
              uid: <string>
              gid: <string>
              mode: <string>
          partitions:
            <partition>:
              sealed: <bool>
              sources:
                url: <string>
                ref: <string>
                path: <string>
              configs:
                <name>:
                  source: <path>
                  target: <path>
                  uid: <string>
                  gid: <string>
                  mode: <string>
          secrets:
            <name>:
              source: <path>
              target: <path>
              uid: <string>
              gid: <string>
              mode: <string>
  partitions:
    # list form (preferred)
    - name: <string>
      match:
        # string form infers exact or glob
        partition: <string|glob>
        # map form allows explicit type selection
        # partition:
        #   type: exact|glob|regexp
        #   pattern: <string>
      project:
        sealed: <bool>
        secrets_engine:
          provider: vault|bao|openbao
          addr: <url>
          auth:
            method: oidc|approle|jwt|kubernetes|tls
            path: <string>
            role: <string>
            audience: <string>
          vault:
            mount: <string>
            path_template: "{project}/{partition}/{stack}/{service}"
          # replaces deployment/base project.secrets_engine as a whole object when present
        sources:
          url: <string>
          ref: <string>
          path: <string>
        configs:
          <name>:
            source: <path>
            target: <path>
            uid: <string>
            gid: <string>
            mode: <string>
        secrets:
          <name>:
            source: <path>
            target: <path>
            uid: <string>
            gid: <string>
            mode: <string>
      stacks:
        <stack>:
          sealed: <bool>
          included_in:
            - deployments: [<deployment>]
              partitions: [<partition>]
              stacks: [<stack>]
          sources:
            url: <string>
            ref: <string>
            path: <string>
          services:
            <service>:
              # overrides only; no source/overrides in overlays
              sealed: <bool>
              image: <string>
              included_in:
                - deployments: [<deployment>]
                  partitions: [<partition>]
                  stacks: [<stack>]
              env:
                <key>: <value>
              labels:
                <key>: <value>
              replicas: <int>
              mode: replicated|global
          configs:
            <name>:
              source: <path>
              target: <path>
              uid: <string>
              gid: <string>
              mode: <string>
          secrets:
            <name>:
              source: <path>
              target: <path>
              uid: <string>
              gid: <string>
              mode: <string>
          partitions:
            <partition>:
              sealed: <bool>
              sources:
                url: <string>
                ref: <string>
                path: <string>
              configs:
                <name>:
                  source: <path>
                  target: <path>
                  uid: <string>
                  gid: <string>
                  mode: <string>
              secrets:
                <name>:
                  source: <path>
                  target: <path>
                  uid: <string>
                  gid: <string>
                  mode: <string>
    # map form (legacy; exact match)
    <partition>:
      project: <same as list entry>
      stacks: <same as list entry>

stacks:
  <stack>:
    source:
      url: <string>
      ref: <string>
      path: <string>
      overrides_path: <string>
    overrides: { ... } # stack fields (when source is set)
    mode: shared|partitioned
    included_in:
      - deployments: [<deployment>] # optional
        partitions: [<partition>] # optional; invalid for mode: shared
        stacks: [<stack>] # optional
    volumes:
      <name>:
        subpath: <string> # optional; appended after stack segment
        target: <path> # container mount target
    partitions:
      <partition>:
        sources:
          url: <string>
          ref: <string>
          path: <string>
        configs:
          <name>:
            source: <path>
            target: <path>
            uid: <string>
            gid: <string>
            mode: <string>
        secrets:
          <name>:
            source: <path>
            target: <path>
            uid: <string>
            gid: <string>
            mode: <string>
    overlays:
      <name>:
        sealed: <bool>
        included_in:
          - deployments: [<deployment>]
            partitions: [<partition>] # optional; invalid for mode: shared
            stacks: [<stack>]
        sources:
          url: <string>
          ref: <string>
          path: <string>
        services:
          <service>:
            # overrides only; no source/overrides in overlays
            sealed: <bool>
            image: <string>
            included_in:
              - deployments: [<deployment>]
                partitions: [<partition>]
                stacks: [<stack>]
            env:
              <key>: <value>
            labels:
              <key>: <value>
            replicas: <int>
            mode: replicated|global
        configs:
          <name>:
            source: <path>
            target: <path>
            uid: <string>
            gid: <string>
            mode: <string>
        secrets:
          <name>:
            source: <path>
            target: <path>
            uid: <string>
            gid: <string>
            mode: <string>
        partitions:
          <partition>:
            sealed: <bool>
            sources:
              url: <string>
              ref: <string>
              path: <string>
            configs:
              <name>:
                source: <path>
                target: <path>
                uid: <string>
                gid: <string>
                mode: <string>
            secrets:
              <name>:
                source: <path>
                target: <path>
                uid: <string>
                gid: <string>
                mode: <string>
# Note: `stacks.<stack>.overlays` is supported in stack templates as well as the project file.
    sources:
      url: <string>
      ref: <string>
      path: <string>
    configs:
      <name>:
        source: <path>
        target: <path>
        uid: <string>
        gid: <string>
        mode: <string>
    secrets:
      <name>:
        source: <path>
        target: <path>
        uid: <string>
        gid: <string>
        mode: <string>
    services:
      <service>:
        source:
          url: <string>
          ref: <string>
          path: <string>
          overrides_path: <string>
        overrides: { ... } # service fields (when source is set)
        image: <image>
        command: [<string>]
        args: [<string>]
        workdir: <path>
        included_in:
          - deployments: [<deployment>] # optional
            partitions: [<partition>] # optional
            stacks: [<stack>] # optional
        env:
          <key>: <value>
        ports:
          - target: <int>
            published: <int> # 0 for auto-assign
            protocol: tcp|udp
            mode: ingress|host
        mode: replicated|global
        replicas: <int>
        labels:
          <key>: <value>
        placement:
          constraints:
            - <constraint>
        healthcheck: { ... }
        depends_on: [<service>]
        jobs:
          before_update:
            - name: <string>
              command: [<string>]
              args: [<string>]
              workdir: <path>
              env:
                <key>: <value>
              timeout: <duration>
              cleanup: always|success|never
          after_update:
            - name: <string>
              command: [<string>]
              args: [<string>]
              workdir: <path>
              env:
                <key>: <value>
              timeout: <duration>
              cleanup: always|success|never
          on_rollback:
            - name: <string>
              command: [<string>]
              args: [<string>]
              workdir: <path>
              env:
                <key>: <value>
              timeout: <duration>
              cleanup: always|success|never
        egress: true|false
        network_ephemeral:
          internal: <bool> # default false
          attachable: <bool> # default true
        configs:
          - name: <name>
            source: <path> # optional; when set, defines the config at service scope
            target: <path>
            uid: <string>
            gid: <string>
            mode: <string>
        secrets:
          - name: <name>
            source: <path> # optional; when set, defines the secret at service scope
            target: <path>
            uid: <string>
            gid: <string>
            mode: <string>
        volumes:
          - name: <name>
            target: <path>
            subpath: <string> # optional; appended after service segment
          - standard: <service_standard> # reserved per-service bind mount
            target: <path> # optional override (defaults to defaults.volumes.service_target)
            subpath: <string> # optional; appended after service segment
            readonly: <bool> # optional
            category: <string> # optional; used by runtime_value standard_volumes filters
          - source: <path>
            target: <path>
            readonly: <bool> # optional; ad-hoc bind (no base path or placement rules)
          - standard: <name> # uses defaults.volumes.standards
            category: <string> # optional; used by runtime_value standard_volumes filters
        overlays:
          deployments:
            <name>:
              sealed: <bool>
              # overrides only; no source/overrides in overlays
              image: <string>
              included_in:
                - deployments: [<deployment>]
                  partitions: [<partition>]
                  stacks: [<stack>]
              env:
                <key>: <value>
              labels:
                <key>: <value>
              replicas: <int>
              mode: replicated|global
          partitions:
            - name: <string>
              match:
                partition: <string|glob>
              sealed: <bool>
              # overrides only; no source/overrides in overlays
              image: <string>
              included_in:
                - deployments: [<deployment>]
                  partitions: [<partition>]
                  stacks: [<stack>]
              env:
                <key>: <value>
              labels:
                <key>: <value>
              replicas: <int>
              mode: replicated|global
        sources:
          url: <string>
          ref: <string>
          path: <string>
```

Release config outline:
```yaml
project:
  values:
    - name: <existing git-backed project.values name>
      ref: <ref|tag|commit>
stacks:
  <stack>:
    source:
      ref: <ref|tag|commit>
    services:
      <service>:
        image: <image tag or digest>
        replicas: <int>
        env:
          <key>: <scalar>
        labels:
          <key>: <scalar>
        update_config:
          parallelism: <int>
        rollback_config:
          parallelism: <int>
```

`source` may include a YAML/JSON fragment selector like `values.yaml#/global/domain`.
If the source is a template, it is rendered first, then the fragment is resolved.
Non-scalar fragment values render as compact JSON (valid YAML) to keep inline usage simple.
The `values#/path` prefix resolves fragments from the merged values store (see CLI flags).

`included_in` schema notes:
- Each entry is a rule object.
- At least one of `deployments`, `partitions`, or `stacks` must be present in each rule.
- In normal use, rules should usually specify `deployments` and omit the other dimensions.
- Omitted dimensions are wildcards.
- Referenced deployment names must exist in `project.deployments` when that list is declared.
- Referenced partition names must exist in `project.partitions`.
- Referenced stack names must exist in `stacks`.
- `included_in` is allowed in normal config files and stack/service override overlays, but it is not allowed in release overlays.
- Stack-level `included_in` is evaluated before service-level `included_in`.
- Service-level `included_in` may only narrow membership within an already included stack; it must not widen stack inclusion.
- `partitions` should be considered an advanced narrowing field, primarily for partitioned stacks. It is not the primary design goal of `included_in`.

Note: secret sources are files like config sources. Secrets engine lookups occur inside templates via functions like `secret_value` or `secret_ref`.
If a secret definition omits `source`, it defaults to `inline: {{ secret_value "<name>" }}`. Service-level secrets without a source use this default only when no definition exists in higher scopes.
Sources inherit by scope: service overrides partition overrides stack overrides project.
Service entries without `source` are treated as mount refs unless no higher-scope definition exists.
Simple token expansion (`{project}`, `{deployment}`, `{partition}`, `{stack}`, `{service}`) is applied to:
- service label keys
- service env keys
- placement constraint strings
Stack and partition `secrets` accept either:
- a map of definitions (`name: {source: ...}`), or
- a list of secret entries (`- name: foo`, optional `source/target/uid/gid/mode`, or a scalar `- foo`).
Project `secrets` remains definitions-only.
Stack and partition `configs` follow the same shape as secrets (map or list); project `configs` remains definitions-only.

## Examples
Core example (draft):
```yaml
project:
  name: primary
  partitions: [dev, qa]
  defaults:
    networks:
      internal: primary_<partition>_internal
      egress: primary_egress
      attachable: false
    volumes:
      driver: local
      base_path: /srv/data
      layout: "/srv/data/<name>/<partition>"
      node_label_key: swarmcp.volume
  nodes:
    node-1:
      roles: [manager]
      labels:
        swarmcp.volume.db_data: "true"
      volumes:
        - db_data

stacks:
  core:
    mode: shared
    configs:
      traefik:
        source: templates/configs/traefik.yml.tmpl
      dynamic:
        source: templates/configs/dynamic.yml.tmpl
    services:
      ingress:
        image: ghcr.io/cmmoran/traefik:3.6b
        ports:
          - target: 80
            published: 80
            protocol: tcp
            mode: ingress
        egress: true
        healthcheck:
          interval: 10s
          timeout: 3s
          retries: 3
          start_period: 10s
        configs: [traefik, dynamic]
```

Examples:
- `examples/primary/project.yaml` and template files under `examples/primary/templates/` demonstrate a minimal config + templates layout.
- `examples/primary/secrets.yaml` provides a companion secrets values file for `--secrets-file`.
- Example run: `swarmcp plan --config examples/primary/project.yaml --secrets-file examples/primary/secrets.yaml --values examples/primary/values/values.yaml.tmpl`
- `examples/nginx/` provides a small, runnable nginx stack with configs, secrets, volumes, and an update-friendly HTML page.

## Appendix A. Implementation Notes (Non-Normative)
This appendix is non-normative. It records implementation-oriented notes and checklists that may change without changing the product contract.

### Repeated `--config` Layering
Implementation checklist:
1. CLI surface:
- Change `--config` from singular to repeatable while preserving `.swarmcp.project` compatibility.
2. Loader:
- Add a merged-config load path that accepts ordered config files, parses them as mapping documents, applies the field-policy merge rules, and then runs import resolution and validation once on the merged result.
3. Path semantics:
- Preserve the first config path as the base identity for relative paths, inferred values/secrets, and state/cache naming.
4. Validation:
- Reject later-file stack/service `overrides`.
- Preserve existing sourced-vs-local validation rules on the final merged config.
5. Tests:
- Add merge tests for map merge, scalar replace, list replace, whole-object replace, and invalid later-file `overrides`.
- Add command-loading tests proving repeated `--config` is honored consistently by `plan`, `diff`, `status`, `apply`, and `validate`.

### Runtime `--stack` Targeting
Notes:
- `--stack` applies to runtime commands (`plan`/`diff`/`status`/`apply`) and is independent of `diff config --stack`, which is a history filter.
- Schema/template validation remains global where required for config integrity, but runtime render/plan/apply surface is target-scoped.

Implementation checklist:
1. CLI surface:
- Add `Options.Stack string`.
- Add persistent flag `--stack` with help text aligned to logical stack key semantics.
2. Context and validation:
- Extend project context/options to carry stack selector.
- Validate stack existence during context load (same stage as deployment/partition checks).
3. Filtering model:
- Introduce stack filter in render/apply planning paths so desired state generation is stack-scoped.
- Keep existing deployment/partition behavior and treat stack as additional intersection filter.
4. Command wiring:
- Pass stack selector through `plan`/`diff`/`status`/`apply` pipelines.
- Scope warnings/output sections to selected stacks only.
5. Prune guards:
- For stack-scoped runs, constrain prune candidate selection to matching stack labels/scope IDs.
- Keep non-stack-scoped behavior unchanged.
6. State cache:
- Include stack selector in state snapshot/cache compatibility checks to avoid false no-op cache hits across different stack scopes.
7. Tests:
- Add positive/negative selector validation tests.
- Add command-level scope tests per command for shared and partitioned stacks.
- Add prune safety regression tests proving no cross-stack deletions during stack-scoped apply.

### `explain` Provenance
Implementation checklist:
1. Resolution:
- Reuse the same resolved-config pipeline as `resolve`.
2. Path lookup:
- Implement dotted-path lookup against the resolved config model with map-key traversal only for v1.
3. Provenance tracking:
- Capture provenance while merging repeated config files and applying import/overlay resolution rather than reconstructing it afterward.
4. Output:
- Print a stable, human-readable layer list and winner summary.
5. Tests:
- Add positive tests for repeated-config provenance, import provenance, and scoped overlay provenance.
- Add negative tests for unknown paths and multi-target selector rejection.

## Appendix B. Draft Design Areas (Non-Normative)
This appendix is non-normative. It records draft design areas that have not yet been promoted to stable contract.

### Volumes and Placement (Draft)
- Volume policy defaults are configurable at the project level (driver, base path, and node label key).
- Stack/service volume declarations define intent and mount targets; they do not override the base path.
- Computed bind paths are derived from the project base path and scope:
  - `<base>/<project>/<stack>` for stack-scoped volumes
  - `<base>/<project>/<stack>/<service>` for service-scoped volumes
  - `stack` is the resolved stack name (`<stack>` for shared, `<partition>_<stack>` for partitioned)
  - Optional subpath is appended after the stack or service segment for finer granularity (defaults to the volume name when omitted).
- Service-standard volumes:
  - A reserved standard name provides per-service persistent binds with the service-scoped path above.
  - The reserved name defaults to `service` and is configurable via `project.defaults.volumes.service_standard`.
  - `project.defaults.volumes.standards` may not define the reserved name.
  - Services may override the container `target`, `subpath`, and `readonly` for the service standard only.
  - Placement/label requirements use a derived volume name: `<resolved_stack>.<service>` (e.g., `core.postgres`, `usw_api.redis`).
- Example:
```yaml
project:
  defaults:
    volumes:
      base_path: /srv/swarm
      service_standard: persist
      service_target: /data

stacks:
  core:
    services:
      postgres:
        volumes:
          - standard: persist
            target: /var/lib/postgresql/data
```
- Bind mount prerequisites:
  - Host paths must exist on target nodes before scheduling (Swarm will not create them).
  - Path creation/ownership is a provisioning concern (bootstrap scripts, config management, or node init).
- Volume declarations imply placement constraints:
  - Each volume maps to a node label key/value pair (e.g., `swarmcp.volume.<volume>=true`).
  - Services using a volume inherit the required node label constraint.
  - If a service uses multiple volumes, all required labels must be satisfied.
- Plan/status report volume placement checks using deployment targets and node volume/label declarations.
- Node-volume mappings live in project config; the tool may auto-label nodes to satisfy placement.
- Standard mounts:
  - Defaults may define standard mounts for common ad-hoc binds.
  - Standard mounts may declare required roles (e.g., `manager` for Docker socket access).
- Node roles can be declared to identify managers for manager-only operations.
- Omitted node roles imply `worker`.
- Node label declarations are enforced; missing labels are applied when permitted.
- For shared stacks, volume layouts use `_` as the partition directory.
