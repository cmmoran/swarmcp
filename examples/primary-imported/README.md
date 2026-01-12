# Primary Imported Example

This example mirrors `examples/primary/` but moves stack and service definitions
into external files referenced from `project.yaml`.

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
From the repository root:
```bash
swarmcp plan \
  --config examples/primary-imported/project.yaml \
  --secrets-file examples/primary-imported/secrets.yaml \
  --values examples/primary-imported/values/values.yaml.tmpl \
  --debug
```

## Apply
```bash
swarmcp apply \
  --config examples/primary-imported/project.yaml \
  --secrets-file examples/primary-imported/secrets.yaml \
  --values examples/primary-imported/values/values.yaml.tmpl
```

## Verify
```bash
swarmcp status \
  --config examples/primary-imported/project.yaml \
  --secrets-file examples/primary-imported/secrets.yaml \
  --values examples/primary-imported/values/values.yaml.tmpl

docker stack services primary_core
docker service ps primary_core_ingress
```

Notes:
- `--debug` in `plan` prints rendered configs/secrets plus the generated stack compose.
- If you rely on the secrets engine, omit `--secrets-file` and provide engine credentials.
