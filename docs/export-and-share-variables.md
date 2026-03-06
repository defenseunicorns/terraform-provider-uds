## Exported variables and sharing between packages

The `uds_package` resource supports exporting zarf package variables produced by a package deployment. This lets one package expose values that other packages or resources in your configuration can consume.

- `export_vars` (Optional, Set of String): list of variable names (case-insensitive lookup) that the provider should export from the package's zarf variables. Use UPPER or lower case — lookup is case-insensitive but the exported key will preserve the user-provided name.
- `exported_vars` (Read-Only, Map(String), Sensitive): computed sensitive map containing the exported variables requested via `export_vars`. Values are strings; structured values are YAML-encoded.

Example: exporting a secret from a `core_secrets` package and consuming it from another package. The exported values are sensitive — they will not be printed in plan output.

```hcl
module "core_secrets" {
  source = "./modules/uds-package"
  pkg = {
    source                   = "oci://ghcr.io/example/core-secrets:1.0.0"
    export_vars              = toset(["DB_PASSWORD", "API_TOKEN"]) # request these names be exported
  }
}

module "app" {
  source = "./modules/uds-package"
  pkg = {
    source = "oci://ghcr.io/example/app:1.0.0"
    overrides = {
      appComponent = {
        appChart = [
          { path = "app.dbpass", value = module.core_secrets.exported_vars["DB_PASSWORD"] },
        ]
      }
    }
  }
}
```
