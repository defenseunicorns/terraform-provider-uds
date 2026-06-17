// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package acc

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"testing"
	"text/template"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// renovate: datasource=github-tags depName=zarf-dev/zarf
const initPackageVersion = "v0.79.0"

func buildZarfValuesFixture(t *testing.T) string {
	t.Helper()

	fixtureDir, err := filepath.Abs("fixtures/zarf_values")
	if err != nil {
		t.Fatal(err)
	}
	outputDir := t.TempDir()

	cmd := exec.Command(
		"uds",
		"zarf",
		"package",
		"create",
		fixtureDir,
		"--architecture", runtime.GOARCH,
		"--confirm",
		"--features", "values=true",
		"--output", outputDir,
		"--skip-sbom",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build values package: %v\n%s", err, output)
	}

	packagePath := filepath.Join(outputDir, fmt.Sprintf("zarf-package-zarf-values-%s-0.1.0.tar.zst", runtime.GOARCH))
	if _, err := os.Stat(packagePath); err != nil {
		t.Fatalf("expected values package at %s: %v\n%s", packagePath, err, output)
	}
	return packagePath
}

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

var testAccPackageResourcePlanValidationDisabledConfig = fmt.Sprintf(`
provider "uds" {
  validate_packages_on_plan = false
}

resource "uds_package" "init" {
  source       = "oci://ghcr.io/zarf-dev/packages/init:values-test-missing"
  architecture = "%s"
  optional_components = ["definitely-not-a-component"]

  values = {
    definitely_unexposed_by_zarf_init_values_test = "deferred"
  }
}
`, runtime.GOARCH)

func TestAccPackageResourcePlanValidation(t *testing.T) {
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
			// Disabling package validation on plan skips package-dependent checks.
			// This unavailable source would otherwise fail package loading, signature
			// verification, optional_components validation, and values sourcePath validation.
			{
				Config:             testAccPackageResourcePlanValidationDisabledConfig,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

type zarfValuesTestConfig struct {
	PackagePath       string
	InitPackageSource string
	Architecture      string
	AnnotationVersion string
	Message           string
	Secret            string
	Enabled           bool
	Replicas          int
	ListItemOne       string
	ListItemTwo       string
	AppLabel          string
}

func renderZarfValuesPackageConfig(t *testing.T, config zarfValuesTestConfig) string {
	t.Helper()

	const configTemplate = `
locals {
  test_version = "{{ .AnnotationVersion }}"
}

resource "terraform_data" "example" {
  input = {
    version = local.test_version
  }

  triggers_replace = [local.test_version]
}

resource "uds_package" "init" {
  source       = "{{ .InitPackageSource }}"
  architecture = "{{ .Architecture }}"
  signature_verification = {
    keyless = {
      certificate_identity_regexp = "https://github\\.com/zarf-dev/zarf/\\.github/workflows/release\\.yml@refs/tags/v\\d+\\.\\d+\\.\\d+"
      certificate_oidc_issuer     = "https://token.actions.githubusercontent.com"
    }
  }
}

resource "uds_package" "values" {
  depends_on   = [uds_package.init]
  source       = "{{ .PackagePath }}"
  architecture = "{{ .Architecture }}"
  namespace    = "zarf-values"

  values = {
    config = {
      annotations = {
        test-id      = terraform_data.example.id
        test-version = local.test_version
      }
      settings = {
        message = "{{ .Message }}"
      }
      enabled = {{ .Enabled }}
      items = [
        "{{ .ListItemOne }}",
        "{{ .ListItemTwo }}",
      ]
      labels = {
        app = "{{ .AppLabel }}"
      }
      replicas = {{ .Replicas }}
    }
  }

  sensitive_values = {
    config = {
      settings = {
        secret = "{{ .Secret }}"
      }
    }
  }
}
`

	tmpl, err := template.New("zarf values package config").Parse(configTemplate)
	if err != nil {
		t.Fatal(err)
	}

	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, config); err != nil {
		t.Fatal(err)
	}

	return rendered.String()
}

func checkAppliedZarfValuesConfigMap(expectedConfig zarfValuesTestConfig) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		kubeconfigPath := os.Getenv("KUBECONFIG")
		if kubeconfigPath == "" {
			kubeconfigPath = clientcmd.RecommendedHomeFile
		}

		kubeClientConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return fmt.Errorf("failed to build kubernetes config: %w", err)
		}
		clientset, err := kubernetes.NewForConfig(kubeClientConfig)
		if err != nil {
			return fmt.Errorf("failed to create kubernetes client: %w", err)
		}

		configMap, err := clientset.CoreV1().ConfigMaps("zarf-values").Get(context.Background(), "zarf-values", metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get values acceptance configmap: %w", err)
		}

		checks := []struct {
			label    string
			expected string
			actual   string
		}{
			{label: "configmap data.message", expected: expectedConfig.Message, actual: configMap.Data["message"]},
			{label: "configmap data.enabled", expected: strconv.FormatBool(expectedConfig.Enabled), actual: configMap.Data["enabled"]},
			{label: "configmap data.item-0", expected: expectedConfig.ListItemOne, actual: configMap.Data["item-0"]},
			{label: "configmap data.item-1", expected: expectedConfig.ListItemTwo, actual: configMap.Data["item-1"]},
			{label: "configmap data.replicas", expected: strconv.Itoa(expectedConfig.Replicas), actual: configMap.Data["replicas"]},
			{label: "configmap data.secret", expected: expectedConfig.Secret, actual: configMap.Data["secret"]},
			{label: "configmap label app", expected: expectedConfig.AppLabel, actual: configMap.Labels["app"]},
			{label: "configmap annotation test-version", expected: expectedConfig.AnnotationVersion, actual: configMap.Annotations["test-version"]},
		}
		for _, check := range checks {
			if check.actual != check.expected {
				return fmt.Errorf("expected %s %q, got %q", check.label, check.expected, check.actual)
			}
		}
		if actual := configMap.Annotations["test-id"]; actual == "" {
			return fmt.Errorf("expected configmap annotation test-id to be set")
		}

		return nil
	}
}

