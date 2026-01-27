#resource "uds_package" "core_metrics_server" {
#  source     = "oci://ghcr.io/defenseunicorns/packages/uds/core-metrics-server:${local.uds_version}"
#  depends_on = [uds_package.core_base]
#}
#
#resource "uds_package" "core_runtime_security" {
#  source     = "oci://ghcr.io/defenseunicorns/packages/uds/core-runtime-security:${local.uds_version}"
#  depends_on = [uds_package.core_base]
#}
#
#resource "uds_package" "core_logging" {
#  source     = "oci://ghcr.io/defenseunicorns/packages/uds/core-logging:${local.uds_version}"
#  depends_on = [uds_package.core_base]
#
#  component {
#    name = "loki"
#    override {
#      chart_name = "loki"
#      values = [
#        { path = "write.replicas", value = 1 },
#        { path = "read.replicas", value = 1 },
#        { path = "backend.replicas", value = 1 }
#      ]
#    }
#  }
#}
#
#resource "uds_package" "core_backup_restore" {
#  source     = "oci://ghcr.io/defenseunicorns/packages/uds/core-backup-restore:${local.uds_version}"
#  depends_on = [uds_package.core_base]
#}
#
#
