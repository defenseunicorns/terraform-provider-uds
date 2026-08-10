# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

resource "uds_package" "init" {
  source = "oci://ghcr.io/zarf-dev/packages/init:v0.83.0"

  signature_verification = {
    keyless = {
      certificate_identity_regexp = "https://github\\.com/zarf-dev/zarf/\\.github/workflows/release\\.yml@refs/tags/v\\d+\\.\\d+\\.\\d+"
      certificate_oidc_issuer     = "https://token.actions.githubusercontent.com"
    }
  }
}

resource "uds_package" "dos_games" {
  depends_on = [uds_package.init]
  source     = "oci://ghcr.io/zarf-dev/packages/dos-games:1.3.0"
  namespace  = "demo"

  # Signature verification is disabled for brevity.
  signature_verification = {
    verify = false
  }

  # To verify this package instead, download its public key:
  # curl --fail --location --output dosgames.pub \
  #   https://raw.githubusercontent.com/zarf-dev/zarf/refs/heads/main/cosign.pub
  # Then replace the signature_verification block above with:
  # signature_verification = {
  #   public_key = file("dosgames.pub")
  # }
}
