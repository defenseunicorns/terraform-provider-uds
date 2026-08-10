# Zarf Values Example

This example deploys a local Podinfo Zarf package with `uds_package.values` and `uds_package.sensitive_values`.

Run these commands from this example directory:

```sh
uds zarf package create ./podinfo --confirm
tofu init

# The example uses `arm64` by default. To change the architecture, use one
# of the following methods before running `tofu plan` or `tofu apply`:

# - Use an environment variable:
#     export TF_VAR_architecture=amd64
# - Use a tfvars file (create terraform.tfvars) and add the following line:
#     architecture = "amd64"
tofu plan
tofu apply
```

```sh
tofu destroy
```
