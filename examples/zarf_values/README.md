# Zarf Values Example

This example deploys a local Podinfo Zarf package with `uds_package.values` and `uds_package.sensitive_values`.

Build the Podinfo package with Zarf values enabled before running OpenTofu:

```sh
uds zarf package create ./podinfo --features="values=true" --confirm --skip-sbom
```

Run the example:

```sh
tofu init # do not run if using overrides for local development
tofu plan
tofu apply
```

Clean up:

```sh
tofu destroy
```
