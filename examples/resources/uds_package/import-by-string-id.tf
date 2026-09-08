# Copyright 2026 Defense Unicorns
# SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial


# Prerequisites:
# 1. Fetch the public signing key for the dos-games package:
#    curl https://raw.githubusercontent.com/zarf-dev/zarf/refs/heads/main/cosign.pub -o dosgames.pub
# 2. Deploy the init and dos-games packages outside Terraform or OpenTofu using
#    the sources configured below. Deploy dos-games in the demo namespace and
#    verify it with dosgames.pub.

# Without a namespace override, the import ID is the package name:
# <package-name>
import {
  to = uds_package.init
  id = "init"
}

resource "uds_package" "init" {
  source = "oci://ghcr.io/zarf-dev/packages/init:v0.85.0"
}

# With a namespace override, the import ID is the namespace and package name:
# <namespace>:<package-name>
import {
  to = uds_package.demo_dos_games
  id = "demo:dos-games"
}

resource "uds_package" "demo_dos_games" {
  depends_on = [uds_package.init]
  source     = "oci://ghcr.io/zarf-dev/packages/dos-games:1.3.0"
  namespace  = "demo"

  signature_verification = {
    public_key = file("dosgames.pub")
  }
}
