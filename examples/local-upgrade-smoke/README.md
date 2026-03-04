# Local Swarm Upgrade Smoke Example

This example is designed for Docker SDK/daemon upgrade validation with `swarmcp`.

Goals:
- Run on a local Swarm manager (`default` Docker context).
- No external sources (no git imports, no Vault/OpenBao, no remote templates).
- Minimal resources: one stack, one service, one rendered config.

## Prerequisites
- Your `default` Docker context points to a Swarm manager.
- `swarmcp` is available in your shell.

## Files
- `project.yaml`: minimal project and service definition.
- `values/values.yaml.tmpl`: message value used by the rendered config.
- `templates/configs/index.html.tmpl`: rendered into the nginx container.

## Baseline (Before Upgrade)
From repository root:

```bash
swarmcp plan \
  --config examples/local-upgrade-smoke/project.yaml \
  --values examples/local-upgrade-smoke/values/values.yaml.tmpl

swarmcp apply \
  --config examples/local-upgrade-smoke/project.yaml \
  --values examples/local-upgrade-smoke/values/values.yaml.tmpl \
  --output summary

swarmcp status \
  --config examples/local-upgrade-smoke/project.yaml \
  --values examples/local-upgrade-smoke/values/values.yaml.tmpl

swarmcp diff \
  --config examples/local-upgrade-smoke/project.yaml \
  --values examples/local-upgrade-smoke/values/values.yaml.tmpl

curl -fsS http://127.0.0.1:18080/
```

Expected:
- `apply OK`
- `status` shows no missing/stale resources for this example.
- `diff` shows no pending changes.
- `curl` returns HTML containing `local swarm smoke test`.

## Upgrade Validation (After SDK/Daemon Upgrade)
Run the same commands again. Success criteria:
- No command errors (`plan`, `apply`, `status`, `diff`).
- Service remains healthy and reachable on `http://127.0.0.1:18080/`.
- `diff` remains converged after `apply`.

## Optional Update Check
Change `app_message` in `values/values.yaml.tmpl`, then re-run:

```bash
swarmcp apply \
  --config examples/local-upgrade-smoke/project.yaml \
  --values examples/local-upgrade-smoke/values/values.yaml.tmpl \
  --output summary

curl -fsS http://127.0.0.1:18080/
```

This validates config content rollout and service update behavior.
