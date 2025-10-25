# OpenTofu Provider for UDS (Unicorn Delivery Service)

The UDS (Universal Deployment System) OpenTofu Provider enables declarative infrastructure-as-code capabilities to manage deployment of UDS packages and bundles using OpenTofu.

## Quick Start

1. Configure your `.tofurc` OCI mirror (see [Configure OpenTofu Client OCI Mirror](#configure-opentofu-client-oci-mirror))
2. Create a configuration file (`main.tofu`) referencing the UDS provider and your package(s)
3. Run:

```bash
tofu init
tofu plan
tofu apply
```

## Features

- **Deploy UDS Packages**: Install UDS packages from OCI registries or local file paths (`.tar` or `.tar.zst`)
- **Component Configuration**: Selectively install and configure package components with fine-grained control
- **Helm Chart Overrides**: Customize Helm chart values and sensitive values within components
- **Package Variables**: Set both regular and sensitive variables for packages
- **Multi-Architecture Support**: Deploy packages for `amd64` or `arm64` architectures
- **Registry Flexibility**: Work with public or private OCI registries, including insecure registries
- **Bundle Metadata**: Track and manage UDS Bundle metadata including version and architecture

## Requirements

- [OpenTofu](https://opentofu.org/) >= 1.6
- [Kubernetes](https://kubernetes.io/) cluster with sufficient permissions
- [kubectl](https://kubernetes.io/docs/tasks/tools/) configured to access your cluster
- UDS packages from the following sources:
  - OCI registry reference (e.g., `oci://ghcr.io/zarf-dev/packages/init:v0.63.0`) <!-- renovate: datasource=github-tags depName=zarf-dev/zarf -->
  - Local `.tar` or `.tar.zst` archive file (e.g., `./path/to/package.tar` or `./path/to/package.tar.zst`)

### Provider Registry Authentication/Authorization

In order to pull the UDS provider from the [distribution OCI registries](#provider-registries), you must be authenticated
with and have permissions to pull the provider from the registry.
Please contact the [UDS-CLI team](https://github.com/orgs/defenseunicorns/teams/uds-cli) for assistance.

### Private Package Registry Authentication/Authorization

In order to deploy a UDS package from a private OCI registry source, you must be authenticated with and have permissions
to pull the package from the registry. Please contact the corresponding registry administrator for assistance.

## Using the Provider

### Provider Registries

Currently, released/stable versions and nightly builds of the UDS provider are distributed only through the following OCI registries:

- **GitHub Container Registry (GHCR)** - `ghcr.io/defenseunicorns/opentofu-providers/defenseunicorns/uds` (recommended)
- **UDS Registry** - `registry.defenseunicorns.com/ops/terraform-provider-uds`

> **Note:** In the future, released/stable versions of the UDS provider will be published to the official
> [OpenTofu Public Registry](https://opentofu.org/registry) for typical consumption.
> For now, please use the OCI registry configuration shown above.

### Configure OpenTofu Client OCI Mirror

In your $HOME/.tofurc file, add an OCI mirror configuration specifically for the `defenseunicorns/uds` provider:

```hcl
provider_installation {
  oci_mirror {
    url = "ghcr.io/defenseunicorns/opentofu-providers/defenseunicorns/uds"
    include = ["defenseunicorns/uds"]
  }

  # For all other providers, install them directly from their origin provider registries as normal.
  # This is optional, but recommended.
  direct {}
}
```

### Initialize and Apply Configuration

Once your OpenTofu client is configured with the OCI mirror, you can use the UDS provider just like any other OpenTofu provider.

Example configuration:

<!-- renovate: datasource=github-tags depName=zarf-dev/zarf versioning=semver -->
<!-- renovate: datasource=github-releases depName=defenseunicorns/terraform-provider-uds -->
```hcl
terraform {
  required_providers {
    uds = {
      source  = "defenseunicorns/uds"
      version = "~> 0.1.0"
      # Use the ~> operator for latest stable releases (e.g. ~> 0.1.x), or specify an exact version for nightly builds (e.g. = 0.2.0-nightly)
    }
  }
}

provider "uds" {
  default_architecture = "arm64"
}

resource "uds_package" "init" {
  source = "oci://ghcr.io/zarf-dev/packages/init:v0.63.0"
}

resource "uds_package" "podinfo" {
  source = "oci://ghcr.io/defenseunicorns/uds-cli/podinfo:0.0.2"
}
```

Initialize your workspace to download and install the provider:

```bash
tofu init
```

Plan to preview resource changes:

```bash
tofu plan
```

Apply to create the resources:

```bash
tofu apply
```

## Documentation

For detailed documentation on the provider, resources and their attributes, see:

- **[Provider Configuration](./docs/index.md)** - Configure the UDS provider with architecture defaults and registry options
- **[`uds_package` Resource](./docs/resources/package.md)** - Deploy and manage UDS packages with full schema reference
- **[`uds_bundle_metadata` Resource](./docs/resources/bundle_metadata.md)** - Retrieve metadata from UDS bundles
- **[Examples](./examples)** - Working examples

## Development

### Development Requirements

- [Go](https://golang.org/doc/install) >= 1.21
- [OpenTofu](https://opentofu.org/) >= 1.6
- [golangci-lint](https://golangci-lint.run/usage/install/) for linting
- Access to a Kubernetes cluster for testing

### Configure OpenTofu Client Dev Overrides

Since the UDS provider will be built and installed locally, the OpenTofu client (.tofurc) will need to be configured with a `dev_overrides` to reference the local provider build. The path to the provider build is dependent upon your go configuration. Please refer to the
[Terraform plug-in framework documentation](https://developer.hashicorp.com/terraform/tutorials/providers-plugin-framework/providers-plugin-framework-provider#prepare-terraform-for-local-provider-install) for more information.

Example contents of `$HOME/.tofurc` on macOS:

```hcl
provider_installation {
  dev {
    path = "/Users/username/go/bin"
    include = ["defenseunicorns/uds"]
  }

  # For all other providers, install them directly from their origin provider registries as normal.
  # If you omit this, OpenTofu will _only_ use the dev_overrides block, and so no other providers will be available.
  direct {}
}
```

Refer to the [OpenTofu documentation](https://opentofu.org/docs/cli/config/config-file/#development-overrides-for-provider-developers) for more information.

### Build/Install the Provider Locally

```bash
uds run install
```

### Run Unit Tests

```bash
uds run test-unit
```

### Run Acceptance Tests

```bash
uds run test-acc
```

### Generate Documentation

This provider leverages [terraform-plugin-docs](https://github.com/hashicorp/terraform-plugin-docs) to generate documentation:

```bash
uds run generate
```

## License

This project is licensed under the AGPL-3.0 License. Portions may also be used under the Defense Unicorns Commercial License — see LICENSE for details.

## Support

- **Documentation**: [UDS Documentation](https://uds.defenseunicorns.com/)
- **Issues**: [GitHub Issues](https://github.com/defenseunicorns/terraform-provider-uds/issues)
- **Community**: [UDS Community](https://github.com/defenseunicorns/uds-core/discussions)

## Related Projects

- [UDS Core](https://github.com/defenseunicorns/uds-core) - Core UDS bundle for Kubernetes
- [Zarf](https://zarf.dev/) - DevSecOps tool for airgap Kubernetes deployments
- [UDS CLI](https://github.com/defenseunicorns/uds-cli) - CLI tool for UDS bundle operations
