provider "uds" {
  # Set a default architecture for packages if not set explicitly in the resource
  default_architecture = "arm64"

  # Enable HTTP-only connections when working with non-TLS registries
  insecure_force_http = true

  # Skip TLS verification when using custom or self-signed certificates
  insecure_skip_tls_verification = true

  # Use a custom Zarf cache directory for package downloads and verification
  zarf_cache_path = "~/.zarf-cache"

  # Force Helm to take ownership of conflicting Server-Side Apply fields
  force_helm_ssa_conflicts = false

  # Validate package-dependent configuration during plan
  validate_packages_on_plan = true
}
