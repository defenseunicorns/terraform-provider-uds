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
uds run provider:dev
```

`provider:dev` builds the provider and generates the repository-local
`.tofurc.dev` configuration used by the OpenTofu tasks. After mise is activated
in your shell, run `uds` commands directly; `mise exec` is not needed.

## Development loop

Run the checks and local OpenTofu workflow with:

```console
uds run test-unit
uds run tofu:plan
uds run tofu:apply
uds run tofu:destroy
uds run setup-cluster
uds run e2e
```

Unit tests compile changed packages automatically. The OpenTofu tasks rebuild
the provider as needed and generate the local configuration before planning,
applying, or destroying. They use `examples/zarf_values` by default; set
`TOFU_DIR` with the UDS task runner when working with another configuration.
The e2e suite requires live-cluster infrastructure. Run `uds run setup-cluster`
once and reuse that cluster across local iterations with `uds run test:e2e`.
Use `uds run e2e` when you want the self-contained setup-and-test workflow.

Commits run hk automatically. Other available development tasks include:

```console
uds run lint:check
uds run generate
```
