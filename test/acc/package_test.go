// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package acc

import (
	"fmt"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// renovate: datasource=github-tags depName=zarf-dev/zarf
const initPackageVersion = "v0.75.0"

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
  description  = "A demo bundle for the init package"
  architecture = "%s"
}

resource "uds_package" "init" {
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
  source       = "oci://ghcr.io/zarf-dev/packages/init:%s"
  architecture = "%s"
}
`, initPackageVersion, runtime.GOARCH)

func TestNamespacedPackage(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccNamespacedPackageResourceConfig(t),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uds_package.dos_games", "namespace", "demo"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccNamespacedPackageResourceConfig(t *testing.T) string {
	t.Helper()
	// terraform-plugin-testing runs Terraform from a temp directory, so file() requires
	// an absolute path. We resolve it here relative to the test package directory.
	pubKeyPath, err := filepath.Abs("dosgames.pub")
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf(`
resource "uds_package" "init" {
  source       = "oci://ghcr.io/zarf-dev/packages/init:%s"
  architecture = "%s"
}

resource "uds_package" "dos_games" {
  source       = "oci://ghcr.io/zarf-dev/packages/dos-games:1.2.0"
  architecture = uds_package.init.architecture
  namespace    = "demo"
  depends_on   = [uds_package.init]

  public_key = file("%s")
}
`, initPackageVersion, runtime.GOARCH, pubKeyPath)
}

// CLI-173: end-to-end check that the configured timeout bounds the entire
// create operation (cluster wait + package load + Zarf deploy), not just the
// inner Zarf deploy call. We use a tiny timeout so the operation cannot
// possibly complete; the apply must surface a timeout-flavored diagnostic
// rather than running for many minutes.
func TestAccPackageResource_TimeoutEnforced_Create(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccPackageResourceTimeoutCreateConfig,
				ExpectError: regexp.MustCompile(`(?s)Error creating package.*exceeded the configured timeout of 1s`),
			},
		},
	})
}

var testAccPackageResourceTimeoutCreateConfig = fmt.Sprintf(`
resource "uds_package" "init" {
  source       = "oci://ghcr.io/zarf-dev/packages/init:%s"
  architecture = "%s"
  timeout      = "1s"
}
`, initPackageVersion, runtime.GOARCH)

// CLI-173: end-to-end check that the configured timeout bounds the entire
// update operation. Step 1 deploys successfully with the default timeout.
// Step 2 adds a component override (forcing Update) and lowers the timeout
// to 1s so the redeploy cannot complete in time. The auto-destroy at the
// end of the TestCase uses Step 1's state (timeout=15m) and is unaffected.
func TestAccPackageResource_TimeoutEnforced_Update(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPackageResourceTimeoutUpdateBaseline,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uds_package.init", "metadata.version", initPackageVersion),
				),
			},
			{
				Config:      testAccPackageResourceTimeoutUpdateTight,
				ExpectError: regexp.MustCompile(`(?s)Error updating package.*exceeded the configured timeout of 1s`),
			},
		},
	})
}

var testAccPackageResourceTimeoutUpdateBaseline = fmt.Sprintf(`
resource "uds_package" "init" {
  source       = "oci://ghcr.io/zarf-dev/packages/init:%s"
  architecture = "%s"
}
`, initPackageVersion, runtime.GOARCH)

var testAccPackageResourceTimeoutUpdateTight = fmt.Sprintf(`
resource "uds_package" "init" {
  source       = "oci://ghcr.io/zarf-dev/packages/init:%s"
  architecture = "%s"
  timeout      = "1s"

  component {
    name = "zarf-registry"
    override {
      chart_name = "docker-registry"
      values = [
        { path = "replicaCount", value = "1" },
      ]
    }
  }
}
`, initPackageVersion, runtime.GOARCH)
