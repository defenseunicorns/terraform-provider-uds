# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

terraform {
  required_providers {
    uds = {
      source  = "defenseunicorns/uds"
      version = "~> 0.5.1" # x-release-please-version
    }
  }
}

variable "architecture" {
  description = "Architecture of the created podinfo zarf package and environment to deploy into (i.e. amd64 or arm64)"
  type        = string
  default     = "arm64"
}

locals {
  change_me          = "initial-value"
  uds_package_flavor = "upstream"

  # renovate: datasource=docker depName=ghcr.io/zarf-dev/packages/init versioning=semver
  zarf_init_version = "v0.83.0"

  # renovate: datasource=docker depName=ghcr.io/defenseunicorns/packages/uds/core-crds versioning=semver extractVersion=^(?<version>.*)-upstream$
  uds_core_crds_version = "1.11.1"

  # renovate: datasource=docker depName=ghcr.io/defenseunicorns/packages/uds/nginx versioning=semver extractVersion=^(?<version>.*)-upstream$
  uds_nginx_version = "1.31.1-uds.1"
}

provider "uds" {
  default_architecture = var.architecture
}

resource "terraform_data" "dynamic_test_data" {
  input = {
    dynamic_value = local.change_me
  }
}

resource "terraform_data" "dynamic_ui" {
  input = {
    color = "#663399"
  }
}

resource "uds_package" "init" {
  source       = "oci://ghcr.io/zarf-dev/packages/init:${local.zarf_init_version}"
  architecture = var.architecture

  signature_verification = {
    keyless = {
      certificate_identity_regexp = "https://github\\.com/zarf-dev/zarf/\\.github/workflows/release\\.yml@refs/tags/v\\d+\\.\\d+\\.\\d+"
      certificate_oidc_issuer     = "https://token.actions.githubusercontent.com"
    }
  }
}
