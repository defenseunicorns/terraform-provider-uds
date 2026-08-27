// Copyright 2024-2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package acc

import (
	"bytes"
	"context"
	"encoding/json"
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
	zarfState "github.com/zarf-dev/zarf/src/pkg/state"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	// renovate: datasource=github-tags depName=zarf-dev/zarf
	initPackageVersion = "v0.83.0"

	// renovate: datasource=docker depName=ghcr.io/defenseunicorns/packages/uds/core-crds versioning=semver extractVersion=^(?<version>.*)-upstream$
	udsCoreCRDsPackageVersion = "1.10.0"

	// renovate: datasource=docker depName=ghcr.io/defenseunicorns/packages/uds/nginx versioning=semver extractVersion=^(?<version>.*)-upstream$
	udsNginxPackageVersion = "1.31.1-uds.1"

	udsPackageFlavor = "upstream"
)

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

func buildZarfLifecycleFixture(t *testing.T) string {
	t.Helper()

	fixtureDir, err := filepath.Abs("fixtures/zarf_lifecycle")
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
		"--output", outputDir,
		"--skip-sbom",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build lifecycle package: %v\n%s", err, output)
	}

	packagePath := filepath.Join(outputDir, fmt.Sprintf("zarf-package-zarf-lifecycle-%s-0.1.0.tar.zst", runtime.GOARCH))
	if _, err := os.Stat(packagePath); err != nil {
		t.Fatalf("expected lifecycle package at %s: %v\n%s", packagePath, err, output)
	}
	return packagePath
}

