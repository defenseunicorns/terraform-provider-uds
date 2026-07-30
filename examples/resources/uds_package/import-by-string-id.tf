
# PRE-REQUISITES:
# 1. Fetch public signing key for dos-games package:
#    - curl https://raw.githubusercontent.com/zarf-dev/zarf/refs/heads/main/cosign.pub -o dosgames.pub
# 2. Deploy podinfo and dos-games (with namespace) packages
#    - zarf package deploy oci://ghcr.io/zarf-dev/packages/init:v0.82.0 --confirm
#    - zarf package deploy oci://ghcr.io/zarf-dev/packages/dos-games:1.3.0 --key dosgames.pub --verify -n demo --confirm
# package to import without namespace override
# zarf package deploy oci://ghcr.io/defenseunicorns/uds-cli/podinfo:0.0.2 --confirm

# import package deployed without a namespace override
import {
  to = uds_package.init
  id = "init"
}

resource "uds_package" "init" {
  source = "oci://ghcr.io/zarf-dev/packages/init:v0.82.0"
}

# import package deployed with a namespace override
import {
  to = uds_package.demo_dos_games
  id = "demo:dos-games"
}

resource "uds_package" "demo_dos_games" {
  depends_on = [uds_package.init]
  source     = "oci://ghcr.io/zarf-dev/packages/dos-games:1.3.0"
  namespace  = "demo"
  public_key = file("dosgames.pub") # See PRE-REQUISITES above
}
