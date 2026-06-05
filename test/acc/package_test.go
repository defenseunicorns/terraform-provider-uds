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

var testAccPackageResourceRemoteValuesInvalidPathConfig = fmt.Sprintf(`
resource "uds_package" "init" {
  source       = "oci://ghcr.io/zarf-dev/packages/init:%s"
  architecture = "%s"
  signature_verification = {
    keyless = {
      certificate_identity_regexp = "https://github\\.com/zarf-dev/zarf/\\.github/workflows/release\\.yml@refs/tags/v\\d+\\.\\d+\\.\\d+"
      certificate_oidc_issuer     = "https://token.actions.githubusercontent.com"
    }
  }

  values = {
    definitely_unexposed_by_zarf_init_values_test = "invalid"
  }
}
`, initPackageVersion, runtime.GOARCH)

var testAccPackageResourceRemoteValuesUnavailableConfig = fmt.Sprintf(`
resource "uds_package" "init" {
  source       = "oci://ghcr.io/zarf-dev/packages/init:values-test-missing"
  architecture = "%s"
  signature_verification = {
    verify = false
  }

  values = {
    definitely_unexposed_by_zarf_init_values_test = "deferred"
  }
}
`, runtime.GOARCH)

func TestAccPackageResourceRemoteValuesPlanValidation(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Zarf init is a remote package, but this published version does not expose
			// values mappings. This still proves remote metadata is loaded during plan
			// and configured value paths are rejected when not exposed by that package.
			// Add a positive remote values test once a published package exposes values.
			{
				Config:      testAccPackageResourceRemoteValuesInvalidPathConfig,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`value path "definitely_unexposed_by_zarf_init_values_test" does not match any`),
			},
			// If remote metadata cannot be loaded during plan, values sourcePath
			// validation should defer to apply instead of blocking the plan.
			{
				Config:             testAccPackageResourceRemoteValuesUnavailableConfig,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
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
}

resource "uds_package" "dos_games" {
  depends_on   = [uds_package.init]
  source       = "oci://ghcr.io/zarf-dev/packages/dos-games:1.2.0"
  architecture = uds_package.init.architecture
  namespace    = "demo"
  signature_verification = {
    public_key = file("%s")
  }
}

# TODO: Add uds_package resource with local package reference
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
					resource.TestCheckResourceAttr("uds_package.init", "id", "init"),
					resource.TestCheckResourceAttr("uds_package.init", "metadata.version", initPackageVersion),
					resource.TestCheckResourceAttr("uds_package.dos_games", "id", "demo:dos-games"),
					resource.TestCheckResourceAttr("uds_package.dos_games", "metadata.version", "1.2.0"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}
