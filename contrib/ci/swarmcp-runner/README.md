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
docker build \
  -f contrib/ci/swarmcp-runner/Dockerfile \
  -t ghcr.io/<org>/swarmcp-runner:latest \
  --build-arg VERSION="$(git rev-parse --short=12 HEAD)" \
  .
```

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
