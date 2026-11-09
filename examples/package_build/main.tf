# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

terraform {
  required_providers {
    uds = {
      source = "defenseunicorns/uds"
    }
  }
}

variable "architecture" {
  description = "Target cluster architecture: arm64 or amd64."
  type        = string
  default     = "arm64"
}

variable "revision" {
  description = "Manual deployment trigger; change this to rebuild/redeploy without changing the package version."
  type        = string
  default     = "manual-1"
}

variable "build_on_plan" {
  description = "Also build and validate during every normal plan."
  type        = bool
  default     = false
}

variable "replica_count" {
  description = "Changing a deployment value also requests a fresh build."
  type        = number
  default     = 2
}

provider "uds" {
  default_architecture = var.architecture
}

resource "uds_package" "init" {
  source       = "oci://ghcr.io/zarf-dev/packages/init:v0.89.0"
  architecture = var.architecture

  signature_verification = {
    keyless = {
      certificate_identity_regexp = "https://github\\.com/zarf-dev/zarf/\\.github/workflows/release\\.yml@refs/tags/v\\d+\\.\\d+\\.\\d+"
      certificate_oidc_issuer     = "https://token.actions.githubusercontent.com"
    }
  }
}

resource "uds_package" "podinfo" {
  depends_on   = [uds_package.init]
  architecture = var.architecture
  namespace    = "podinfo"

  build = {
    source_dir    = "${path.module}/podinfo"
    revision      = var.revision
    build_on_plan = var.build_on_plan
  }

  # These remain deployment inputs, not build-time templating inputs.
  values = {
    logLevel = "debug"
    service = {
      enabled = true
    }
    pod = {
      replicaCount = var.replica_count
      annotations = {
        "example.com/source" = "terraform-provider-uds-build"
      }
    }
    ui = {
      color = "#663399"
    }
  }

  sensitive_values = {
    pod = {
      annotations = {
        "example.com/sensitive-note" = "redacted-from-terraform-output"
      }
    }
  }
}
