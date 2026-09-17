# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

resource "uds_package" "dos_games" {
  source = "oci://ghcr.io/zarf-dev/packages/dos-games:1.3.0"
}

output "dos_games_source_digest" {
  description = "Immutable package digest resolved from the mutable OCI tag."
  value       = uds_package.dos_games.source_digest
}