func buildFailedPackageFixture(t *testing.T) string {
	t.Helper()

	fixtureDir, err := filepath.Abs("fixtures/failed_package")
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
		"--output", outputDir,
		"--skip-sbom",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build failed package: %v\n%s", err, output)
	}

	packagePath := filepath.Join(outputDir, fmt.Sprintf("zarf-package-failed-package-%s-0.1.0.tar.zst", runtime.GOARCH))
	if _, err := os.Stat(packagePath); err != nil {
		t.Fatalf("expected failed package at %s: %v\n%s", packagePath, err, output)
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

var testAccPackageResourceRemoteValuesConfig = fmt.Sprintf(`
resource "uds_package" "nginx" {
  source       = "oci://ghcr.io/defenseunicorns/packages/uds/nginx:%s-%s"
  architecture = "%s"

  values = {
    nginx = {
      replicaCount = 1
    }
  }
}
`, udsNginxPackageVersion, udsPackageFlavor, runtime.GOARCH)

var testAccPackageResourceRemoteValuesInvalidPathConfig = fmt.Sprintf(`
resource "uds_package" "nginx" {
  source       = "oci://ghcr.io/defenseunicorns/packages/uds/nginx:%s-%s"
  architecture = "%s"

  values = {
    nginx = {
      definitely_unexposed_by_nginx_values_test = "invalid"
    }
  }
}
`, udsNginxPackageVersion, udsPackageFlavor, runtime.GOARCH)

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
			// Remote packages load metadata during plan so configured values can be
			// validated before apply.
			{
				Config:             testAccPackageResourceRemoteValuesConfig,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config:   testAccPackageResourceRemoteValuesInvalidPathConfig,
				PlanOnly: true,
				ExpectError: regexp.MustCompile(
					`Additional property definitely_unexposed_by_nginx_values_test is not\s+allowed`,
				),
			},
			// Disabling package validation on plan skips package-dependent checks.
			// This unavailable source would otherwise fail package loading, signature
			// verification and optional_components validation.
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
	CoreCRDsSource    string
	NginxSource       string
	Architecture      string
	AnnotationVersion string
	Message           string
	Secret            string
	Enabled           bool
	Replicas          int
	ListItemOne       string
	ListItemTwo       string
	AppLabel          string
	NginxReplicas     int
	NginxAnnotation   string
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

resource "uds_package" "uds_crds" {
  depends_on = [uds_package.init]

  source       = "{{ .CoreCRDsSource }}"
  architecture = "{{ .Architecture }}"
}

resource "uds_package" "nginx" {
  depends_on = [uds_package.init, uds_package.uds_crds]

  source       = "{{ .NginxSource }}"
  architecture = "{{ .Architecture }}"

  values = {
    nginx = {
      replicaCount = {{ .NginxReplicas }}
      podAnnotations = {
        "example.com/source" = "{{ .NginxAnnotation }}"
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

func checkAppliedZarfValuesResources(expectedConfig zarfValuesTestConfig) resource.TestCheckFunc {
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

		deployment, err := clientset.AppsV1().Deployments("nginx").Get(context.Background(), "nginx", metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get nginx deployment: %w", err)
		}
		if deployment.Spec.Replicas == nil {
			return fmt.Errorf("expected nginx deployment replicas to be set")
		}
		if actual := int(*deployment.Spec.Replicas); actual != expectedConfig.NginxReplicas {
			return fmt.Errorf("expected nginx deployment replicas %d, got %d", expectedConfig.NginxReplicas, actual)
		}
		if actual := deployment.Spec.Template.Annotations["example.com/source"]; actual != expectedConfig.NginxAnnotation {
			return fmt.Errorf("expected nginx pod annotation example.com/source %q, got %q", expectedConfig.NginxAnnotation, actual)
		}

		return nil
	}
}

func checkZarfValuesResourcesDestroyed(_ *terraform.State) error {
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

	checks := []struct {
		label string
		get   func() error
	}{
		{
			label: "values acceptance configmap",
			get: func() error {
				_, err := clientset.CoreV1().ConfigMaps("zarf-values").Get(context.Background(), "zarf-values", metav1.GetOptions{})
				return err
			},
		},
		{
			label: "nginx deployment",
			get: func() error {
				_, err := clientset.AppsV1().Deployments("nginx").Get(context.Background(), "nginx", metav1.GetOptions{})
				return err
			},
		},
	}
	for _, check := range checks {
		if err := check.get(); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to check %s cleanup: %w", check.label, err)
		} else if err == nil {
			return fmt.Errorf("expected %s to be deleted", check.label)
		}
	}

	return nil
}

func renderZarfLifecyclePackageConfig(packagePath string) string {
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

resource "uds_package" "lifecycle" {
  depends_on   = [uds_package.init]
  source       = %q
  architecture = "%s"
}
`, initPackageVersion, runtime.GOARCH, packagePath, runtime.GOARCH)
}

func checkAppliedZarfLifecycleResources(_ *terraform.State) error {
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

	actionMarkers, err := clientset.CoreV1().ConfigMaps("default").Get(context.Background(), "zarf-lifecycle-action-markers", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get action marker configmap: %w", err)
	}
	for label, expected := range map[string]string{
		"non-muted-action-marker": "non-muted-action-marker",
		"muted-action-marker":     "muted-action-marker",
	} {
		if actual := actionMarkers.Data[label]; actual != expected {
			return fmt.Errorf("expected %s marker %q, got %q", label, expected, actual)
		}
	}

	packageState, err := clientset.CoreV1().ConfigMaps("default").Get(context.Background(), "zarf-lifecycle-package-state", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get managed package-state configmap: %w", err)
	}
	if actual := packageState.Data["fixture"]; actual != "zarf-lifecycle-package-state" {
		return fmt.Errorf("expected managed package-state marker %q, got %q", "zarf-lifecycle-package-state", actual)
	}

	return nil
}

func checkZarfLifecycleResourcesDestroyed(_ *terraform.State) error {
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

	_, err = clientset.CoreV1().ConfigMaps("default").Get(context.Background(), "zarf-lifecycle-action-markers", metav1.GetOptions{})
	if err == nil {
		return fmt.Errorf("expected action marker configmap to be deleted")
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to check action marker cleanup: %w", err)
	}

	_, err = clientset.CoreV1().ConfigMaps("default").Get(context.Background(), "zarf-lifecycle-package-state", metav1.GetOptions{})
	if err == nil {
		return fmt.Errorf("expected managed fixture configmap to be deleted")
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to check managed fixture cleanup: %w", err)
	}

	return nil
}

func TestAccPackageResourceZarfValues(t *testing.T) {
	if os.Getenv(resource.EnvTfAcc) == "" {
		t.Skip("Acceptance tests skipped unless TF_ACC=1")
	}

	packagePath := buildZarfValuesFixture(t)
	initialValues := zarfValuesTestConfig{
		PackagePath:       packagePath,
		InitPackageSource: fmt.Sprintf("oci://ghcr.io/zarf-dev/packages/init:%s", initPackageVersion),
		CoreCRDsSource:    fmt.Sprintf("oci://ghcr.io/defenseunicorns/packages/uds/core-crds:%s-%s", udsCoreCRDsPackageVersion, udsPackageFlavor),
		NginxSource:       fmt.Sprintf("oci://ghcr.io/defenseunicorns/packages/uds/nginx:%s-%s", udsNginxPackageVersion, udsPackageFlavor),
		Architecture:      runtime.GOARCH,
		AnnotationVersion: "0.1.0",
		Message:           "hello-values",
		Secret:            "sensitive-value",
		Enabled:           true,
		Replicas:          3,
		ListItemOne:       "alpha",
		ListItemTwo:       "bravo",
		AppLabel:          "zarf-values",
		NginxReplicas:     1,
		NginxAnnotation:   "terraform-provider-uds-initial",
	}
	updatedValues := initialValues
	updatedValues.AnnotationVersion = "0.1.1"
	updatedValues.Message = "updated-values"
	updatedValues.NginxReplicas = 2
	updatedValues.NginxAnnotation = "terraform-provider-uds-updated"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkZarfValuesResourcesDestroyed,
		Steps: []resource.TestStep{
			{
				Config: renderZarfValuesPackageConfig(t, initialValues),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uds_package.values", "id", "zarf-values:zarf-values"),
					resource.TestCheckResourceAttr("uds_package.values", "metadata.version", "0.1.0"),
					resource.TestCheckResourceAttr("uds_package.uds_crds", "id", "core-crds"),
					resource.TestCheckResourceAttr("uds_package.uds_crds", "metadata.version", udsCoreCRDsPackageVersion),
					resource.TestCheckResourceAttr("uds_package.nginx", "id", "nginx"),
					resource.TestCheckResourceAttr("uds_package.nginx", "metadata.version", udsNginxPackageVersion),
					checkAppliedZarfValuesResources(initialValues),
				),
			},
			{
				Config: renderZarfValuesPackageConfig(t, updatedValues),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uds_package.values", "id", "zarf-values:zarf-values"),
					resource.TestCheckResourceAttr("uds_package.values", "metadata.version", "0.1.0"),
					resource.TestCheckResourceAttr("uds_package.uds_crds", "id", "core-crds"),
					resource.TestCheckResourceAttr("uds_package.uds_crds", "metadata.version", udsCoreCRDsPackageVersion),
					resource.TestCheckResourceAttr("uds_package.nginx", "id", "nginx"),
					resource.TestCheckResourceAttr("uds_package.nginx", "metadata.version", udsNginxPackageVersion),
					checkAppliedZarfValuesResources(updatedValues),
				),
			},
		},
	})
}

func TestAccPackageResourceZarfLifecycle(t *testing.T) {
	if os.Getenv(resource.EnvTfAcc) == "" {
		t.Skip("Acceptance tests skipped unless TF_ACC=1")
	}

	packagePath := buildZarfLifecycleFixture(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkZarfLifecycleResourcesDestroyed,
		Steps: []resource.TestStep{
			{
				Config: renderZarfLifecyclePackageConfig(packagePath),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uds_package.lifecycle", "id", "zarf-lifecycle"),
					resource.TestCheckResourceAttr("uds_package.lifecycle", "metadata.version", "0.1.0"),
					checkAppliedZarfLifecycleResources,
				),
			},
		},
	})
}

func TestAccPackageResourceFailedCreateRetry(t *testing.T) {
	if os.Getenv(resource.EnvTfAcc) == "" {
		t.Skip("Acceptance tests skipped unless TF_ACC=1")
	}

	packagePath := buildFailedPackageFixture(t)
	initialDigest := ""
	recoveryGeneration := 0

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkFailedPackageResourcesDestroyed,
		Steps: []resource.TestStep{
			{
				Config: renderFailedPackageConfig(packagePath, true, ""),
				ExpectError: regexp.MustCompile(
					`intentional acceptance-test\s+deployment failure`,
				),
			},
			{
				Config: renderFailedPackageConfig(packagePath, false, ""),
				// terraform-plugin-testing does not execute Check for an ExpectError
				// step. The successful same-address recovery below therefore verifies
				// Terraform's partial state indirectly: without the persisted state,
				// Create would reject the existing package as a duplicate.
				PreConfig: func() {
					deployedPackage, err := readFailedPackageState()
					if err != nil {
						t.Fatal(err)
					}
					if deployedPackage.Generation != 1 {
						t.Fatalf("expected initial package generation 1, got %d", deployedPackage.Generation)
					}
					if deployedPackage.Digest == "" {
						t.Fatal("expected non-empty failed package digest")
					}
					if len(deployedPackage.DeployedComponents) != 1 || deployedPackage.DeployedComponents[0].Status != zarfState.ComponentStatusFailed {
						t.Fatalf("expected failed component status, got %#v", deployedPackage.DeployedComponents)
					}
					if charts := deployedPackage.DeployedComponents[0].InstalledCharts; len(charts) != 1 || charts[0].Status != zarfState.ChartStatusFailed {
						t.Fatalf("expected failed installed chart status, got %#v", charts)
					}
					initialDigest = deployedPackage.Digest
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uds_package.failed", "id", "failed-package:failed-package"),
					resource.TestCheckResourceAttr("uds_package.failed", "metadata.status", "Succeeded"),
					checkRecoveredFailedPackageState(&initialDigest, &recoveryGeneration),
				),
			},
			{
				Config: renderFailedPackageConfig(packagePath, false, "subsequent-update"),
				Check:  checkSubsequentPackageUpdate(&recoveryGeneration, &initialDigest),
			},
		},
	})
}

func renderFailedPackageConfig(packagePath string, failDeployment bool, diagnosticValue string) string {
	return fmt.Sprintf(`
resource "uds_package" "failed" {
  source       = %q
  architecture = "%s"
  namespace    = "failed-package"

  signature_verification = {
    verify = false
  }

  values = {
    failed = {
      failDeployment = %t
      diagnosticValue = %q
    }
  }
}
`, packagePath, runtime.GOARCH, failDeployment, diagnosticValue)
}

func checkRecoveredFailedPackageState(initialDigest *string, recoveryGeneration *int) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		resourceState, ok := state.RootModule().Resources["uds_package.failed"]
		if !ok || resourceState.Primary == nil {
			return fmt.Errorf("expected recovered failed package in Terraform state")
		}
		if actual := resourceState.Primary.Attributes["namespace"]; actual != "failed-package" {
			return fmt.Errorf("expected Terraform namespace %q, got %q", "failed-package", actual)
		}
		if actual := resourceState.Primary.Attributes["metadata.digest"]; actual != *initialDigest {
			return fmt.Errorf("expected Terraform digest %q, got %q", *initialDigest, actual)
		}
		deployedPackage, err := readFailedPackageState()
		if err != nil {
			return err
		}
		terraformGeneration, err := strconv.Atoi(resourceState.Primary.Attributes["metadata.generation"])
		if err != nil {
			return fmt.Errorf("invalid Terraform metadata.generation %q: %w", resourceState.Primary.Attributes["metadata.generation"], err)
		}
		if terraformGeneration != deployedPackage.Generation {
			return fmt.Errorf("expected Terraform generation %d to match Zarf generation %d", terraformGeneration, deployedPackage.Generation)
		}
		*recoveryGeneration = deployedPackage.Generation
		if deployedPackage.NamespaceOverride != "failed-package" {
			return fmt.Errorf("expected package namespace override %q, got %q", "failed-package", deployedPackage.NamespaceOverride)
		}
		if deployedPackage.Digest != *initialDigest {
			return fmt.Errorf("expected digest %q, got %q", *initialDigest, deployedPackage.Digest)
		}
		if len(deployedPackage.DeployedComponents) != 1 || deployedPackage.DeployedComponents[0].Status != zarfState.ComponentStatusSucceeded {
			return fmt.Errorf("expected succeeded component status, got %#v", deployedPackage.DeployedComponents)
		}

		clientset, err := acceptanceKubernetesClient()
		if err != nil {
			return err
		}
		configMap, err := clientset.CoreV1().ConfigMaps("failed-package").Get(context.Background(), "failed-package", metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get recovered package configmap: %w", err)
		}
		if configMap.Data["status"] != "healthy" {
			return fmt.Errorf("expected recovered package to be healthy, got %#v", configMap.Data)
		}
		return nil
	}
}

func checkSubsequentPackageUpdate(recoveryGeneration *int, initialDigest *string) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		resourceState := state.RootModule().Resources["uds_package.failed"]
		if resourceState == nil || resourceState.Primary == nil {
			return fmt.Errorf("expected package state after subsequent update")
		}
		deployedPackage, err := readFailedPackageState()
		if err != nil {
			return err
		}
		if resourceState.Primary.Attributes["namespace"] != "failed-package" {
			return fmt.Errorf("expected Terraform namespace %q, got %q", "failed-package", resourceState.Primary.Attributes["namespace"])
		}
		if resourceState.Primary.Attributes["metadata.status"] != "Succeeded" {
			return fmt.Errorf("expected Terraform status %q, got %q", "Succeeded", resourceState.Primary.Attributes["metadata.status"])
		}
		terraformGeneration, err := strconv.Atoi(resourceState.Primary.Attributes["metadata.generation"])
		if err != nil {
			return fmt.Errorf("invalid Terraform metadata.generation %q: %w", resourceState.Primary.Attributes["metadata.generation"], err)
		}
		if terraformGeneration != deployedPackage.Generation {
			return fmt.Errorf("expected Terraform generation %d to match Zarf generation %d", terraformGeneration, deployedPackage.Generation)
		}
		if deployedPackage.Generation <= *recoveryGeneration {
			return fmt.Errorf("expected subsequent Zarf generation greater than %d, got %d", *recoveryGeneration, deployedPackage.Generation)
		}
		if terraformGeneration <= *recoveryGeneration {
			return fmt.Errorf("expected subsequent Terraform generation greater than %d, got %d", *recoveryGeneration, terraformGeneration)
		}
		if deployedPackage.Digest != *initialDigest {
			return fmt.Errorf("expected subsequent digest %q, got %q", *initialDigest, deployedPackage.Digest)
		}
		if len(deployedPackage.DeployedComponents) != 1 || deployedPackage.DeployedComponents[0].Status != zarfState.ComponentStatusSucceeded {
			return fmt.Errorf("expected succeeded component status, got %#v", deployedPackage.DeployedComponents)
		}
		clientset, err := acceptanceKubernetesClient()
		if err != nil {
			return err
		}
		configMap, err := clientset.CoreV1().ConfigMaps("failed-package").Get(context.Background(), "failed-package", metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get subsequent package configmap: %w", err)
		}
		if configMap.Data["status"] != "healthy" || configMap.Annotations["diagnostic-value"] != "subsequent-update" {
			return fmt.Errorf("expected healthy subsequent package resources, got data=%#v annotations=%#v", configMap.Data, configMap.Annotations)
		}
		return nil
	}
}

func checkFailedPackageResourcesDestroyed(_ *terraform.State) error {
	clientset, err := acceptanceKubernetesClient()
	if err != nil {
		return err
	}

	_, err = clientset.CoreV1().ConfigMaps("failed-package").Get(context.Background(), "failed-package", metav1.GetOptions{})
	if err == nil {
		return fmt.Errorf("expected failed package ConfigMap to be deleted")
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to check failed package ConfigMap cleanup: %w", err)
	}

	deployedPackageRef := zarfState.DeployedPackage{Name: "failed-package", NamespaceOverride: "failed-package"}
	_, err = clientset.CoreV1().Secrets(zarfState.ZarfNamespaceName).Get(context.Background(), deployedPackageRef.GetSecretName(), metav1.GetOptions{})
	if err == nil {
		return fmt.Errorf("expected failed package state Secret to be deleted")
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to check failed package state Secret cleanup: %w", err)
	}

	return nil
}

func readFailedPackageState() (zarfState.DeployedPackage, error) {
	clientset, err := acceptanceKubernetesClient()
	if err != nil {
		return zarfState.DeployedPackage{}, err
	}
	deployedPackageRef := zarfState.DeployedPackage{Name: "failed-package", NamespaceOverride: "failed-package"}
	secret, err := clientset.CoreV1().Secrets(zarfState.ZarfNamespaceName).Get(context.Background(), deployedPackageRef.GetSecretName(), metav1.GetOptions{})
	if err != nil {
		return zarfState.DeployedPackage{}, fmt.Errorf("failed to get failed package state secret: %w", err)
	}
	var deployedPackage zarfState.DeployedPackage
	if err := json.Unmarshal(secret.Data["data"], &deployedPackage); err != nil {
		return zarfState.DeployedPackage{}, fmt.Errorf("failed to decode failed package state secret: %w", err)
	}
	return deployedPackage, nil
}

func acceptanceKubernetesClient() (*kubernetes.Clientset, error) {
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		kubeconfigPath = clientcmd.RecommendedHomeFile
	}
	kubeClientConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubernetes config: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(kubeClientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}
	return clientset, nil
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
  source       = "oci://ghcr.io/zarf-dev/packages/dos-games:1.3.0"
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
					resource.TestCheckResourceAttrSet("uds_package.dos_games", "metadata.version"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}
