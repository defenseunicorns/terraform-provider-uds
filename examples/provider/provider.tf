provider "uds" {
  # Set a default architecture for packages if not set explicitly in the resource
  default_architecture = "arm64"

  # Allow HTTP-only OCI package sources or force HTTP for an external Zarf registry
  # insecure_force_http = true

  # Skip TLS verification for package sources or external registries with self-signed certificates
  # insecure_skip_tls_verification = true

  # Use a custom Zarf cache directory for package downloads and verification
  zarf_cache_path = "~/.zarf-cache"

  # Force Helm to take ownership of conflicting Server-Side Apply fields
  force_helm_ssa_conflicts = false

  # Validate package-dependent configuration during plan
  validate_packages_on_plan = true
}
