terraform {
  required_providers {
    uds = {
      source  = "defenseunicorns/uds"
      version = "~> 0.4.1" # x-release-please-version
    }
  }
}

provider "uds" {
  default_architecture = "arm64"
}

locals {
  # renovate: datasource=docker depName=ghcr.io/zarf-dev/packages/init versioning=semver
  zarf_init_version = "v0.82.0"
}

resource "uds_package" "init" {
  source = "oci://ghcr.io/zarf-dev/packages/init:${local.zarf_init_version}"

  signature_verification = {
    keyless = {
      certificate_identity_regexp = "https://github\\.com/zarf-dev/zarf/\\.github/workflows/release\\.yml@refs/tags/v\\d+\\.\\d+\\.\\d+"
      certificate_oidc_issuer     = "https://token.actions.githubusercontent.com"
    }
  }

  # Install optional git-server via alpha optional_components
  optional_components = ["git-server"]
}

# This zarf package example produces sensitive variables using zarf action setVariables sensitive: true.
# The variable used in this example is AUTHSERVICE_REDIS_URI

# zarf package example yaml for reference:

# kind: ZarfPackageConfig
# metadata:
#   name: authservice-ha-deps
#   version: 1.0.0
#
# variables:
#   - name: AUTHSERVICE_REDIS_URI
#     sensitive: true
#
# components:
#   - name: valkey
#     required: true
#     charts:
#       - name: uds-valkey-config
#         namespace: valkey
#         version: 0.1.0
#         localPath: ../chart
#       - name: valkey
#         version: 4.0.2
#         namespace: valkey
#         url: oci://ghcr.io/defenseunicorns/bitferno/valkey
#         valuesFiles:
#           - ../values/values.yaml
#     images:
#       - bitnamilegacy/valkey:8.1.3-debian-12-r3
#       - bitnamilegacy/redis-exporter:1.76.0-debian-12-r0
#       - bitnamilegacy/valkey-sentinel:8.1.3-debian-12-r3
#
#   - name: core-secrets
#     required: true
#     actions:
#       onDeploy:
#         before:
#           - cmd: |
#               uds zarf tools kubectl get secret valkey-valkey \
#                 -n valkey \
#                 -o jsonpath='{.data.valkey-password}' \
#                 | base64 -d
#             mute: true
#             setVariables:
#               - name: VALKEY_PASSWORD
#                 sensitive: true
#
#           - cmd: |
#               echo "redis://:${ZARF_VAR_VALKEY_PASSWORD}@valkey-valkey-primary.valkey.svc.cluster.local:6379"
#             mute: true
#             setVariables:
#               - name: AUTHSERVICE_REDIS_URI
#                 sensitive: true

# resource for the built authservice-ha-deps zarf package example above
resource "uds_package" "authservice_ha_deps" {
  depends_on = [uds_package.init]
  source     = "zarf-package-authservice-ha-deps-arm64-1.0.0.tar.zst"
}

# This package example consumes the variable contained in the authservice-ha-deps package.
# The variable is referenced using the syntax: <package_name>.<set_variables>.<variable_name>,
# which in this example is: uds_package.authservice_ha_deps.set_variables.authservice_redis_uri
resource "uds_package" "core_identity_authorization" {
  depends_on = [uds_package.authservice_ha_deps]
  source     = "oci://ghcr.io/defenseunicorns/packages/uds/core-identity-authorization:${local.uds_version}"

  component {
    name = "authservice"

    override {
      chart_name = "authservice"
      values = [
        { path = "redis.uri", value = uds_package.authservice_ha_deps.set_variables.authservice_redis_uri }
      ]
    }
  }
}

resource "uds_package" "podinfo" {
  depends_on = [uds_package.init]
  source     = "oci://ghcr.io/defenseunicorns/uds-cli/podinfo:0.0.2"
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
        { path = "ui.color", value = "'#663399'" } # Could also use: value = yamlencode("#663399")
      ]
    }
  }
}

resource "uds_package" "dos_games" {
  depends_on = [uds_package.init]
  source     = "oci://ghcr.io/zarf-dev/packages/dos-games:1.3.0"
  namespace  = "demo"

  # This package is signed with a key, but signature verification is disabled here with verify = false.
  # To verify it, set verify = true or remove that attribute, then provide the public key directly
  # or fetch it locally and reference it with the file function as shown below:
  #   curl https://raw.githubusercontent.com/zarf-dev/zarf/refs/heads/main/cosign.pub -o dosgames.pub
  signature_verification = {
    verify = false
    #public_key = file("dosgames.pub")
  }
}
