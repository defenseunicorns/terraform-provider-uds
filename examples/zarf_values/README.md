# Zarf Values Example

This example deploys a local Podinfo Zarf package with `uds_package.values` and `uds_package.sensitive_values`.

Build the Podinfo package with Zarf values enabled before running OpenTofu:

```sh
uds zarf package create ./podinfo --features="values=true" --confirm --skip-sbom
```

Run the example:

```sh
# If you need to change the architecture to use something other than `arm64` you can use one of 
# the following methods:

# - Pass via CLI:
#     tofu apply -var='architecture=amd64'
# - Use an environment variable:
#     export TF_VAR_architecture=amd64
# - Use a tfvars file (create terraform.tfvars) and add the following line:
#     architecture = "amd64"

tofu init # do not run if using overrides for local development
tofu plan
tofu apply
```

Clean up:

```sh
tofu destroy
```
