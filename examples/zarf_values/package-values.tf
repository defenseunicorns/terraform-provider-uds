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

resource "uds_package" "uds_crds" {
  depends_on = [uds_package.init]

  source       = "oci://ghcr.io/defenseunicorns/packages/uds/core-crds:${local.uds_core_crds_version}-${local.uds_package_flavor}"
  architecture = var.architecture
}

resource "uds_package" "nginx" {
  depends_on = [uds_package.init, uds_package.uds_crds]

  source       = "oci://ghcr.io/defenseunicorns/packages/uds/nginx:${local.uds_nginx_version}-${local.uds_package_flavor}"
  architecture = var.architecture

  values = {
    nginx = {
      replicaCount = 3
      podAnnotations = {
        "example.com/source" = "terraform-provider-uds"
      }
    }
  }
}
