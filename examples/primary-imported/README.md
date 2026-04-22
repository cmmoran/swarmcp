# Primary Imported Example

This example uses the same general primary-style topology, but moves stack and
service definitions into imported local files referenced from `project.yaml`.

## Prerequisites
- A Docker Swarm manager context.
- `swarmcp` available on your PATH.
- Host paths prepared on target nodes (Swarm does not create bind paths):
  - `/srv/data/primary/core/ingress` (service-standard volume bind source)
  - `/var/log/traefik` (ad-hoc bind mount for logs)
- Docker socket access for `docker_sock` standard on manager nodes.

## Secrets Sources
This example is configured to use a secrets engine (`project.secrets_engine`).
For local testing with the included secrets file, pass `--secrets-file` explicitly.

## Plan
Validate first:
```bash
swarmcp validate \
  --config examples/primary-imported/project.yaml \
  --secrets-file examples/primary-imported/secrets.yaml \
  --values examples/primary-imported/values/values.yaml.tmpl
```

Then inspect a targeted nonprod slice from the repository root:
```bash
swarmcp plan \
  --config examples/primary-imported/project.yaml \
  --secrets-file examples/primary-imported/secrets.yaml \
  --values examples/primary-imported/values/values.yaml.tmpl \
  --deployment nonprod \
  --stack core \
  --debug
```

## Introspection
Inspect the resolved imported stack model:
```bash
swarmcp resolve \
  --config examples/primary-imported/project.yaml \
  --secrets-file examples/primary-imported/secrets.yaml \
  --values examples/primary-imported/values/values.yaml.tmpl \
  --deployment nonprod \
  --stack core \
  --output yaml
```

Explain the imported ingress image winner:
```bash
swarmcp explain stacks.core.services.ingress.image \
  --config examples/primary-imported/project.yaml \
  --secrets-file examples/primary-imported/secrets.yaml \
  --values examples/primary-imported/values/values.yaml.tmpl \
  --deployment nonprod
```

## Release Overlay
This example includes a sample deploy-time pin file at `examples/primary-imported/releases/nonprod.yaml`.
Because this project imports stacks from local files, the release overlay demonstrates release-compatible `source.ref` pins on imported stack definitions:
```bash
swarmcp plan \
  --config examples/primary-imported/project.yaml \
  --secrets-file examples/primary-imported/secrets.yaml \
  --values examples/primary-imported/values/values.yaml.tmpl \
  --deployment nonprod \
  --release-config examples/primary-imported/releases/nonprod.yaml
```

## Apply
```bash
swarmcp apply \
  --config examples/primary-imported/project.yaml \
  --secrets-file examples/primary-imported/secrets.yaml \
  --values examples/primary-imported/values/values.yaml.tmpl \
  --deployment nonprod
```

## Verify
```bash
swarmcp status \
  --config examples/primary-imported/project.yaml \
  --secrets-file examples/primary-imported/secrets.yaml \
  --values examples/primary-imported/values/values.yaml.tmpl \
  --deployment nonprod

docker stack services primary_core
docker service ps primary_core_ingress
```

Notes:
- `examples/primary-imported/.swarmcp.project` lets you run commands from that directory without repeating `--config`.
- `--debug` in `plan` prints rendered configs/secrets plus the generated stack compose.
- `resolve` may print broad models; `explain` is single-target and should be narrowed with deployment/partition/stack selectors when needed.
- `--release-config` is for deploy-time pins only. Keep topology changes in `project.yaml` and imported stack or service files.
- If you rely on the secrets engine, omit `--secrets-file` and provide engine credentials.
