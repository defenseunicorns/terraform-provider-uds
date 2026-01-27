provider "uds" {
  default_architecture = "arm64"
}

resource "uds_bundle_metadata" "k3d-core-slim-dev" {
  description = "A UDS bundle for deploying Istio from UDS Core on a development cluster"
  version     = "0.57.0"
}


# === UDS PACKAGES ===
resource "uds_package" "uds_k3d_dev" {
  source = "oci://ghcr.io/defenseunicorns/packages/uds-k3d:${local.uds_k3d_version}"

  vars = [
    { name = "K3D_IMAGE", value = "rancher/k3s:v1.34.2-k3s1" },
    { name = "K3D_EXTRA_ARGS", value = "--api-port 127.0.0.1:6443 --k3s-arg --kube-apiserver-arg=feature-gates=ImageVolume=true@server:0 --k3s-arg --kubelet-arg=feature-gates=ImageVolume=true@server:0" }
  ]

  component {
    name = "uds-dev-stack"
    override {
      chart_name = "minio"
      values = [
        {
          path  = "buckets"
          value = <<EOT
              - name: my-public-bucket
                policy: public
              - name: my-download-only-bucket
                policy: download
              - name: my-private-bucket
                policy: none
            EOT
        },
      ]
    }
  }
}

resource "uds_package" "init" {
  source     = "oci://ghcr.io/zarf-dev/packages/init:v${local.zarf_version}"
  depends_on = [uds_package.uds_k3d_dev]

  # ==> Enable/disable optional components
  component {
    name = "git-server"
  }
}

resource "uds_package" "core_base" {
  source     = "oci://ghcr.io/defenseunicorns/packages/uds/core-base:${local.uds_version}"
  depends_on = [uds_package.init]

  # ==> Patch to add uds-dev-stack to ignored namespaces - leverage tofu built-in functionality
  #provisioner "local-exec" {
  #  command = "kubectl patch deployment pepr-uds-core -n pepr-system --type json -p '[{\"op\": \"replace\", \"path\": \"/spec/template/spec/containers/0/env/6/value\", \"value\": \"zarf, istio-system, uds-dev-stack\"}]'"
  #}

  component {
    name = "pepr-uds-core"
    override {
      chart_name = "module"
      values = [
        { path = "additionalIgnoredNamespaces", value = yamlencode(["uds-dev-stack"]) },
        { path = "watcher.resources.requests.memory", value = local.pepr_watcher_memory_request },
        { path = "admission.resources.requests.memory", value = local.pepr_admission_memory_request },
        { path = "watcher.resources.requests.cpu", value = local.pepr_watcher_cpu_request },
        { path = "admission.resources.requests.cpu", value = local.pepr_admission_cpu_request }
      ]
    }
  }

  component {
    name = "istio-controlplane"
    override {
      chart_name = "istiod"
      values = [
        { path = "resources.requests.memory", value = local.istiod_memory_request },
        { path = "resources.requests.cpu", value = local.istiod_cpu_request },
        { path = "global.proxy.resources.requests.memory", value = local.proxy_memory_request },
        { path = "global.proxy.resources.limits.memory", value = local.proxy_memory_limit },
        { path = "global.proxy.resources.requests.cpu", value = local.proxy_cpu_request },
        { path = "global.proxy.resources.limits.cpu", value = local.proxy_cpu_limit }
      ]
    }
  }

  component {
    name = "istio-admin-gateway"
    override {
      chart_name = "uds-istio-config"
      values = [
        { path = "tls.supportTLSV1_2", value = local.admin_tls1_2_support }
      ]
    }
  }

  component {
    name = "istio-tenant-gateway"
  }
}

resource "uds_package" "core_identity_authorization" {
  source     = "oci://ghcr.io/defenseunicorns/packages/uds/core-identity-authorization:${local.uds_version}"
  depends_on = [uds_package.core_base]

  component {
    name = "authservice"
    override {
      chart_name = "authservice"
      values = [
        { path = "replicaCount", value = local.authservice_replica_count }
      ]
    }
  }

  component {
    name = "keycloak"
    override {
      chart_name = "keycloak"
      values = [
        { path = "resources.requests.memory", value = local.keycloak_memory_request },
        { path = "resources.requests.cpu", value = local.keycloak_cpu_request },
        { path = "resources.limits.memory", value = local.keycloak_memory_limit },
        { path = "resources.limits.cpu", value = local.keycloak_cpu_limit },
        { path = "waypoint.horizontalPodAutoscaler.enabled", value = local.keycloak_waypoint_hpa_enabled },
        { path = "waypoint.deployment.requests.cpu", value = local.keycloak_waypoint_cpu_request },
        { path = "waypoint.deployment.requests.memory", value = local.keycloak_waypoint_memory_request },
        { path = "env", value = local.keycloak_env },
        { path = "realmAuthFlows", value = local.keycloak_realm_auth_flows },
        { path = "themeCustomizations.Settings", value = local.keycloak_theme_customization_settings },
        #{ path = "postgresql.username", value = resource.random_string.keycloak_db_username.result },
        #{ path = "postgresql.password", value = resource.random_password.keycloak_db_password.result },
      ]
      # ==> Secure sensitive values
      sensitive_values = [
        { path = "realmInitEnv", value = local.keycloak_realm_init_env },
      ]
    }
  }
}

resource "uds_package" "podinfo" {
  source     = "oci://ghcr.io/defenseunicorns/uds-cli/podinfo:0.0.2"
  depends_on = [uds_package.init]

  #namespace = "demo" # => Namespace support

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

# ==> Use 3rd party providers
#resource "random_string" "keycloak_db_username" {
#  length  = 16
#  special = false
#  upper   = false
#  numeric = true
#}
#
#resource "random_password" "keycloak_db_password" {
#  length           = 32
#  special          = true
#  override_special = "!#$%&*()-_=+[]{}<>:?"
#  min_special      = 2
#  min_upper        = 2
#  min_lower        = 2
#  min_numeric      = 2
#}
#
#
