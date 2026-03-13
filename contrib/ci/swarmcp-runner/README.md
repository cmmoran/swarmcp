# `swarmcp-runner` CI Image

Helper image for CI/CD (including Drone docker-runner) that bundles:

- `swarmcp`
- `docker` CLI
- `git`
- `openssh-client`
- `yq`
- `jq`
- `bash`
- `curl`

## Build

From repo root:

```bash
CI_IMAGE=ghcr.io/<org>/swarmcp-runner:latest task ci-image
```

For a local, non-pushed runner image:

```bash
task runner:image
```

## Local Containerized Usage

The runner image can execute `swarmcp` functionally inside a container as long as the container can see:

- your workspace
- a Docker socket or Docker context metadata/TLS material
- optional SSH agent and `known_hosts` if imports use git over SSH
- optional `~/.gitconfig` if you rely on git URL rewrites

From repo root, the simplest local entrypoint is:

```bash
task runner:exec
```

That defaults to:

```bash
swarmcp version
```

Override the command with `RUNNER_CMD`, for example:

```bash
RUNNER_CMD='swarmcp plan --config deploy/platform/project.yaml --partition dev' task runner:exec
```

```bash
RUNNER_CMD='swarmcp apply --config deploy/platform/project.yaml --partition dev' task runner:exec
```

The Taskfile-mounted runner will:

- mount the current repo at `/workspace`
- mount `/var/run/docker.sock` when present
- mount `${DOCKER_CONFIG:-$HOME/.docker}` at `/root/.docker` when present
- forward `SSH_AUTH_SOCK` when present
- mount `~/.ssh/known_hosts` when present
- mount `~/.gitconfig` when present

This is sufficient for:

- local-engine access through `DOCKER_HOST=unix:///var/run/docker.sock`
- named Docker context resolution through mounted Docker config metadata
- remote git imports that rely on SSH agent auth or git URL rewrite config

## Typical Drone Step

```yaml
steps:
  - name: deploy-dev
    image: ghcr.io/<org>/swarmcp-runner:latest
    volumes:
      - name: docker_sock
        path: /var/run/docker.sock
    environment:
      DOCKER_HOST: unix:///var/run/docker.sock
    commands:
      - swarmcp version
      - swarmcp plan --config deploy/platform/project.yaml --partition dev
      - swarmcp apply --config deploy/platform/project.yaml --partition dev
```

Notes:

- Add SSH keys/known_hosts in CI if your stack imports use git over SSH.
- Mount Docker config/context data in CI if you use named Docker contexts instead of direct socket access.
- Pass `--values ...` and `--secrets-file ...` as needed by your project.
- If you need Vault userpass login, you can use `curl` + `jq` instead of the Vault CLI, for example:

```bash
export VAULT_TOKEN="$(
  curl -fsS \
    -H 'Content-Type: application/json' \
    --data "{\"password\":\"${VAULT_USERPASS_PASSWORD}\"}" \
    "${VAULT_ADDR%/}/v1/auth/userpass/login/drone" \
  | jq -r '.auth.client_token'
)"
```
