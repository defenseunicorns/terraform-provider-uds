# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

terraform {
  required_providers {
    uds = {
      source  = "defenseunicorns/uds"
      version = "~> 0.4.0"
    }
  }
}

variable "architecture" {
  description = "Architecture of the created podinfo zarf package and environment to deploy into (i.e. amd64 or arm64)"
  type        = string
  default     = "arm64"
}

locals {
  change_me = "initial-value"
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
  source       = "oci://ghcr.io/zarf-dev/packages/init:v0.78.0"
  architecture = var.architecture

  signature_verification = {
    keyless = {
      certificate_identity_regexp = "https://github\\.com/zarf-dev/zarf/\\.github/workflows/release\\.yml@refs/tags/v\\d+\\.\\d+\\.\\d+"
      certificate_oidc_issuer     = "https://token.actions.githubusercontent.com"
    }
  }
}

resource "uds_package" "podinfo" {
  depends_on = [uds_package.init]

  source       = "./zarf-package-podinfo-${var.architecture}-0.1.0.tar.zst"
  architecture = var.architecture
  namespace    = "podinfo"

  values = {
    logLevel = "debug"
    service = {
      enabled = true
    }
    pod = {
      replicaCount = 3
      annotations = {
        "example.com/source"           = "terraform-provider-uds"
        "example.com/change-me"        = local.change_me
        "example.com/dynamic-value-id" = terraform_data.dynamic_test_data.id
        "example.com/dynamic-value"    = terraform_data.dynamic_test_data.output.dynamic_value
      }
      tolerations = [
        {
          key      = "example-key"
          operator = "Exists"
          effect   = "NoSchedule"
        }
      ]
    }
    ui = terraform_data.dynamic_ui.output
  }

  sensitive_values = {
    pod = {
      annotations = {
        "example.com/sensitive-changed-value" = local.change_me
        "example.com/sensitive-dynamic-value" = terraform_data.dynamic_test_data.id
        "example.com/sensitive-note"          = "redacted-from-terraform-output"
      }
    }
  }
}
