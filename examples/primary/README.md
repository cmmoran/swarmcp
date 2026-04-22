# Primary Example

This example is a real-world Traefik ingress stack with deployments, partitions,
config templates, secrets, placement requirements, and a sample release overlay.

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
  --config examples/primary/project.yaml \
  --secrets-file examples/primary/secrets.yaml \
  --values examples/primary/values/values.yaml.tmpl
```

Then inspect a targeted nonprod slice from the repository root:
```bash
swarmcp plan \
  --config examples/primary/project.yaml \
  --secrets-file examples/primary/secrets.yaml \
  --values examples/primary/values/values.yaml.tmpl \
  --deployment nonprod \
  --partition dev \
  --stack core \
  --debug
```

## Introspection
Resolved config view for the core stack:
```bash
swarmcp resolve \
  --config examples/primary/project.yaml \
  --secrets-file examples/primary/secrets.yaml \
  --values examples/primary/values/values.yaml.tmpl \
  --deployment nonprod \
  --stack core \
  --output yaml
```

Explain one resolved field:
```bash
swarmcp explain stacks.core.services.ingress.image \
  --config examples/primary/project.yaml \
  --secrets-file examples/primary/secrets.yaml \
  --values examples/primary/values/values.yaml.tmpl \
  --deployment nonprod
```

## Release Overlay
This example includes a sample deploy-time pin file at `examples/primary/releases/nonprod.yaml`:
```bash
swarmcp plan \
  --config examples/primary/project.yaml \
  --secrets-file examples/primary/secrets.yaml \
  --values examples/primary/values/values.yaml.tmpl \
  --deployment nonprod \
  --release-config examples/primary/releases/nonprod.yaml
```

## Apply
```bash
swarmcp apply \
  --config examples/primary/project.yaml \
  --secrets-file examples/primary/secrets.yaml \
  --values examples/primary/values/values.yaml.tmpl \
  --deployment nonprod
```

## Verify
```bash
swarmcp status \
  --config examples/primary/project.yaml \
  --secrets-file examples/primary/secrets.yaml \
  --values examples/primary/values/values.yaml.tmpl \
  --deployment nonprod

docker stack services primary_core
docker service ps primary_core_ingress
```

Notes:
- `examples/primary/.swarmcp.project` lets you run commands from that directory without repeating `--config`.
- `--debug` in `plan` prints rendered configs/secrets plus the generated stack compose.
- `resolve` may print broad models; `explain` is single-target and should be narrowed with deployment/partition/stack selectors when needed.
- `--release-config` is for deploy-time pins only. Keep topology changes in `project.yaml`.
- If you rely on the secrets engine, omit `--secrets-file` and provide engine credentials.
