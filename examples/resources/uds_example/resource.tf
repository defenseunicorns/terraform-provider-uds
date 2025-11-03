terraform {
  required_providers {
    uds = {
      source  = "defenseunicorns/uds"
      version = "~> 0.1.0" # renovate: datasource=github-releases depName=defenseunicorns/terraform-provider-uds
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
  # renovate: datasource=github-tags depName=zarf-dev/zarf
  source       = "oci://ghcr.io/zarf-dev/packages/init:v0.63.0"
  architecture = uds_bundle_metadata.example_bundle.architecture

  # Install optional git-server
  component {
    name = "git-server"
  }
}

resource "uds_package" "podinfo" {
  source       = "oci://ghcr.io/defenseunicorns/uds-cli/podinfo:0.0.2"
  architecture = uds_package.init.architecture
  depends_on   = [uds_package.init]
  vars = [
    {
      name  = "this_is_a_variable"
      value = "this is the value"
    },
    {
      name  = "this_is_another_variable"
      value = "this is another value"
    }
  ]
  sensitive_vars = [
    {
      name  = "this_is_a_sensitive_variable"
      value = "this is the value"
    },
    {
      name  = "this_is_another_sensitive_variable"
      value = "this is another value"
    }
  ]

  component {
    name = "podinfo"

    override {
      chart_name = "podinfo"
      values = [
        { path = "replicaCount", value = "3" },
        # Use single quotes or yamlencode function in single-line strings to escape special HCL characters ("#").
        { path = "ui.color", value = "'#663399'" }
      ]
    }
  }
}

resource "uds_package" "dos_games" {
  source       = "oci://ghcr.io/zarf-dev/packages/dos-games:1.2.0"
  architecture = uds_package.init.architecture
  namespace    = "demo"
  depends_on   = [uds_package.init]

  # Skip signature validation if public key is not available. Otherwise set to false (or remove this attribute) and provide public key.
  skip_signature_validation = true

  # public_key can be set explicitly to the key value, or use file function to reference local file:
  #    curl https://raw.githubusercontent.com/zarf-dev/zarf/refs/heads/main/cosign.pub -o dosgames.pub
  #public_key                = file("dosgames.pub") 
}
