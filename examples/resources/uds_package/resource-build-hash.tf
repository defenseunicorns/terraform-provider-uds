# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

locals {
  package_dir          = "${path.module}/package"
  package_architecture = "amd64"
  package_files        = sort(tolist(fileset(local.package_dir, "**")))
  package_revision = sha256(jsonencode({
    files = {
      for file in local.package_files :
      file => filesha256("${local.package_dir}/${file}")
    }
    architecture = local.package_architecture
  }))
}

# Alternative to a manual revision. This hash covers only the listed inputs;
# include any other inputs that should request deployment in your own expression.
resource "uds_package" "hashed" {
  architecture = local.package_architecture
  build = {
    source_dir    = local.package_dir
    revision      = local.package_revision
    build_on_plan = true
  }
}