func renderZarfValuesInvalidPathConfig(packagePath string) string {
	return fmt.Sprintf(`
resource "uds_package" "values" {
  source       = %q
  architecture = "%s"
  namespace    = "zarf-values"

  values = {
    not_exposed = "invalid"
  }
}
`, packagePath, runtime.GOARCH)
}

func TestAccPackageResourceLocalZarfValues(t *testing.T) {
	if os.Getenv(resource.EnvTfAcc) == "" {
		t.Skip("Acceptance tests skipped unless TF_ACC=1")
	}

	packagePath := buildZarfValuesFixture(t)
	initialValues := zarfValuesTestConfig{
		PackagePath:       packagePath,
		InitPackageSource: fmt.Sprintf("oci://ghcr.io/zarf-dev/packages/init:%s", initPackageVersion),
		Architecture:      runtime.GOARCH,
		AnnotationVersion: "0.1.0",
		Message:           "hello-values",
		Secret:            "sensitive-value",
		Enabled:           true,
		Replicas:          3,
		ListItemOne:       "alpha",
		ListItemTwo:       "bravo",
		AppLabel:          "zarf-values",
	}
	updatedValues := initialValues
	updatedValues.AnnotationVersion = "0.1.1"
	updatedValues.Message = "updated-values"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      renderZarfValuesInvalidPathConfig(packagePath),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`value path "not_exposed" does not match any`),
			},
			{
				Config: renderZarfValuesPackageConfig(t, initialValues),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uds_package.values", "id", "zarf-values:zarf-values"),
					resource.TestCheckResourceAttr("uds_package.values", "metadata.version", "0.1.0"),
					checkAppliedZarfValuesConfigMap(initialValues),
				),
			},
			{
				Config: renderZarfValuesPackageConfig(t, updatedValues),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uds_package.values", "id", "zarf-values:zarf-values"),
					resource.TestCheckResourceAttr("uds_package.values", "metadata.version", "0.1.0"),
					checkAppliedZarfValuesConfigMap(updatedValues),
				),
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
