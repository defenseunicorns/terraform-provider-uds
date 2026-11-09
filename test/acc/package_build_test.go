// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package acc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	zPackager "github.com/zarf-dev/zarf/src/pkg/packager"
	zarfState "github.com/zarf-dev/zarf/src/pkg/state"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	udsPackager "github.com/defenseunicorns/terraform-provider-uds/internal/packager"
)

func jitFixture(t *testing.T) string {
	t.Helper()
	source := filepath.Join(t.TempDir(), "package")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(source, os.DirFS("fixtures/jit_package")); err != nil {
		t.Fatal(err)
	}
	return source
}

func jitBuildCount(source string) (int, error) {
	data, err := os.ReadFile(filepath.Join(source, "build-count"))
	if os.IsNotExist(err) {
		return 0, nil
	}
	return strings.Count(string(data), "build\n"), err
}

func renderJITConfig(source, archive, revision, marker string, onPlan bool, timeout string) string {
	input := fmt.Sprintf("build = { source_dir = %q, revision = %q, build_on_plan = %t }", source, revision, onPlan)
	if archive != "" {
		input = fmt.Sprintf("source = %q\nsignature_verification = { verify = false }", archive)
	}
	return fmt.Sprintf(`
provider "uds" { validate_packages_on_plan = false }
resource "uds_package" "init" {
  source = "oci://ghcr.io/zarf-dev/packages/init:%s"
  architecture = %q
  signature_verification = {
    keyless = {
      certificate_identity_regexp = "https://github\\.com/zarf-dev/zarf/\\.github/workflows/release\\.yml@refs/tags/v\\d+\\.\\d+\\.\\d+"
      certificate_oidc_issuer = "https://token.actions.githubusercontent.com"
    }
  }
}
resource "uds_package" "jit" {
  depends_on = [uds_package.init]
  architecture = %q
  %s
  values = { marker = %q }
  timeouts = { create = %q, update = %q }
}
`, initPackageVersion, runtime.GOARCH, runtime.GOARCH, input, marker, timeout, timeout)
}

func checkJITDeployment(source, marker string, generation, builds int) resource.TestCheckFunc {
	return resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckResourceAttr("uds_package.jit", "id", "uds-jit-lifecycle"),
		resource.TestCheckResourceAttr("uds_package.jit", "version", "0.1.0"),
		resource.TestCheckResourceAttr("uds_package.jit", "metadata.generation", strconv.Itoa(generation)),
		func(_ *terraform.State) error {
			client, err := acceptanceKubernetesClient()
			if err != nil {
				return err
			}
			cm, err := client.CoreV1().ConfigMaps("default").Get(context.Background(), "uds-jit-marker", metav1.GetOptions{})
			if err != nil {
				return err
			}
			if cm.Data["marker"] != marker {
				return fmt.Errorf("deployed marker = %q, want %q", cm.Data["marker"], marker)
			}
			if builds >= 0 {
				count, err := jitBuildCount(source)
				if err != nil {
					return err
				}
				if count != builds {
					return fmt.Errorf("build count = %d, want %d", count, builds)
				}
			}
			archives, err := filepath.Glob(filepath.Join(source, "*.tar*"))
			if err != nil {
				return err
			}
			if len(archives) != 0 {
				return fmt.Errorf("unexpected source-directory archives: %v", archives)
			}
			return nil
		},
	)
}

func TestAccPackageResourceJITBuildLifecycle(t *testing.T) {
	if os.Getenv(resource.EnvTfAcc) == "" {
		t.Skip("Acceptance tests skipped unless TF_ACC=1")
	}
	source := jitFixture(t)
	archive, err := udsPackager.NewPackager().Create(context.Background(), source, t.TempDir(), zPackager.CreateOptions{Architecture: runtime.GOARCH, CachePath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(source, "build-count")); err != nil {
		t.Fatal(err)
	}
	config := func(archive, revision, marker, timeout string) string {
		return renderJITConfig(source, archive, revision, marker, false, timeout)
	}
	moved := source + "-removed"
	finalBuildCount := 5
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(_ *terraform.State) error {
			client, err := acceptanceKubernetesClient()
			if err != nil {
				return err
			}
			_, err = client.CoreV1().ConfigMaps("default").Get(context.Background(), "uds-jit-marker", metav1.GetOptions{})
			if !apierrors.IsNotFound(err) {
				return fmt.Errorf("expected marker removal, got %v", err)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{Config: config("", "manual-1", "initial", "2m"), Check: checkJITDeployment(source, "initial", 1, 1)},
			{Config: config("", "manual-1", "initial", "2m"), PlanOnly: true},
			{Config: config("", "manual-2", "initial", "2m"), Check: checkJITDeployment(source, "initial", 2, 2)},
			{Config: config("", "manual-2", "updated", "2m"), Check: checkJITDeployment(source, "updated", 3, 3)},
			{Config: config("", "manual-2", "updated", "3m"), Check: checkJITDeployment(source, "updated", 3, 3)},
			{Config: config(archive, "", "updated", "3m"), Check: checkJITDeployment(source, "updated", 4, 3)},
			{Config: config("", "manual-2", "updated", "3m"), Check: checkJITDeployment(source, "updated", 5, 4)},
			{
				PreConfig: func() {
					client, err := acceptanceKubernetesClient()
					if err != nil {
						t.Fatal(err)
					}
					deployed := zarfState.DeployedPackage{Name: "uds-jit-lifecycle"}
					if err := client.CoreV1().Secrets(zarfState.ZarfNamespaceName).Delete(context.Background(), deployed.GetSecretName(), metav1.DeleteOptions{}); err != nil {
						t.Fatal(err)
					}
					if err := client.CoreV1().ConfigMaps("default").Delete(context.Background(), "uds-jit-marker", metav1.DeleteOptions{}); err != nil {
						t.Fatal(err)
					}
				},
				RefreshState: true, ExpectNonEmptyPlan: true,
				Check: func(state *terraform.State) error {
					if _, ok := state.RootModule().Resources["uds_package.jit"]; ok {
						return fmt.Errorf("confirmed absence must remove the resource from refreshed state")
					}
					count, err := jitBuildCount(source)
					if err != nil {
						return err
					}
					if count != 4 {
						return fmt.Errorf("refresh built %d packages, want unchanged count 4", count)
					}
					return nil
				},
			},
			{Config: config("", "manual-2", "updated", "3m"), Check: checkJITDeployment(source, "updated", 1, 5)},
			{
				Config: renderJITConfig(source, "", "manual-2", "updated", true, "3m"),
				Check:  checkJITDeployment(source, "updated", 1, -1),
				PostApplyFunc: func() {
					count, err := jitBuildCount(source)
					if err != nil {
						t.Fatal(err)
					}
					if count <= finalBuildCount {
						t.Fatal("policy opt-in did not validate during normal planning")
					}
					finalBuildCount = count
				},
			},
			{
				Config: config("", "manual-2", "updated", "3m"),
				Check: func(state *terraform.State) error {
					return checkJITDeployment(source, "updated", 1, finalBuildCount)(state)
				},
			},
			{
				PreConfig: func() {
					if err := os.Rename(source, moved); err != nil {
						t.Fatal(err)
					}
				},
				Config: config("", "manual-2", "updated", "3m"), Destroy: true,
				Check: func(_ *terraform.State) error {
					count, err := jitBuildCount(moved)
					if err != nil {
						return err
					}
					if count != finalBuildCount {
						return fmt.Errorf("destroy unexpectedly built: count %d", count)
					}
					return nil
				},
			},
		},
	})
}

