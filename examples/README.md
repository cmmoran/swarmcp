# SwarmCP Examples: Getting Started

This directory contains runnable example projects for SwarmCP. This guide walks through how SwarmCP works, how to plan and apply changes safely, and how to maintain long-running swarms.

## How SwarmCP Works

SwarmCP is a declarative control plane for Docker Swarm built around a YAML project config. The workflow is:

1) Load and validate the project config (and optional overlays).
2) Resolve sources (local paths, git sources, inline content, values fragments).
3) Render configs and secrets from templates and values.
4) Compute the desired state: configs, secrets, networks, and service intents.
5) Compare desired state with the running swarm.
6) Apply changes with safe, ordered operations.

### Key Concepts

- **Project config**: The root YAML describing stacks, services, configs, secrets, networks, and defaults.
- **Sources**: Config/secret content can come from local files, git sources, or inline content. Sources are resolved before rendering.
- **Templates**: Configs and secrets can be Go templates. Template functions (like `config_ref` and `secret_ref`) allow references to other configs/secrets.
- **Rendered defs**: After rendering, SwarmCP creates immutable configs/secrets named with a content hash and labels them for tracking.
- **Diff/Status**: SwarmCP computes drift between desired and current state, including config/secret churn and service intent differences.
- **Apply**: SwarmCP creates missing configs/secrets/networks, deploys stacks, and optionally prunes unused resources.

### What SwarmCP Manages

- **Configs/Secrets**: Rendered from templates, named with a hash suffix, and labeled for tracking.
- **Networks**: Derived from project defaults and per-stack/service requirements.
- **Services**: Generated from the config definitions and compared to existing Swarm services by intent.

SwarmCP does not modify resources it does not label as managed, and it will warn about unmanaged drift.

## Quick Start (Example: `examples/primary`)

Pick an example and run commands from the example directory (or pass `--config`).

```
cd examples/primary
swarmcp status
```

If this is your first run, check for missing prerequisites (e.g., Docker context, secrets file).

## Planning Changes

`swarmcp diff` shows the delta between your project config and the current swarm.

```
swarmcp diff
```

You will see counts of resources to create/delete and services to update. If configs/secrets are derived from git sources, `diff` will read them from cache or fetch as needed.

Use `--preserve` to control how many unused configs/secrets are preserved in reports (overrides `project.preserve_unused_resources`).

```
swarmcp diff --preserve 1
```

### Source-aware Diffs (git)

If you want to see content changes in git sources (per-file diffs), add `--sources`:

```
swarmcp diff --sources
```

This fetches the latest git refs for the source and prints per-file diffs between the cached commit and the new commit. Use `--offline` to avoid fetching.

## Applying Changes

Apply uses the plan computed from your config and deploys stacks. Use `--prune` to remove unused configs/secrets and prune removed services via `docker stack deploy --prune`.

```
swarmcp apply
```

```
swarmcp apply --prune
```

### Apply Options

- `--serial`: deploy stacks one at a time (default is all stacks concurrently).
- `--no-ui`: disable the stack UI and emit buffered output per stack.
- `--prune-services`: prune removed services without touching configs/secrets.
- `--preserve N`: preserve the most recent unused configs/secrets when pruning.

Apply reports what it did (created configs/secrets/networks, stacks deployed, and prune results).

## Maintaining a Swarm

### Status Checks

```
swarmcp status
```

`status` summarizes the swarm state, including:

- desired vs running services
- config/secret drift
- managed vs unmanaged resources

### Offline Operation

If you need to run without network access (no git fetch), use `--offline`. This requires that git sources are already cached.

```
swarmcp sources pull
swarmcp status --offline
```

If you change branches or add new git sources, pull again before going offline.

### Pruning

SwarmCP treats unused configs/secrets as stale by default. Use `project.preserve_unused_resources` in your config or `--preserve` on the CLI to keep a safety buffer.

Prune flow:

1) Confirm unused config/secret removal (when `--confirm` is set).
2) Optionally prune removed services via `docker stack deploy --prune`.

### Long-running Swarms

Recommended maintenance workflow:

1) `swarmcp diff` to inspect changes.
2) `swarmcp apply` (with `--prune` if you want cleanup).
3) `swarmcp status` to verify convergence.

If you run multiple environments (deployments/partitions), use `--deployment` and `--partition` to target specific slices.

## Sources Management

SwarmCP can fetch and manage external git sources independently of apply.

```
swarmcp sources
swarmcp sources pull
swarmcp sources diff
```

- `sources` or `sources view` lists all external sources detected in your config.
- `sources pull` fetches git sources into the local cache.
- `sources diff` compares cached metadata (commit/subtree) to the latest remote metadata.

## Tips

- Keep secrets values in a separate file and pass `--secrets-file`.
- Use `values` files to parametrize templates across deployments.
- For large stacks, omit `--serial` to deploy all stacks concurrently and reduce total apply time.
- If `diff` reports service intent drift you did not expect, inspect the specific service intent fields in the output.

## Example Layouts

- `examples/nginx`: minimal single-stack example.
- `examples/primary`: primary project with multiple stacks.
- `examples/primary-imported`: demonstrates stack import and git/local sources.
