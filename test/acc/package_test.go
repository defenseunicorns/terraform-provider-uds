// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package acc

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

const initPackageVersion = "v0.59.0"

func TestAccPackageResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccPackageResourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uds_bundle_metadata.example_bundle", "version", "0.0.1"),
					resource.TestCheckResourceAttr("uds_package.init", "metadata.version", initPackageVersion),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

var testAccPackageResourceConfig = fmt.Sprintf(`
resource "uds_bundle_metadata" "example_bundle" {
  version      = "0.0.1"
  kind         = "UDSBundle"
  description  = "A demo bundle for the podinfo and nginx packages"
  architecture = "%s"
}

resource "uds_package" "init" {
  name         = "init"
  source       = "oci://ghcr.io/zarf-dev/packages/init:%s"
  architecture = uds_bundle_metadata.example_bundle.architecture
}
`, runtime.GOARCH, initPackageVersion)

func TestInitPackage(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccInitPackageResourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uds_package.init", "metadata.version", initPackageVersion),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

var testAccInitPackageResourceConfig = fmt.Sprintf(`
resource "uds_package" "init" {
  name         = "init"
  source       = "oci://ghcr.io/zarf-dev/packages/init:%s"
  architecture = "%s"
}
`, initPackageVersion, runtime.GOARCH)
