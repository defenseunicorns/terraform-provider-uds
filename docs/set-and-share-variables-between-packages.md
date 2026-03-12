## Sharing zarf runtime set variables between packages

The `uds_package` resource now automatically persists runtime set variables produced by a package deployment (via Zarf actions that call `setVariable`) into two computed maps on the resource:

- `set_variables` (Read-Only, Map(String)): non-sensitive runtime variables written by the package at deploy time.
- `sensitive_set_variables` (Read-Only, Map(String), Sensitive): sensitive runtime variables written by the package at deploy time.

Notes:
- Only runtime set variables produced by the package deploy result are persisted to `set_variables` / `sensitive_set_variables`.
- Sensitive values remain sensitive in the Terraform state and will not be printed in plan output.
- If you require an explicit deployment ordering, add an explicit `depends_on` so the consumer waits for the producer.

Simple example showing a producer package that sets a runtime variable and a consumer package that consumes it:

```hcl
module "core_secrets" {
  source = "./modules/uds-package"
  pkg = {
    # This package produces sensitive variables using zarf action setVariables sesitive: true
    source                   = "oci://ghcr.io/example/core-secrets:1.0.0"
  }
}

module "app" {
  source = "./modules/uds-package"
  pkg = {
    source = "oci://ghcr.io/example/app:1.0.0"
    overrides = {
      appComponent = {
        appChart = [
          { path = "app.dbpass", value = module.core_secrets.sensitive_set_vars["DB_PASSWORD"] },
        ]
      }
    }
  }
}
```

Producer implementation notes:

- Your package's deploy actions should populate the package deploy result's VariableConfig with values via the Zarf SDK (for example, `vc.SetVariable("OUTPUT", "outval", false, ...)`). The provider will read those runtime SetVariables and persist them into `set_variables` / `sensitive_set_variables`.
```yaml
kind: ZarfPackageConfig
metadata:
  name: example-zarf-package
  description: "setVariables exampole"
  version: 0.1.0

components:
  - name: example
    required: true
    actions:
      onDeploy:
        before:
          - cmd: echo "this-will-be-the-value-of-KEY"
            setVariables:
              - name: KEY
                sensitive: false # set true if sensitive
```

Consumer notes:

- Reference runtime values via `uds_package.<name>.set_variables["KEY"]` for non-sensitive values and `uds_package.<name>.sensitive_set_variables["KEY"]` for sensitive values.
- Because `sensitive_set_variables` is marked sensitive, Terraform will treat those values as sensitive in state and hide them from plan output.

Examples and patterns:

- Keys are exposed using the runtime names set by the package; the provider's lookup is case-insensitive when validating uniqueness, but the stored keys preserve the names as provided by the package at runtime.
