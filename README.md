# OpenTofu Provider for UDS (Unified Defense Stack)

Manage [UDS](https://docs.defenseunicorns.com/) resources with OpenTofu. Currently, the provider supports deploying and managing [UDS packages](https://docs.defenseunicorns.com/core/how-to-guides/packaging-applications/overview/).

> [!WARNING]
> This provider is an alpha release. Its interfaces and behavior may change between
> releases, including breaking changes.

## Installation

Provider releases are available from the
[OpenTofu Registry](https://search.opentofu.org/provider/defenseunicorns/uds/latest)
and [Terraform Registry](https://registry.terraform.io/providers/defenseunicorns/uds/latest).

## Usage

```hcl
terraform {
  required_providers {
    uds = {
      source  = "defenseunicorns/uds"
      version = "~> 0.5.3" # x-release-please-version
    }
  }
}

resource "uds_package" "init" {
  source = "oci://ghcr.io/zarf-dev/packages/init:v0.85.0"

  # init is keyless-signed. This example skips verification for brevity.
  signature_verification = {
    verify = false
  }
}

resource "uds_package" "dos_games" {
  depends_on = [uds_package.init]
  source     = "oci://ghcr.io/zarf-dev/packages/dos-games:1.3.0"
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

### Logging

Provider and Zarf output is emitted through OpenTofu's provider log stream rather than written directly to stdout. The following settings control that provider/OpenTofu log stream only:

- Set `TF_LOG=INFO` for lifecycle messages and non-muted Zarf command output.
- Set `TF_LOG=DEBUG` for detailed Zarf diagnostics.
- Set `TF_LOG_PROVIDER=INFO` or `TF_LOG_PROVIDER=DEBUG` to filter provider logs.
- Set `TF_LOG_PATH=/absolute/path` to persist logs.

Failed non-muted Zarf output is included in the normal Terraform diagnostic regardless of these log settings. Successful Zarf command output remains log-only, while muted command output remains suppressed. Raw successful Zarf command output is still subject to the logging system's filtering and persistence settings.

## Documentation

- [Provider configuration](./docs/index.md)
- [`uds_package` resource](./docs/resources/package.md)
- [Signed package examples](./examples/resources/uds_package/resource.tf)

## Development

See [CONTRIBUTING.md](./CONTRIBUTING.md).

## License

See [LICENSE](./LICENSE).
