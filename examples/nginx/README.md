# Nginx Example

This example deploys a single nginx service with one config template, one secret, and a
service-standard persistent volume. It is intended to be a minimal, runnable stack.

## Prerequisites
- A Docker Swarm manager context (local or remote).
- `swarmcp` available on your PATH.
- Host paths prepared on target nodes (Swarm does not create bind paths):
  - `/srv/data/nginx/web/nginx` (service-standard volume bind source)
  - `/tmp/nginx-logs` (ad-hoc bind mount for logs)

## Files
- `project.yaml`: swarmcp configuration for the nginx stack.
- `values/values.yaml.tmpl`: values for `server_name` and `index_message`.
- `secrets.yaml`: htpasswd entry for basic auth.
- `templates/configs/`: nginx config and HTML template.

## Plan
From the repository root:
```bash
swarmcp plan \
  --config examples/nginx/project.yaml \
  --secrets-file examples/nginx/secrets.yaml \
  --values examples/nginx/values/values.yaml.tmpl \
  --debug
```

## Apply
```bash
swarmcp apply \
  --config examples/nginx/project.yaml \
  --secrets-file examples/nginx/secrets.yaml \
  --values examples/nginx/values/values.yaml.tmpl
```

## Verify
```bash
swarmcp status \
  --config examples/nginx/project.yaml \
  --secrets-file examples/nginx/secrets.yaml \
  --values examples/nginx/values/values.yaml.tmpl

docker stack services nginx
docker service ps nginx_web_nginx

curl -u admin:password http://<node-ip>:8080/
```

Notes:
- The stack name is `nginx_web` (project + stack), so use `docker stack services nginx_web`.
- If you see `invalid mount config for type "bind"`, ensure the bind paths exist on all nodes
  that can schedule the service or constrain placement to nodes that have them.
 - Successful response contains the HTML with your `index_message` and an `X-Example-Message` header.

## Update Example
Edit `examples/nginx/values/values.yaml.tmpl` and change `index_message`, then re-run
`swarmcp apply` with the same flags to roll out the updated HTML.

## Cleanup Configs and Secrets
`swarmcp apply` only removes stale managed configs/secrets when `--prune` is set.
To clean up this example:
1) Remove or rename the config/secret entries in `examples/nginx/project.yaml`.
2) Re-run `swarmcp apply --prune` with the same flags (add `--confirm` to enable the prompt).
3) Optionally pass `--preserve 0` to remove all unused resources; otherwise the default
   preserve count applies.

If you want to clean up manually, you can use:
```bash
docker config ls
docker secret ls
docker config rm <name>
docker secret rm <name>
```
## Optional: Placement Checks
If you want volume placement checks, add `project.nodes` and `project.deployment_targets`
for your swarm nodes. For the service-standard volume, the derived volume name is
`web.nginx` (stack `web`, service `nginx`).
