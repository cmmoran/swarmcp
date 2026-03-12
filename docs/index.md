# SwarmCP Docs

This local docs site renders the repository's canonical Markdown files directly, so the spec, examples, and engineering notes stay in one place.

## Local Preview

Run the local docs server through `task`:

```bash
task docs:serve
```

The local site will be available at `http://127.0.0.1:8000/`.

## Build Check

```bash
task docs:build
```

## Content Layout

- [Specification](spec.md): product contract, command behavior, schema, and appendices.
- [Examples](examples/index.md): runnable example projects and workflows.

## Notes

- The docs site uses `mkdocs.yml` at the repo root.
- The task entrypoint is `Taskfile.yml` at the repo root.
- The site renders the canonical repo Markdown through lightweight wrapper pages in `docs/`, so the source of truth stays in the existing files.
- If you add more example guides or operational notes, prefer linking them into `mkdocs.yml` rather than creating duplicate content.