// jitPlanCheck observes action side effects at framework-managed CLI boundaries.
// It does not execute the CLI or manage a provider process itself.
type jitPlanCheck struct{ check func() error }

func (c jitPlanCheck) CheckPlan(_ context.Context, _ plancheck.CheckPlanRequest, resp *plancheck.CheckPlanResponse) {
	resp.Error = c.check()
}

func TestAccPackageResourceJITPlanValidation(t *testing.T) {
	if os.Getenv(resource.EnvTfAcc) == "" {
		t.Skip("Acceptance tests skipped unless TF_ACC=1")
	}
	// Ask the existing acceptance framework to exercise refresh-only apply too.
	t.Setenv(resource.EnvTfAccRefreshAfterApply, "1")
	source := jitFixture(t)
	config := renderJITConfig(source, "", "constant", "initial", true, "3m")
	originalDefinition, err := os.ReadFile(filepath.Join(source, "zarf.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	beforeApply, afterFrameworkPlans := 0, 0
	readCount := func() int {
		t.Helper()
		count, err := jitBuildCount(source)
		if err != nil {
			t.Fatal(err)
		}
		return count
	}
	noOpStep := resource.TestStep{
		Config: config,
		ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
			plancheck.ExpectEmptyPlan(),
			jitPlanCheck{check: func() error { beforeApply = readCount(); return nil }},
		}},
		Check: resource.ComposeAggregateTestCheckFunc(checkJITDeployment(source, "initial", 1, -1), func(_ *terraform.State) error {
			if got := readCount(); got != beforeApply {
				return fmt.Errorf("saved no-op apply performed a build: before %d, after %d", beforeApply, got)
			}
			return nil
		}),
		PostApplyFunc: func() { afterFrameworkPlans = readCount() },
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config, Check: checkJITDeployment(source, "initial", 1, -1)},
			noOpStep,
			{
				PreConfig: func() {
					if got := readCount(); got != afterFrameworkPlans {
						t.Fatalf("framework refresh-only apply built: before %d, after %d", afterFrameworkPlans, got)
					}
					file := filepath.Join(source, "zarf.yaml")
					data, err := os.ReadFile(file)
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(file, append(data, []byte("\n# content edit without a revision bump\n")...), 0o600); err != nil {
						t.Fatal(err)
					}
				},
				Config: config, PlanOnly: true,
				ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPreRefresh: []plancheck.PlanCheck{
					plancheck.ExpectEmptyPlan(), jitPlanCheck{check: func() error {
						if readCount() <= afterFrameworkPlans {
							return fmt.Errorf("unchanged normal plan did not build")
						}
						return nil
					}},
				}},
			},
			{
				PreConfig: func() {
					file := filepath.Join(source, "zarf.yaml")
					data, err := os.ReadFile(file)
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(file, []byte(strings.Replace(string(data), "echo build >> build-count", "uds-jit-deliberately-unavailable-command-8e4937", 1)), 0o600); err != nil {
						t.Fatal(err)
					}
				},
				Config: config, PlanOnly: true, ExpectError: regexp.MustCompile("uds-jit-deliberately-unavailable-command-8e4937"),
			},
			{
				PreConfig: func() {
					if err := os.WriteFile(filepath.Join(source, "zarf.yaml"), originalDefinition, 0o600); err != nil {
						t.Fatal(err)
					}
				},
				Config:           config,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
				Check:            checkJITDeployment(source, "initial", 1, -1),
			},
		},
	})
}
