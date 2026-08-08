# Contributing

This guide explains how to develop and test the UDS OpenTofu provider locally.

## Prerequisites

- Install [mise](https://mise.jdx.dev/getting-started.html).
- Activate mise in your shell once, following the [shell activation instructions](https://mise.jdx.dev/dev-tools/#activate).

The repository's `mise.toml` installs Go, OpenTofu, the UDS CLI, k3d, and the
other development tools. You do not need to install those tools individually.

## Set up the repository

From the repository root, run:

```console
mise install
hk install
uds run build
```

`uds run build` builds the provider from the repository root and generates the repository-local `.tofurc.dev` configuration. Mise sets `TF_CLI_CONFIG_FILE` to this file automatically throughout the repository, so OpenTofu will use the local provider build from any subdirectory of this repository. After mise is activated in your shell, run `uds` and `tofu` commands directly; `mise exec` is not needed.

## Development loop

Run the unit and acceptance tests with:

```console
uds run test-unit
uds run test-acc
```

> [!NOTE]
> The acceptance test suite requires live-cluster infrastructure. The `test-acc` task automatically creates a fresh k3d cluster prior to running acceptance tests and then leaves it running afterward to aid with troubleshooting if failures occur. This cluster can also be used for subsequent manual testing, such as running the example below.

> [!TIP]
> To rerun acceptance tests against the cluster left running by `test-acc`, use
> `uds run test:acc`. Unlike `test-acc`, this task does not recreate the cluster.

Run OpenTofu directly from a configuration directory:

```console
cd examples/zarf_values
uds zarf package create ./podinfo --confirm
tofu plan
tofu apply
tofu destroy
```

> [!IMPORTANT]
> After making provider code changes, you must rebuild the provider executable by running `uds run build` from the repository root prior to testing with OpenTofu.

> [!TIP]
> Alternatively, the `dev-plan`, `dev-apply`, and `dev-destroy` tasks are available as conveniences that rebuild the provider before running common OpenTofu operations; each requires `TOFU_DIR` to be set to the directory containing the OpenTofu code you'd like to test, for example:

```console
uds run dev-plan --set TOFU_DIR=examples/zarf_values
```

Commits run hk automatically. Other available development tasks include:

```console
uds run lint:check
uds run generate
```
