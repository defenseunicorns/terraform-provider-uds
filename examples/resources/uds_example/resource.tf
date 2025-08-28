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
  description  = "A demo bundle for the init and podinfo packages"
  architecture = "arm64"
}

  resource "uds_package" "init" {
  name         = "init"
  source       = "oci://ghcr.io/zarf-dev/packages/init:v0.60.0"
  architecture = uds_bundle_metadata.example_bundle.architecture

  # Install optional git-server
  component {
    name = "git-server"
  }
}

resource "uds_package" "podinfo" {
  name         = "podinfo"
  source       = "oci://ghcr.io/defenseunicorns/uds-cli/podinfo:0.0.2"
  architecture = uds_package.init.architecture
  depends_on   = [uds_package.init]
  zarf_vars = [
    {
      name = "this_is_a_variable"
      value = "this is the value"
    },
    {
      name = "this_is_another_variable"
      value = "this is another value"
    }
  ] 
}
