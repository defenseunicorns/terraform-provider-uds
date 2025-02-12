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

resource "uds_package" "podinfo" {
  repository   = "ghcr.io/defenseunicorns/uds-cli/podinfo"
  ref          = "0.0.1"
  architecture = uds_bundle_metadata.example_bundle.architecture
}
