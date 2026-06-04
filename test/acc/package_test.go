// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package acc

import (
	"fmt"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// renovate: datasource=github-tags depName=zarf-dev/zarf
const initPackageVersion = "v0.77.0"

var testAccPackageResourceConfig = fmt.Sprintf(`
resource "uds_package" "init" {
  source       = "oci://ghcr.io/zarf-dev/packages/init:%s"
  architecture = "%s"
  signature_verification = {
	keyless = {
	  certificate_identity_regexp = "https://github\\.com/zarf-dev/zarf/\\.github/workflows/release\\.yml@refs/tags/v\\d+\\.\\d+\\.\\d+"
      certificate_oidc_issuer     = "https://token.actions.githubusercontent.com"
	}
  }
}
`, initPackageVersion, runtime.GOARCH)

func TestAccPackageResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccPackageResourceConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uds_package.init", "id", "init"),
					resource.TestCheckResourceAttr("uds_package.init", "metadata.version", initPackageVersion),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccMultiplePackageResourcesConfig(t *testing.T) string {
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
  signature_verification = {
	keyless = {
	  certificate_identity_regexp = "https://github\\.com/zarf-dev/zarf/\\.github/workflows/release\\.yml@refs/tags/v\\d+\\.\\d+\\.\\d+"
      certificate_oidc_issuer     = "https://token.actions.githubusercontent.com"
	}
  }
  component {
    name = "git-server"
  }
}

resource "uds_package" "dos_games" {
  source       = "oci://ghcr.io/zarf-dev/packages/dos-games:1.2.0"
  architecture = uds_package.init.architecture
  namespace    = "demo"
  depends_on   = [uds_package.init]
  signature_verification = {
  	public_key = file("%s")
  }
}

# TODO: Add resource with local package reference
`, initPackageVersion, runtime.GOARCH, pubKeyPath)
}

func TestAccMultiplePackageResources(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccMultiplePackageResourcesConfig(t),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uds_package.init", "metadata.version", initPackageVersion),
					resource.TestCheckResourceAttr("uds_package.init", "id", "init"),
					resource.TestCheckResourceAttr("uds_package.dos_games", "metadata.version", "1.2.0"),
					resource.TestCheckResourceAttr("uds_package.dos_games", "id", "demo:dos-games"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}
