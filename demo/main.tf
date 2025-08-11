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
  architecture = "arm64"
}

resource "uds_package" "uds-k3d-dev" {
  name         = "uds_k3d"
  repository   = "ghcr.io/defenseunicorns/packages/uds-k3d"
  ref          = "0.13.0"
  architecture = local.architecture
}

resource "uds_package" "init" {
  depends_on   = [uds_package.uds-k3d-dev]
  name         = "init"
  repository   = "ghcr.io/zarf-dev/packages/init"
  ref          = "v0.59.0"
  architecture = local.architecture

  # Uncomment the component block below to install the git-server optional component
  #component {
  #  name = "git-server"
  #}
}
