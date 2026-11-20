# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

# Alternative to the source-mode examples: omit source and signature_verification.
resource "uds_package" "local" {
  build = {
    source_dir = "${path.module}/package"
    revision   = "manual-1" # Required; change this to request redeployment.
  }
}
