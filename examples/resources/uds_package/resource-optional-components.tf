resource "uds_package" "init_with_git_server" {
  source              = "oci://ghcr.io/zarf-dev/packages/init:v0.82.0"
  optional_components = ["git-server"]

  signature_verification = {
    keyless = {
      certificate_identity_regexp = "https://github\\.com/zarf-dev/zarf/\\.github/workflows/release\\.yml@refs/tags/v\\d+\\.\\d+\\.\\d+"
      certificate_oidc_issuer     = "https://token.actions.githubusercontent.com"
    }
  }
}
