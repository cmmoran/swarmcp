# SwarmCP Schemas

`swarmcp-project.v1.schema.json` provides editor completion and basic shape
validation for `project.yaml` files.

Use it with YAML language server comments:

```yaml
# yaml-language-server: $schema=../schemas/swarmcp-project.v1.schema.json
```

Adjust the relative path for the location of your project file.

The schema is intentionally an authoring aid, not a replacement for:

```bash
swarmcp validate --config project.yaml
```

SwarmCP accepts some compatibility syntax that standard YAML parsers and
language servers may not parse, especially bare Go-template expressions in
scalar positions. Quote template expressions when you want editor schema support.
