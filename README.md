# SwarmCP

SwarmCP is a YAML-driven control plane for Docker Swarm. It loads project configuration, values, release overlays, templates, and optional external sources, then computes and reconciles the desired state for stacks, services, configs, secrets, networks, and selected volume checks.

The repository contains the CLI, the implementation, the draft product specification, runnable examples, and a small docs site that renders the canonical markdown from the repo.

## What It Does

SwarmCP is built for repeatable Swarm operations around a declarative project model:

- Render configs and secrets from files, inline content, git sources, and `values#/path` references.
- Apply deployment and partition overlays without changing the base topology model.
- Compute desired service intent and diff it against current Swarm state.
- Create immutable configs and secrets with content-addressed names and management labels.
- Reconcile stacks with healthcheck-aware apply behavior and optional prune controls.
- Inspect resolved configuration and provenance with `resolve` and `explain`.
- Work with local secrets files or external secrets engines.
- Cache external git sources for offline or repeatable runs.

## Repository Layout

- `cmd/`: Cobra CLI commands.
- `internal/`: core implementation for config loading, rendering, apply, secrets, templates, and Swarm interactions.
- `examples/`: runnable example projects.
- `docs/`: MkDocs wrappers around the canonical repository markdown.
- `contrib/`: helper Dockerfiles and supporting scripts.
- `SPEC.md`: detailed product and schema specification.

## Installation

### Build from source

```bash
go build -o swarmcp .
```

### Local development build

```bash
task build
```

The built binary is written to `./swarmcp`.

### Install to `~/.local/bin`

```bash
task install
```

## Development Commands

- `task test`: run the Go test suite.
- `task build`: run tests and build the binary with version metadata.
- `task version`: print the computed version used by build and image tasks.
- `task docs:serve`: preview the docs site locally.
- `task docs:build`: build the docs site strictly in Docker.
- `task runner:image`: build the local runner container.
- `task runner:exec`: execute a `swarmcp` command inside the runner container.

## Core Model

SwarmCP is easiest to think about as three authoring artifacts:

- `project.yaml`: desired topology, imports, and structural defaults.
- `values/*.yaml`: template inputs.
- `release.yaml`: deploy-time pins passed with `--release-config`.

It also uses three runtime targeting axes:

- `deployment`: selects overlay/context/node-target behavior. It is not part of stack naming.
- `partition`: selects instances for partitioned stacks.
- `stack`: selects logical stack definitions under `stacks:`.

Shared stacks are named `<project>_<stack>`. Partitioned stacks are named `<project>_<partition>_<stack>`.

## Configuration Basics

A minimal project looks like this:

```yaml
project:
  name: nginx
  preserve_unused_resources: 5
  configs:
    server_name:
      source: values#/server_name
    index_message:
      source: values#/index_message
    nginx_conf:
      source: examples/nginx/templates/configs/nginx.conf.tmpl
      target: /etc/nginx/nginx.conf
  secrets:
    htpasswd:
      source: |
        inline:
          {{ secret_value "htpasswd" }}

stacks:
  web:
    mode: shared
    services:
      nginx:
        image: nginx:alpine
        replicas: 1
        configs:
          - name: nginx_conf
            target: /etc/nginx/nginx.conf
        secrets:
          - name: htpasswd
            target: /run/secrets/htpasswd
```

Related inputs typically live alongside it:

- values file: `values/values.yaml` or `values/values.yaml.tmpl`
- secrets file: `secrets.yaml`
- templates: `templates/...`

## Common Workflow

Validate first:

```bash
./swarmcp validate \
  --config examples/nginx/project.yaml \
  --values examples/nginx/values/values.yaml.tmpl \
  --secrets-file examples/nginx/secrets.yaml
```

Plan the desired state:

```bash
./swarmcp plan \
  --config examples/nginx/project.yaml \
  --values examples/nginx/values/values.yaml.tmpl \
  --secrets-file examples/nginx/secrets.yaml
```

Inspect the current-vs-desired diff:

```bash
./swarmcp diff \
  --config examples/nginx/project.yaml \
  --values examples/nginx/values/values.yaml.tmpl \
  --secrets-file examples/nginx/secrets.yaml
```

Apply changes:

```bash
./swarmcp apply \
  --config examples/nginx/project.yaml \
  --values examples/nginx/values/values.yaml.tmpl \
  --secrets-file examples/nginx/secrets.yaml
```

Check runtime status:

```bash
./swarmcp status \
  --config examples/nginx/project.yaml \
  --values examples/nginx/values/values.yaml.tmpl \
  --secrets-file examples/nginx/secrets.yaml
```

