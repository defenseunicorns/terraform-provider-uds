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
}
