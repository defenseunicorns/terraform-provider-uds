// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package acc

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

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
					resource.TestCheckResourceAttr("uds_package.init", "metadata.version", "v0.48.0"),
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
  repository   = "ghcr.io/zarf-dev/packages/init"
  ref          = "v0.48.0"
  architecture = uds_bundle_metadata.example_bundle.architecture
}
`, runtime.GOARCH)

func TestInitPackage(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccInitPackageResourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uds_package.init", "metadata.version", "v0.59.0"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

var testAccInitPackageResourceConfig = fmt.Sprintf(`
resource "uds_package" "init" {
  name         = "init"
  repository   = "ghcr.io/zarf-dev/packages/init"
  ref          = "v0.59.0"
  architecture = "%s"
}
`, runtime.GOARCH)
