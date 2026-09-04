# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

resource "uds_package" "fleet" {
  source = "oci://ghcr.io/defenseunicorns/uds-fleet-command/dev:dev"
}

output "fleet_source_digest" {
  description = "Immutable package digest resolved from the mutable OCI tag."
  value       = uds_package.fleet.source_digest
}
