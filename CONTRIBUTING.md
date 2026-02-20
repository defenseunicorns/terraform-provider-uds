# Contributing

This guide explains how to build, install, and use the UDS OpenTofu provider locally for development and testing.

## Prerequisites

- [Go](https://golang.org/doc/install) >= 1.21
- [OpenTofu](https://opentofu.org/) >= 1.6
- A Kubernetes cluster (for acceptance tests)
- [`kubectl`](https://kubernetes.io/docs/tasks/tools/) configured to talk to the cluster
- [`golangci-lint`](https://golangci-lint.run/usage/install/) for linting

## Build and install the provider locally

The provider installs into your Go bin directory (typically `$(go env GOPATH)/bin`).

```console
# From the repo root
uds run install
```

## Point OpenTofu at your local build

Since the UDS provider is built and installed locally, you will need to configure a dev override for OpenTofu to load your locally built provider instead of pulling from a registry. You can keep this config "local" to the repo and export `TF_CLI_CONFIG_FILE` to avoid affecting global config. See the [OpenTofu docs](https://opentofu.org/docs/cli/config/config-file/#development-overrides-for-provider-developers) for more details.

Example `.tofurc.dev`:

```hcl
provider_installation {
  # Point OpenTofu at your locally built provider binary
  dev_overrides {
    # This must be a full path to your local binary build
    "defenseunicorns/uds" = "/path/to/your/go/bin"
  }

  # Allow other providers to install normally
  direct {}
}
```

Then to configure OpenTofu to use this dev `.tofurc`, use an environment variable:

```console
# Tell OpenTofu to use this config for the current shell (use an absolute path)
export TF_CLI_CONFIG_FILE="$HOME/.tofurc.dev"
```

## Use the provider in tofu

To use the provider in your tofu files, simply add it to `required_providers`, but omit the `version`:

```hcl
terraform {
  required_providers {
    uds = {
      source  = "defenseunicorns/uds"
      # With dev_overrides, omit the version to avoid registry resolution
    }
  }
}

provider "uds" {
  default_architecture = "arm64"
}

resource "uds_package" "init" {
  source = "oci://ghcr.io/zarf-dev/packages/init:v0.72.0"
}
```

Then run standard plan/apply:

```console
# Generally with `dev_overrides` you should skip `tofu init` as it may error unexpectedly
tofu plan
tofu apply
```

## Lint, test, and docs

Other available dev tasks:

```console
uds run lint         # golangci-lint
uds run test-unit    # unit tests
uds run test-acc     # acceptance tests
uds run generate     # regenerate provider docs
```

## Tips

- To switch back to registry builds, unset `TF_CLI_CONFIG_FILE` or remove the `dev` block from your CLI config.
- Verify which binary OpenTofu is picking up with `tofu providers mirror` or by inspecting `.terraform/plugins` after `tofu init`.
