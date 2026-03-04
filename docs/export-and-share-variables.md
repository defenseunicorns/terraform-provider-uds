## Exported variables and sharing between packages

The `uds_package` resource supports exporting zarf package variables produced by a package deployment. This lets one package expose values that other packages or resources in your configuration can consume.

- `export_vars` (Optional, Set of String): list of variable names (case-insensitive lookup) that the provider should export from the package's zarf variables. Use UPPER or lower case — lookup is case-insensitive but the exported key will preserve the user-provided name.
- `exported_vars` (Read-Only, Map(String), Sensitive): computed sensitive map containing the exported variables requested via `export_vars`. Values are strings; structured values are YAML-encoded.
- `tolerate_missing_deployed` (Optional, Boolean): when set to `true`, the provider will retain the resource in state if the deployed package record is not found during refresh/read (useful for action-only packages that don't persist a deployed record). When `false` (default or omitted) a missing deployed record will remove the resource from state and recreate it on apply.

Example: exporting a secret from a `core_secrets` package and consuming it from another package. The exported values are sensitive — they will not be printed in plan output.

```hcl
module "core_secrets" {
  source = "./modules/uds-package"
  pkg = {
    source                   = "oci://ghcr.io/example/core-secrets:1.0.0"
    export_vars              = toset(["DB_PASSWORD", "API_TOKEN"]) # request these names be exported
    tolerate_missing_deployed = true
  }
}

module "app" {
  source = "./modules/uds-package"
  pkg = {
    source = "oci://ghcr.io/example/app:1.0.0"
    overrides = {
      appComponent = {
        appChart = [
          { path = "app.dbpass", value =  module.core_secrets.exported_vars["DB_PASSWORD"] },
        ]
      }
  }
}
```

Example `tofu plan` when `tolerate_missing_deployed = true` and the deployed record is missing (illustrative):

```text
No changes. Your infrastructure matches the configuration.

OpenTofu has compared your real infrastructure against your configuration and found no differences, so no changes are
needed.
╷
│ Warning: Deployed package not found (tolerated)
│ 
│   with module.core_secrets.uds_package.this,
│   on modules/uds-package/main.tf line 32, in resource "uds_package" "this":
│   32: resource "uds_package" "this" {
│ 
│ Could not find deployed package with name core-secrets; keeping Terraform state because `tolerate_missing_deployed` is set
```