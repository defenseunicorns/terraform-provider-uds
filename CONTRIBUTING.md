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

`uds run build` builds the provider from the repository root and generates the
repository-local `.tofurc.dev` configuration used by the OpenTofu tasks. After
mise is activated in your shell, run `uds` commands directly; `mise exec` is
not needed.

## Development loop

Run the checks and local OpenTofu workflow with:

```console
uds run test-unit
uds run dev-plan --set TOFU_DIR=examples/zarf_values
uds run dev-apply --set TOFU_DIR=examples/zarf_values
uds run dev-destroy --set TOFU_DIR=examples/zarf_values
uds run setup-cluster
uds run test-acc
```

Unit tests compile changed packages automatically. The OpenTofu tasks rebuild
the provider as needed and generate the local configuration before planning,
applying, or destroying. `TOFU_DIR` is required for each OpenTofu task. The
examples above use `examples/zarf_values`; set another configuration directory
with `--set TOFU_DIR=path/to/configuration`.

The acceptance test suite requires live-cluster infrastructure. `test-acc` sets
up the cluster and runs the acceptance tests in one workflow. To reuse an
existing cluster across local iterations, run `uds run setup-cluster` once and
then use `uds run test:acc`.

Commits run hk automatically. Other available development tasks include:

```console
uds run lint:check
uds run generate
```
