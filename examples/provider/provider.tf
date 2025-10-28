provider "uds" {
  # Set a default architecture for packages if not set explicitly in the resource
  default_architecture = "arm64"

  # Enable HTTP-only connections when working with non-TLS registries
  insecure_force_http = true

  # Skip TLS verification when using custom or self-signed certificates
  insecure_skip_tls_verification = true
}
