#resource "uds_package" "dos_games" {
#  source     = "oci://ghcr.io/zarf-dev/packages/dos-games:1.2.0"
#  depends_on = [uds_package.init]
#
#  namespace = "demo" # => Set/change namespacez
#
#  # Note: skip_signature_validation defaults to false, which allows unsigned packages
#  # Set to true to require all package to be signed
#  skip_signature_validation = false
#  public_key                = file("zarf-dev-cosign.pub") # Fetch from https://raw.githubusercontent.com/zarf-dev/zarf/refs/heads/main/cosign.pub
#}
