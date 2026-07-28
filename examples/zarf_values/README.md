# Zarf Values Example

This example deploys a local Podinfo Zarf package with `uds_package.values` and `uds_package.sensitive_values`.

Build the Podinfo package with Zarf values enabled before running OpenTofu from the repository root:

```sh
uds zarf package create ./examples/zarf_values/podinfo --features="values=true" --confirm --skip-sbom
```

Run the example with the repository's generated local provider configuration:

```sh
# From the repository root:
uds run tofu:plan
uds run tofu:apply

# The configured example workflow uses `arm64` by default. To change the
# architecture, use one of the following methods before running the workflow:

# - Use an environment variable:
#     export TF_VAR_architecture=amd64
# - Use a tfvars file (create terraform.tfvars) and add the following line:
#     architecture = "amd64"
```

The e2e suite requires live-cluster infrastructure. From the repository root,
run `uds run setup-cluster` once and reuse the cluster across local iterations,
then run `uds run e2e`.

Clean up:

```sh
uds run tofu:destroy
```
