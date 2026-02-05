# SwarmCP Redux Specification (Draft)

## Table of Contents
- [Goals](#goals)
- [Concepts and Naming](#concepts-and-naming)
- [Configuration Sources](#configuration-sources)
- [External Imports (Stacks and Services)](#external-imports-stacks-and-services)
- [Values Store (values#/path)](#values-store-valuespath)
- [Template Resolution and Scope](#template-resolution-and-scope)
- [Overlays (Deployment + Partition)](#overlays-deployment--partition)
- [Configs and Secrets (Swarm)](#configs-and-secrets-swarm)
- [Labeling](#labeling)
- [Service Intent (Milestone 2)](#service-intent-milestone-2)
- [Networks and Volumes](#networks-and-volumes)
- [Volumes and Placement (Draft)](#volumes-and-placement-draft)
- [Secrets Engines](#secrets-engines)
- [Change Detection and Apply Flow](#change-detection-and-apply-flow)
- [Service Dependencies and Update Policy](#service-dependencies-and-update-policy)
- [Rollback Policy](#rollback-policy)
- [State and Cache](#state-and-cache)
- [CLI (Cobra)](#cli-cobra)
- [YAML Schema](#yaml-schema)
- [Examples](#examples)

## Goals
- Provision and manage Docker Swarm resources from YAML configuration and templates.
- Provide atomic, healthcheck-gated service updates.
- Detect drift and reconcile configs, secrets, services, networks, and volumes.
- Support reusable templates and external secrets engines.

## Concepts and Naming
- **Project**: top-level grouping; declares partitions and shared/partitioned stacks.
- **Partition**: environment-like duplication (e.g., `dev`, `qa`, `uat`).
- **Reserved partition name**: `_` (used as the shared-stack placeholder).
- **Stack modes**:
  - **shared**: one per project. Name: `<project>_<stack>`.
  - **partitioned**: one per partition. Name: `<project>_<partition>_<stack>`.
- **Reserved stack**: `core`.

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
- Service template `config_ref` / `secret_ref` calls infer required mounts and definitions when no explicit `configs` / `secrets` are listed.
- Inferred configs default to `source: values#/name` (lookup uses normal values scoping).
- Inferred secrets default to `source: inline: {{ secret_value "name" }}`.
- `config_value` falls back to `values#/name` when no config definition exists (uses normal values scoping when a values store is configured). When the values entry is a map or list, the fallback returns the structured value (so `config_value_index` and `config_value_get` work).
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
- `config_ref "<name>"`
- `runtime_value "<template>"`
- `runtime_value "standard_volumes" ["standard=<name>[,<name>...]"] ["category=<name>[,<name>...]"] ["_format=csv|json|yaml"]`
- `external_ip`
- `escape_template "<expression-or-text>" ["<levels-or-open-delim>"] ["<open-delim>"] ["<close-delim>"]`
- `escape_swarm_template "<expression-or-text>" ["<levels-or-open-delim>"] ["<open-delim>"] ["<close-delim>"]`
- `swarm_network_cidrs [name]`

`config_value` and `config_ref` resolve names using the same scope hierarchy as config references. When inference is enabled, `config_value` falls back to `values#/name` if no config definition exists.
`config_value_index` and `config_value_get` help extract list and map values from config templates.
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
Service definition string fields (env values, image, command/args, placement constraints, network names, volume targets, config/secret mount fields, etc.) are rendered as templates using the service scope.
Bare `{{ ... }}` scalars in `project.yaml` are auto-quoted before YAML parsing, so unquoted template expressions are accepted.

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

Overlay scope (draft):
- Project, stack, and stack partition config/secret definitions.
- Stack service overrides via overlays (`overlays.*.stacks.<stack>.services.<service>`); overlay services merge into the base service definition and do not support `source` or `overrides`.
- Stack-level overlay definitions are supported at `stacks.<stack>.overlays.<name>` with the same rules as `overlays.*.stacks.<stack>`.
- Stack/service templates may also define `overlays` (`stacks.<stack>.overlays` inside stack templates, `services.<service>.overlays` inside service templates).
- Service template overlays are service-scoped maps (no `services:` nesting); service partition overlays may be a map (exact match) or a list with `match.partition`.
- `sealed: true` may be set on project/stack/service overlay blocks; sealed fields override non-sealed values even if they are narrower in scope.

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

Required:
- `image`

Optional:
- `command`, `args`, `workdir`
- `env` (key/value map)
- `ports` (`target`, `published`, `protocol`, `mode`)
- `mode` (`replicated` or `global`) and `replicas`
- `healthcheck`, `depends_on`, `egress`
- `restart_policy` (`condition`, `delay`, `max_attempts`, `window`)
- `update_config` (`parallelism`, `delay`, `failure_action`, `monitor`, `max_failure_ratio`, `order`)
- `rollback_config` (`parallelism`, `delay`, `failure_action`, `monitor`, `max_failure_ratio`, `order`)
- `labels` (merged with managed labels; `swarmcp.io/*` reserved)
- `placement.constraints` (Swarm placement constraint expressions)
- `configs`, `secrets`, `sources`

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

## Volumes and Placement (Draft)
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

## Service Dependencies and Update Policy
- Dependencies are explicit via `depends_on`.
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
- Map keys ending in `+` append to lists (e.g., `ports+`).
- Map keys ending in `~` perform keyed list merges using `_key` (default: `name`).
- For scalar lists, `~` treats the item value as the key; unmatched items are appended (duplicates preserved).
- Overlays deep-merge config/secret definitions; fields omitted in overlays inherit from base.

## YAML Schema
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
          sources:
            url: <string>
            ref: <string>
            path: <string>
          services:
            <service>:
              # overrides only; no source/overrides in overlays
              sealed: <bool>
              image: <string>
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
          sources:
            url: <string>
            ref: <string>
            path: <string>
          services:
            <service>:
              # overrides only; no source/overrides in overlays
              sealed: <bool>
              image: <string>
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
        sources:
          url: <string>
          ref: <string>
          path: <string>
        services:
          <service>:
            # overrides only; no source/overrides in overlays
            sealed: <bool>
            image: <string>
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

`source` may include a YAML/JSON fragment selector like `values.yaml#/global/domain`.
If the source is a template, it is rendered first, then the fragment is resolved.
Non-scalar fragment values render as compact JSON (valid YAML) to keep inline usage simple.
The `values#/path` prefix resolves fragments from the merged values store (see CLI flags).

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
