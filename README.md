# OpenTofu Provider for UDS (Unified Defense Stack)

Manage [UDS](https://docs.defenseunicorns.com/) resources with OpenTofu. Currently, the provider supports deploying and managing [UDS packages](https://docs.defenseunicorns.com/core/how-to-guides/packaging-applications/overview/).

## Installation

> Provider releases are currently distributed as OCI artifacts. Publication to the
> [OpenTofu Registry](https://search.opentofu.org/) and
> [Terraform Registry](https://registry.terraform.io/) is planned.

Until then, add the following mirror to your [OpenTofu CLI configuration](https://opentofu.org/docs/cli/config/config-file/):

```hcl
provider_installation {
  oci_mirror {
    repository_template = "ghcr.io/defenseunicorns/opentofu-providers/defenseunicorns/uds"
    include             = ["defenseunicorns/uds"]
  }

  direct {}
}
```

## Usage

```hcl
terraform {
  required_providers {
    uds = {
      source  = "defenseunicorns/uds"
      version = "~> 0.4.0" # x-release-please-version
    }
  }
}

resource "uds_package" "init" {
  source = "oci://ghcr.io/zarf-dev/packages/init:v0.63.0"

  # init is keyless-signed. This example skips verification for brevity.
  signature_verification = {
    verify = false
  }
}

resource "uds_package" "dos_games" {
  depends_on = [uds_package.init]
  source     = "oci://ghcr.io/zarf-dev/packages/dos-games:1.2.0"
  namespace  = "demo"

  # dos-games is key-signed. This example skips verification for brevity.
  signature_verification = {
    verify = false
  }
}
```

```bash
tofu init
tofu plan
tofu apply
```

## Documentation

- [Provider configuration](./docs/index.md)
- [`uds_package` resource](./docs/resources/package.md)
- [Signed package examples](./examples/resources/uds_package/resource.tf)

## Development

See [CONTRIBUTING.md](./CONTRIBUTING.md).

## License

See [LICENSE](./LICENSE).