## Layering and Release Overlays

`--config` is repeatable. Later files overlay earlier files.

```bash
./swarmcp plan \
  --config project.yaml \
  --config overlays/shared-nonprod.yaml \
  --values values/nonprod.yaml
```

`--release-config` is also repeatable, but narrower by design. It is intended for deploy-time pins such as image refs, replica counts, and selected service intent changes.

```bash
./swarmcp apply \
  --config project.yaml \
  --values values/prod.yaml \
  --release-config releases/prod-2026-03-24.yaml
```

Use `--release-config` for release intent, not for topology rewrites.

## Targeting Deployments, Partitions, and Stacks

Runtime commands accept repeatable selectors:

```bash
./swarmcp diff \
  --config examples/primary/project.yaml \
  --deployment nonprod \
  --partition qa \
  --stack core
```

Notes:

- `plan`, `diff`, `status`, and `apply` can operate on repeated selector sets.
- `resolve`, `explain`, `validate`, `sources`, `bootstrap`, and `secrets put` are effectively single-target commands.
- Deployment targets may declare allowed partitions. Invalid deployment/partition combinations fail validation.

## Secrets Handling

SwarmCP supports two broad modes:

- local secrets values file via `--secrets-file`
- external secrets engine via `project.secrets_engine`

Check which secret values are currently required:

```bash
./swarmcp secrets check \
  --config examples/nginx/project.yaml \
  --values examples/nginx/values/values.yaml.tmpl \
  --secrets-file examples/nginx/secrets.yaml
```

Write a secret to a file-backed or engine-backed store:

```bash
./swarmcp secrets put htpasswd \
  --config examples/nginx/project.yaml \
  --secrets-file examples/nginx/secrets.yaml \
  --stdin
```

## Introspection Commands

Print the resolved config model:

```bash
./swarmcp resolve \
  --config examples/primary/project.yaml \
  --deployment nonprod \
  --output yaml
```

Print one path from the resolved model:

```bash
./swarmcp resolve \
  --config examples/primary/project.yaml \
  --path stacks.core.services.ingress.image
```

Explain where a resolved field came from:

```bash
./swarmcp explain stacks.core.services.ingress.image \
  --config examples/primary/project.yaml \
  --deployment nonprod
```

## External Sources

SwarmCP can read config and secret sources from local paths or git-backed source roots.

Useful commands:

- `swarmcp sources view`: list discovered external sources.
- `swarmcp sources pull`: fetch external git sources into cache.
- `swarmcp sources diff`: compare cached source metadata with remote state.
- `swarmcp diff --sources`: include source-aware diffs where available.

For offline runs, fetch sources first and then use `--offline`.

## Apply and Prune Behavior

`apply` reconciles only the targeted scope. Pruning is opt-in.

- `--prune`: remove unused managed configs/secrets and prune removed services.
- `--prune-services`: prune removed services only.
- `--preserve N`: keep the most recent unused managed configs/secrets.
- `--confirm`: enable confirmation prompts for prune operations.
- `--serial`: deploy one stack at a time.
- `--no-ui`: disable the apply UI and print buffered stack output.

SwarmCP labels managed resources and avoids mutating unmanaged resources directly. It may still report unmanaged drift or warnings unless `--no-warn-unmanaged` is set.

## Examples

Start with:

- [examples/nginx/README.md](examples/nginx/README.md): smallest runnable example.
- [examples/primary/README.md](examples/primary/README.md): multi-stack project with deployments, partitions, and ingress.
- [examples/primary-imported/README.md](examples/primary-imported/README.md): imported stacks and sources.
- [examples/README.md](examples/README.md): broader examples guide.

## Documentation

- [SPEC.md](SPEC.md): canonical behavior, schema, and command contract.
- [docs/index.md](docs/index.md): docs site entrypoint.

Preview the docs site locally:

```bash
task docs:serve
```

## Current Command Set

Top-level commands currently available:

- `apply`
- `bootstrap`
- `diff`
- `explain`
- `plan`
- `resolve`
- `secrets`
- `sources`
- `status`
- `validate`
- `version`

## Notes and Constraints

- Docker Swarm does not create host bind paths automatically. Required host paths must already exist on eligible nodes.
- Git-backed sources require network access unless already cached and used with `--offline`.
- `--allow-missing-secrets` exists for development and inspection flows, but it is not appropriate for normal production applies.
- The full behavior surface is still documented most precisely in `SPEC.md`; the README is the operator overview.
