terraform {
  required_providers {
    uds = {
      source  = "defenseunicorns/uds"
      version = "0.0.1"
    }
  }
}

provider "uds" {}

resource "uds_bundle_metadata" "example_bundle" {
  version      = "0.0.1"
  kind         = "UDSBundle"
  description  = "A demo bundle for the podinfo and nginx packages"
  architecture = "arm64"
}

resource "uds_package" "init" {
  repository   = "oci://ghcr.io/zarf-dev/packages/init:v0.48.0"
  ref          = "v0.48.0"
  architecture = uds_bundle_metadata.example_bundle.architecture
}

resource "uds_package" "podinfo" {
  repository   = "oci://ghcr.io/defenseunicorns/uds-cli/podinfo:0.0.2"
  ref          = "0.0.2"
  architecture = uds_package.init.architecture
}
