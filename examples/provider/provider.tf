# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

terraform {
  required_providers {
    uds = {
      source = "defenseunicorns/uds"
    }
  }
}

provider "uds" {
  # Override the architecture detected from the local machine.
  # default_architecture = "amd64"

  # Use a custom directory for downloaded packages.
  # zarf_cache_path = "~/.zarf-cache"

  # Disable package loading and validation during planning.
  # validate_packages_on_plan = false

  # Force Helm to take ownership of conflicting Server-Side Apply fields.
  # force_helm_ssa_conflicts = true
}
