terraform {
  required_providers {
    uds = {
      source  = "defenseunicorns/uds"
      version = "~> 0.1.4"
    }
    #random = {
    #  source  = "hashicorp/random"
    #  version = "~> 3.6"
    #}
  }
}

# ==============================================================================
# Global Configuration
# ==============================================================================
locals {
  flavor = "upstream"
  domain = "uds.dev"

  zarf_version    = "0.66.0"
  uds_version     = "0.57.0-${local.flavor}" # ==> Demo upgrade: 0.57.0 => 0.59.1
  uds_k3d_version = "0.19.4-airgap"
  podinfo_version = "6.9.4"
}

# ==============================================================================
# UDS Package Configuration
# ==============================================================================
locals {
  # == core-base ==
  pepr_watcher_memory_request   = "64Mi"
  pepr_admission_memory_request = "64Mi"
  pepr_watcher_cpu_request      = "100m"
  pepr_admission_cpu_request    = "100m"
  istiod_memory_request         = "1024Mi"
  istiod_cpu_request            = "100m"
  proxy_memory_request          = "40Mi"
  proxy_memory_limit            = "1024Mi"
  proxy_cpu_request             = "10m"
  proxy_cpu_limit               = "2000m"
  admin_tls_cert                = null
  admin_tls_key                 = null
  admin_tls1_2_support          = false
  tenant_tls_cert               = null
  tenant_tls_key                = null
  tenant_tls1_2_support         = false
  tenant_service_ports          = []
  # == core-identity ==
  authservice_replica_count        = 1
  keycloak_memory_request          = "512Mi"
  keycloak_cpu_request             = "100m"
  keycloak_memory_limit            = "1Gi"
  keycloak_cpu_limit               = "1000m"
  keycloak_waypoint_hpa_enabled    = false
  keycloak_waypoint_cpu_request    = "100m"
  keycloak_waypoint_memory_request = "128Mi"
  keycloak_env = yamlencode([
    {
      name  = "JAVA_OPTS_KC_HEAP"
      value = "-XX:MaxRAMPercentage=70 -XX:MinRAMPercentage=70 -XX:InitialRAMPercentage=50 -XX:MaxRAM=1G"
    }
  ])
  keycloak_insecure_admin_password_generation = null
  keycloak_ha                                 = null
  keycloak_pg_username                        = null
  keycloak_pg_password                        = null
  keycloak_pg_database                        = null
  keycloak_pg_host                            = null
  keycloak_devmode                            = null
  keycloak_heap_options                       = "-XX:MaxRAMPercentage=70 -XX:MinRAMPercentage=70 -XX:InitialRAMPercentage=50 -XX:MaxRAM=1G"
  keycloak_realm_init_env = yamlencode({
    OPENTOFU_CLIENT_ENABLED      = true
    GOOGLE_IDP_ENABLED           = true
    GOOGLE_IDP_ID                = "C01881u7t"
    GOOGLE_IDP_SIGNING_CERT      = "MIIDdDCCAlygAwIBAgIGAXkza8/+MA0GCSqGSIb3DQEBCwUAMHsxFDASBgNVBAoTC0dvb2dsZSBJbmMuMRYwFAYDVQQHEw1Nb3VudGFpbiBWaWV3MQ8wDQYDVQQDEwZHb29nbGUxGDAWBgNVBAsTD0dvb2dsZSBGb3IgV29yazELMAkGA1UEBhMCVVMxEzARBgNVBAgTCkNhbGlmb3JuaWEwHhcNMjEwNTAzMTgwOTMzWhcNMjYwNTAyMTgwOTMzWjB7MRQwEgYDVQQKEwtHb29nbGUgSW5jLjEWMBQGA1UEBxMNTW91bnRhaW4gVmlldzEPMA0GA1UEAxMGR29vZ2xlMRgwFgYDVQQLEw9Hb29nbGUgRm9yIFdvcmsxCzAJBgNVBAYTAlVTMRMwEQYDVQQIEwpDYWxpZm9ybmlhMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAu9en1CO4EriCJ5jzss6TqUmtYMXXRBfsSkdnhVvMx0fYOegxy0d8DouUEEITlPW+YPBG1T72kiV9KGtKVw90ff4Y+siNDNrME81w4K3Zjo6VukvATfD05lVzh9JyO0VxdzBpdRXSJqBOVLo38cwVbyTcX5Nk/nHENjDSN7as3UvbXa7eT4Xswy1GARGAZ3MAaLTZn1+Cctn0MDKniQOS6QDryYgKWz8ko/H4T9XCxgjHJVsL6obezaPZF+pibyyVPCuePssuxUbFHF6yiP5rCfAsK6VTv/8pbYGauGpYHDgnM941RtN2ThltORgi+P9i9wQ8VRBQpEm1RvDXOqJ7OwIDAQABMA0GCSqGSIb3DQEBCwUAA4IBAQB5L26tpco6EgVunmZYBAFiFE+Dhqwvy4J1iKuXApaKhqabeKJ8kBv/pJBnZl7CRF5Pv8dLfhNoNm2BsXbpH91/rhDj9zl/Imkc5ttVGbXbKSBpUaduwBZpsVIX0xCugNPflHFz9kf/zsGWb3X6wO/2eNewj3fr8jNRC/KWQ7otcdqwYbe1BO4yo6FjAIs5L+wCQcc2JjRWgBon4wL25ccX3nH8aMHl4/gz5trKwPqH0/lYcScJmMSRPzHbmd62LlmZE9eWEwuYJ+h8fssTZA9JTMXvkPhg05w2snaM9XdSuXIRo4UtqGpMQC0KRMmwDHbVSluX63wn7iSZD4TGHZGa"
    GOOGLE_IDP_NAME_ID_FORMAT    = "urn:oasis:names:tc:SAML:1.1:nameid-format:unspecified"
    GOOGLE_IDP_CORE_ENTITY_ID    = "https://sso.uds.dev/realms/uds"
    GOOGLE_IDP_ADMIN_GROUP       = "uds-core-dev-admin"
    GOOGLE_IDP_AUDITOR_GROUP     = "uds-core-dev-auditor"
    EMAIL_VERIFICATION_ENABLED   = false
    TERMS_AND_CONDITIONS_ENABLED = true
    X509_OCSP_FAIL_OPEN          = true
  })
  keycloak_realm_auth_flows = yamlencode({
    USERNAME_PASSWORD_AUTH_ENABLED = false
    X509_AUTH_ENABLED              = false
    SOCIAL_AUTH_ENABLED            = true
    OTP_ENABLED                    = false
    WEBAUTHN_ENABLED               = false
    X509_MFA_ENABLED               = false
  })
  keycloak_theme_customization_settings = yamlencode({
    enableRegistrationFields = false
  })
  # == core-logging ==
  loki_write_replica_count = 1
}
