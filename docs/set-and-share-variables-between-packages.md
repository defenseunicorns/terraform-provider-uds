## Sharing zarf runtime set variables between packages

The `uds_package` resource now automatically persists runtime set variables produced by a package deployment (via Zarf actions that call `setVariable`) into two computed maps on the resource:

- `set_variables` (Read-Only, Map(String)): non-sensitive runtime variables written by the package at deploy time.
- `sensitive_set_variables` (Read-Only, Map(String), Sensitive): sensitive runtime variables written by the package at deploy time.

Notes:
- Only runtime set variables produced by the package deploy result are persisted to `set_variables` / `sensitive_set_variables`.
- Reference runtime values via `uds_package.<name>.set_variables.key` for non-sensitive values and `uds_package.<name>.sensitive_set_variables.key` for sensitive values. Note that keys are stored using the runtime variable names set by the package, but are normalized to all lowercase before being persisted to the `set_variables` / `sensitive_set_variables` maps.
- Because `sensitive_set_variables` is marked sensitive, Terraform will treat those values as sensitive in state and hide them from plan output.
- If you require an explicit deployment ordering, add an explicit `depends_on` so the consumer waits for the producer.


Simple example showing a deps zarf package and then an example of how to use it with the provider:

Example zarf package that creates a variable called AUTHSERVICE_REDIS_URI
```yaml
kind: ZarfPackageConfig
metadata:
  name: authservice-ha-deps
  version: 1.0.0

variables:
  - name: AUTHSERVICE_REDIS_URI
    sensitive: true

components:
  - name: valkey
    required: true
    charts:
      - name: uds-valkey-config
        namespace: valkey
        version: 0.1.0
        localPath: ../chart
      - name: valkey
        version: 4.0.2
        namespace: valkey
        url: oci://ghcr.io/defenseunicorns/bitferno/valkey
        valuesFiles:
          - ../values/values.yaml
    images:
      - bitnamilegacy/valkey:8.1.3-debian-12-r3
      - bitnamilegacy/redis-exporter:1.76.0-debian-12-r0
      - bitnamilegacy/valkey-sentinel:8.1.3-debian-12-r3

  - name: core-secrets
    required: true
    actions:
      onDeploy:
        before:
          - cmd: |
              uds zarf tools kubectl get secret valkey-valkey \
                -n valkey \
                -o jsonpath='{.data.valkey-password}' \
                | base64 -d
            mute: true
            setVariables:
              - name: VALKEY_PASSWORD
                sensitive: true

          - cmd: |
              echo "redis://:${ZARF_VAR_VALKEY_PASSWORD}@valkey-valkey-primary.valkey.svc.cluster.local:6379"
            mute: true
            setVariables:
              - name: AUTHSERVICE_REDIS_URI
                sensitive: true
```

Example provider usage of the above zarf package
```hcl
resource "uds_package" "init" {
  source = "oci://ghcr.io/zarf-dev/packages/init:v0.73.0"
}

# This package produces sensitive variables using zarf action setVariables sensitive: true. The variable is AUTHSERVICE_REDIS_URI in this example

resource "uds_package" "authservice-ha-deps" {
  depends_on = [uds_package.init]
  source     = "../build/authservice-ha-deps/*.zst"
}

resource "uds_package" "core-identity-authorization" {
  depends_on = [uds_package.authservice-ha-deps]
  source       = "oci://ghcr.io/defenseunicorns/packages/uds/core-identity-authorization:${local.uds_version}"

  component {
    name = "authservice"

    override {
      chart_name = "authservice"
      values = [
        { path = "redis.uri", value = uds_package.authservice-ha-deps.sensitive_set_vars.authservice_redis_uri }
       ]
     }
  } 
}
```
