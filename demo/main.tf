# # Copyright 2024 Defense Unicorns
# # SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial
terraform {
  required_providers {
    uds = {
      source  = "defenseunicorns/uds"
      version = "0.0.1"
    }
  }
}

provider "uds" {}

locals {
  architecture     = "arm64"
  zarf_version     = "0.61.0"
  podinfo_version  = "0.0.2"
  dosgames_version = "1.2.0"
}

resource "uds_package" "uds-k3d-dev" {
  source       = "oci://ghcr.io/defenseunicorns/packages/uds-k3d:0.13.0"
  architecture = local.architecture
}

resource "uds_package" "init" {
  source       = "oci://ghcr.io/zarf-dev/packages/init:v${local.zarf_version}"
  architecture = local.architecture
  depends_on   = [uds_package.uds-k3d-dev]

  # Uncomment the component block below to install the git-server optional component
  #component {
  #  name = "git-server"
  #}
}

# Requires podinfo UDS package locally in the current directory. Update the podinfo_version to match.
resource "uds_package" "podinfo" {
  source = "oci://ghcr.io/defenseunicorns/uds-cli/podinfo:${local.podinfo_version}"
  # Optionally, pull/download the package locally and set source to the path to the pulled package tar file.
  #source       = "zarf-package-podinfo-dev-${local.architecture}-v${local.podinfo_version}.tar.zst"
  architecture = local.architecture
  depends_on   = [uds_package.init]

  component {
    name = "podinfo"

    override {
      chart_name = "podinfo"
      values = [
        { path = "replicaCount", value = "3" }
      ]
    }
  }
}

resource "uds_package" "demo_dos_games" {
  source       = "oci://ghcr.io/zarf-dev/packages/dos-games:${local.dosgames_version}"
  architecture = local.architecture
  namespace    = "demo"
  depends_on   = [uds_package.init]

  # Skip signature validation if public key is not available. Otherwise set to false (or remove this attribute) and provide public key.
  skip_signature_validation = true

  # dos-games public key: curl https://raw.githubusercontent.com/zarf-dev/zarf/refs/heads/main/cosign.pub -o dosgames.pub
  #public_key                = file("dosgames.pub") // Can be set explicitly to key value, or use file function to reference local file
}

output "init_package" {
  value = uds_package.init
}
