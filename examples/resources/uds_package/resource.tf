terraform {
  required_providers {
    uds = {
      source  = "defenseunicorns/uds"
      version = "~> 0.1.7"
    }
  }
}

provider "uds" {
  default_architecture = "arm64"
}

resource "uds_bundle_metadata" "example_bundle" {
  version      = "0.0.1"
  kind         = "UDSBundle"
  description  = "A demo bundle for the init and podinfo packages"
  architecture = "arm64"
}

resource "uds_package" "init" {
  source = "oci://ghcr.io/zarf-dev/packages/init:v0.73.0"

  # Install optional git-server
  component {
    name = "git-server"
  }
}

# The `uds_package` resource now automatically persists runtime set variables produced by a package deployment (via Zarf actions that call `setVariable`) into two computed maps on the resource:
# - `set_variables` (Read-Only, Map(String)): non-sensitive runtime variables written by the package at deploy time.
# - `sensitive_set_variables` (Read-Only, Map(String), Sensitive): sensitive runtime variables written by the package at deploy time.

# This package example produces sensitive variables using zarf action setVariables sensitive: true. 
# The variable is AUTHSERVICE_REDIS_URI in this example
resource "uds_package" "authservice-ha-deps" {
  depends_on = [uds_package.init]
  source     = "../build/authservice-ha-deps/*.zst"
}

# This package example consumes the sensitive variable produced by the authservice-ha-deps package. 
# The variable is referenced using the syntax: <package_name>.<sensitive_set_vars>.<variable_name>, 
# which in this example is: uds_package.authservice-ha-deps.sensitive_set_vars.authservice_redis_uri
resource "uds_package" "core-identity-authorization" {
  depends_on = [uds_package.authservice-ha-deps]
  source     = "oci://ghcr.io/defenseunicorns/packages/uds/core-identity-authorization:${local.uds_version}"

  component {
    name = "authservice"

    override {
      chart_name = "authservice"
      values = [
        { path = "redis.uri", value = uds_package.authservice-ha-deps.sensitive_set_vars.authservice_redis_uri }
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
  source     = "oci://ghcr.io/zarf-dev/packages/dos-games:1.2.0"
  namespace  = "demo"

  # Do not verify package signature if public key is not available. Otherwise set to true (or remove this attribute) and provide public key.
  verify_signature = false

  # public_key can be set explicitly to the key value, or use file function to reference local file:
  #    curl https://raw.githubusercontent.com/zarf-dev/zarf/refs/heads/main/cosign.pub -o dosgames.pub
  #public_key                = file("dosgames.pub") 
}
