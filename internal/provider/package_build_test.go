// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/defaults"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	zarfAPI "github.com/zarf-dev/zarf/src/api"
	zarfCluster "github.com/zarf-dev/zarf/src/pkg/cluster"
	zarfPackager "github.com/zarf-dev/zarf/src/pkg/packager"
	zarfState "github.com/zarf-dev/zarf/src/pkg/state"
	zarfValue "github.com/zarf-dev/zarf/src/pkg/value"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	udsPackager "github.com/defenseunicorns/terraform-provider-uds/internal/packager"
)

func testBuildObject(dir, revision types.String) types.Object {
	return types.ObjectValueMust(packageBuildAttrTypes, map[string]attr.Value{
		"source_dir": dir, "revision": revision, "build_on_plan": types.BoolValue(false),
	})
}

func TestPackageBuildInputValidation(t *testing.T) {
	build := testBuildObject(types.StringValue("./package"), types.StringValue("constant"))
	for _, tc := range []struct {
		name    string
		source  types.String
		build   types.Object
		invalid bool
	}{
		{"source", types.StringValue("./package.tar.zst"), types.ObjectNull(packageBuildAttrTypes), false},
		{"build constant revision", types.StringNull(), build, false},
		{"both", types.StringValue("./package.tar.zst"), build, true},
		{"neither", types.StringNull(), types.ObjectNull(packageBuildAttrTypes), true},
		{"unknown source", types.StringUnknown(), types.ObjectNull(packageBuildAttrTypes), false},
		{"unknown build", types.StringNull(), types.ObjectUnknown(packageBuildAttrTypes), false},
		{"both unknown", types.StringUnknown(), types.ObjectUnknown(packageBuildAttrTypes), false},
		{"known source unknown build", types.StringValue("./package.tar.zst"), types.ObjectUnknown(packageBuildAttrTypes), false},
		{"unknown source known build", types.StringUnknown(), build, false},
		{"unknown leaves", types.StringNull(), testBuildObject(types.StringUnknown(), types.StringUnknown()), false},
		{"missing directory", types.StringNull(), testBuildObject(types.StringNull(), types.StringValue("v1")), true},
		{"missing revision", types.StringNull(), testBuildObject(types.StringValue("./package"), types.StringNull()), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var resp resource.ValidateConfigResponse
			validatePackageInput(PackageResourceModel{Source: tc.source, Build: tc.build}, &resp)
			require.Equal(t, tc.invalid, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
		})
	}
}

func TestPackageBuildSchema(t *testing.T) {
	ctx := context.Background()
	var resp resource.SchemaResponse
	NewPackageResource(nil, nil, nil, nil).Schema(ctx, resource.SchemaRequest{}, &resp)
	require.False(t, resp.Diagnostics.HasError())
	require.False(t, resp.Schema.ValidateImplementation(ctx).HasError())
	require.True(t, resp.Schema.Attributes["source"].IsOptional())
	require.False(t, resp.Schema.Attributes["source"].IsComputed())
	build := resp.Schema.Attributes["build"].(schema.SingleNestedAttribute)
	require.True(t, build.IsOptional())
	for _, name := range []string{"source_dir", "revision"} {
		require.True(t, build.Attributes[name].IsRequired())
	}
	policy := build.Attributes["build_on_plan"].(schema.BoolAttribute)
	var defaultResp defaults.BoolResponse
	policy.Default.DefaultBool(ctx, defaults.BoolRequest{}, &defaultResp)
	require.Equal(t, types.BoolValue(false), defaultResp.PlanValue)
}

func TestPackageBuildSignatureConfiguration(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name      string
		signature types.Object
		invalid   bool
	}{
		{"omitted", types.ObjectNull(signatureVerificationAttrTypes), false},
		{"unknown", types.ObjectUnknown(signatureVerificationAttrTypes), false},
		{"empty", types.ObjectValueMust(signatureVerificationAttrTypes, map[string]attr.Value{
			"verify": types.BoolNull(), "public_key": types.StringNull(), "keyless": types.ObjectNull(keylessVerificationAttrTypes),
		}), true},
		{"disabled", NewTestPackageResourceModel(WithSignatureVerificationEnabled(false)).SignatureVerification, true},
		{"public key", NewTestPackageResourceModel(WithPublicKey("test-key")).SignatureVerification, true},
		{"keyless", NewTestPackageResourceModel(WithKeylessVerification("identity", "issuer")).SignatureVerification, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := PackageResourceModel{
				Build:                 testBuildObject(types.StringValue("./package"), types.StringValue("manual")),
				SignatureVerification: tc.signature,
			}
			var resp resource.ValidateConfigResponse
			validateSignatureVerificationAttributes(ctx, model, &resp)
			require.Equal(t, tc.invalid, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
			// A defaulted signature object remains untouched in plan/state but
			// must never enable verification for the unsigned built archive.
			model.SignatureVerification = defaultSignatureVerification
			require.False(t, getEffectiveSignatureVerification(ctx, model))
			require.Equal(t, defaultSignatureVerification, model.SignatureVerification)
		})
	}
}

func TestPackageBuildUnknownModeSignatureConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name      string
		signature types.Object
	}{
		{"unknown", types.ObjectUnknown(signatureVerificationAttrTypes)},
		{"omitted", types.ObjectNull(signatureVerificationAttrTypes)},
		{"disabled", NewTestPackageResourceModel(WithSignatureVerificationEnabled(false)).SignatureVerification},
		{"public key", NewTestPackageResourceModel(WithPublicKey("test-key")).SignatureVerification},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var resp resource.ValidateConfigResponse
			validateSignatureVerificationAttributes(context.Background(), PackageResourceModel{
				Build:                 types.ObjectUnknown(packageBuildAttrTypes),
				SignatureVerification: tc.signature,
			}, &resp)
			require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
		})
	}
}

func TestPackageBuildHistoricalState(t *testing.T) {
	ctx := context.Background()
	r := NewPackageResource(nil, nil, nil, nil).(*PackageResource)
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	// Historical JSON state has every old attribute, but no build attribute.
	historical := map[string]any{}
	for name := range schemaResp.Schema.Attributes {
		if name != "build" {
			historical[name] = nil
		}
	}
	historical["component"] = []any{}
	historical["source"] = "./historical.tar.zst"
	historical["id"] = "historical"
	raw, err := json.Marshal(historical)
	require.NoError(t, err)
	server := providerserver.NewProtocol6(New("test")())()
	upgraded, err := server.UpgradeResourceState(ctx, &tfprotov6.UpgradeResourceStateRequest{
		TypeName: "uds_package", Version: 0, RawState: &tfprotov6.RawState{JSON: raw},
	})
	require.NoError(t, err)
	require.Empty(t, upgraded.Diagnostics)
	value, err := upgraded.UpgradedState.Unmarshal(schemaResp.Schema.Type().TerraformType(ctx))
	require.NoError(t, err)
	state := tfsdk.State{Schema: schemaResp.Schema, Raw: value}
	var model PackageResourceModel
	require.False(t, state.Get(ctx, &model).HasError())
	require.True(t, model.Build.IsNull())
	require.Equal(t, types.StringValue("./historical.tar.zst"), model.Source)
}

func TestPackageBuildStateRoundTrip(t *testing.T) {
	ctx := context.Background()
	r := NewPackageResource(nil, nil, nil, nil).(*PackageResource)
	model := NewTestPackageResourceModel()
	model.Metadata = types.ObjectNull(packageMetadataAttrTypes)
	model.ConnectStrings = types.SetNull(types.ObjectType{AttrTypes: connectStringAttrTypes})
	model.SetVariables = types.MapNull(types.StringType)
	model.Source = types.StringNull()
	model.Build = testBuildObject(types.StringValue("../relative/package"), types.StringValue("constant"))
	model.SignatureVerification = defaultSignatureVerification
	plan := buildTestPlan(t, r, model)
	var planned PackageResourceModel
	require.False(t, plan.Get(ctx, &planned).HasError())
	state := buildTestState(t, r, planned)
	var stored PackageResourceModel
	require.False(t, state.Get(ctx, &stored).HasError())
	require.True(t, stored.Source.IsNull())
	require.Equal(t, model.Build, stored.Build)
	require.Equal(t, model.SignatureVerification, stored.SignatureVerification)
}

func TestPackageBuildEffectiveOptions(t *testing.T) {
	r := NewPackageResource(&udsProviderConfig{DefaultArchitecture: "arm64", ZarfCachePath: "cache", InsecureForceHTTP: true, InsecureSkipTLSVerification: true}, nil, nil, nil).(*PackageResource)
	for _, arch := range []types.String{types.StringNull(), types.StringValue("amd64")} {
		model := NewTestPackageResourceModel()
		model.Build = testBuildObject(types.StringValue("./package"), types.StringValue("manual"))
		model.Architecture = arch
		input, err := r.packageBuildInput(context.Background(), model)
		require.NoError(t, err)
		want := "arm64"
		if !arch.IsNull() {
			want = "amd64"
		}
		require.Equal(t, want, input.Architecture)
		require.Equal(t, "cache", input.CachePath)
		require.True(t, input.PlainHTTP)
		require.True(t, input.InsecureSkipTLSVerify)
	}
}

func TestPackageBuildCreate(t *testing.T) {
	for _, scenario := range []string{"success", "plan then apply", "build failure", "duplicate"} {
		t.Run(scenario, func(t *testing.T) {
			pkg := betaLifecycleTestPackage()
			pkgLayout := newTestPackageLayout(t, pkg)
			deployed := newLifecycleDeployedPackage(t, pkgLayout)
			clientset := fake.NewSimpleClientset()
			if scenario == "duplicate" {
				_, err := clientset.CoreV1().Secrets(zarfState.ZarfNamespaceName).Create(context.Background(), newPackageStateSecret(t, deployed), metav1.CreateOptions{})
				require.NoError(t, err)
			}
			cluster := &MockCluster{}
			p := &MockPackager{}
			model := newCreateLifecycleModel()
			model.Source = types.StringNull()
			model.Build = testBuildObject(types.StringValue(t.TempDir()), types.StringValue("opaque-manual"))
			model.SignatureVerification = defaultSignatureVerification
			success := scenario == "success" || scenario == "plan then apply"
			builds := 1
			if scenario == "plan then apply" {
				builds = 2
				attrs := model.Build.Attributes()
				attrs["build_on_plan"] = types.BoolValue(true)
				model.Build = types.ObjectValueMust(packageBuildAttrTypes, attrs)
				p.On("LoadPackage", mock.Anything, "returned-package.tar.zst", mock.Anything).Return(newTestPackageLayout(t, pkg), nil).Once()
			}
			var output string
			var buildErr error
			if scenario == "build failure" {
				buildErr = errors.New("build failed")
			}
			p.On("Create", mock.Anything, model.Build.Attributes()["source_dir"].(types.String).ValueString(), mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
				output = args.String(2)
				opts := args.Get(3).(zarfPackager.CreateOptions)
				require.Nil(t, opts.SignBlobOptions)
				require.Empty(t, opts.SetVariables)
			}).Return("returned-package.tar.zst", buildErr).Times(builds)
			if buildErr == nil {
				p.On("LoadPackage", mock.Anything, "returned-package.tar.zst", mock.Anything).Return(pkgLayout, nil).Once()
				cluster.On("NewWithWait", mock.Anything).Return(&zarfCluster.Cluster{Clientset: clientset}, nil)
			}
			if success {
				p.On("Deploy", mock.Anything, pkgLayout, mock.Anything).Return(zarfPackager.DeployResult{}, nil).Once()
			}
			r := NewPackageResource(nil, p, nil, cluster).(*PackageResource)
			if scenario == "plan then apply" {
				plan := buildTestPlan(t, r, model)
				resp := resource.ModifyPlanResponse{Plan: plan}
				r.ModifyPlan(context.Background(), resource.ModifyPlanRequest{Config: buildTestConfig(t, r, model), Plan: plan}, &resp)
				require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
				require.NoDirExists(t, output)
			}
			resp := runCreateLifecycleTest(t, r, model)
			require.Equal(t, !success, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
			if success {
				stored := requirePackageState(t, resp.State)
				require.Equal(t, model.Build, stored.Build)
				require.True(t, stored.Source.IsNull())
				require.Equal(t, defaultSignatureVerification, stored.SignatureVerification)
			} else {
				p.AssertNotCalled(t, "Deploy", mock.Anything, mock.Anything, mock.Anything)
			}
			p.AssertNotCalled(t, "Remove", mock.Anything, mock.Anything, mock.Anything)
			require.NoDirExists(t, output)
			p.AssertExpectations(t)
		})
	}
}

func TestPackageBuildUpdateIdentityAndTransitions(t *testing.T) {
	for _, scenario := range []string{"build update", "source to build", "build to source", "name mismatch", "identity failure"} {
		t.Run(scenario, func(t *testing.T) {
			pkg := betaLifecycleTestPackage()
			oldLayout := newTestPackageLayout(t, pkg)
			existing := newLifecycleDeployedPackage(t, oldLayout, zarfState.DeployedComponent{Name: "required"}, zarfState.DeployedComponent{Name: "optional"})
			if scenario == "name mismatch" {
				pkg.Metadata.Name = "different-name"
			}
			newLayout := newTestPackageLayout(t, pkg)
			clientset := fake.NewSimpleClientset(newPackageStateSecret(t, existing))
			cluster := &MockCluster{}
			p := &MockPackager{}
			old := NewTestPackageResourceModel(WithDeployedState(), WithOptionalComponents([]string{"optional"}))
			old.ID = types.StringValue(existing.Name)
			old.Name = types.StringValue(existing.Name)
			if scenario != "source to build" {
				old.Source = types.StringNull()
				old.Build = testBuildObject(types.StringValue(t.TempDir()), types.StringValue("old"))
				old.SignatureVerification = defaultSignatureVerification
			}
			plan := old
			WithOptionalComponents([]string{})(&plan)
			if scenario == "build to source" {
				plan.Build = types.ObjectNull(packageBuildAttrTypes)
				plan.Source = types.StringValue("oci://example.test/packages/source:1")
				WithSignatureVerificationEnabled(false)(&plan)
			} else {
				plan.Source = types.StringNull()
				plan.Build = testBuildObject(types.StringValue(t.TempDir()), types.StringValue("new"))
				plan.SignatureVerification = defaultSignatureVerification
			}
			if scenario == "identity failure" {
				cluster.On("NewWithWait", mock.Anything).Return((*zarfCluster.Cluster)(nil), errors.New("lookup failed")).Once()
			} else {
				cluster.On("NewWithWait", mock.Anything).Return(&zarfCluster.Cluster{Clientset: clientset}, nil)
				if scenario == "build to source" {
					p.On("LoadPackage", mock.Anything, mock.Anything, mock.Anything).Return(newLayout, nil).Twice()
				} else {
					p.On("Create", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("built.tar.zst", nil).Once()
					p.On("LoadPackage", mock.Anything, "built.tar.zst", mock.Anything).Return(newLayout, nil).Once()
				}
			}
			valid := scenario != "name mismatch" && scenario != "identity failure"
			if valid {
				p.On("Remove", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
					definition := args.Get(1).(zarfAPI.Package)
					require.Equal(t, pkg.Metadata.Name, definition.Metadata.Name)
					require.Equal(t, "optional", definition.Components[0].Name)
				}).Return(nil).Once()
				p.On("Deploy", mock.Anything, newLayout, mock.Anything).Return(zarfPackager.DeployResult{}, nil).Once()
			}
			r := NewPackageResource(nil, p, nil, cluster).(*PackageResource)
			resp := runUpdateLifecycleTest(t, r, plan, old)
			require.Equal(t, !valid, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
			if valid {
				stored := requirePackageState(t, resp.State)
				require.Equal(t, old.ID, stored.ID)
				require.Equal(t, plan.Build, stored.Build)
				require.Equal(t, plan.Source, stored.Source)
			} else {
				p.AssertNotCalled(t, "Remove", mock.Anything, mock.Anything, mock.Anything)
				p.AssertNotCalled(t, "Deploy", mock.Anything, mock.Anything, mock.Anything)
			}
			p.AssertExpectations(t)
		})
	}
}

func TestPackageBuildStateOnlyChanges(t *testing.T) {
	for _, change := range []string{"policy", "timeout", "both", "revision", "directory", "architecture", "values", "mixed", "unknown revision"} {
		t.Run(change, func(t *testing.T) {
			r := NewPackageResource(nil, &MockPackager{}, nil, &MockCluster{}).(*PackageResource)
			old := NewTestPackageResourceModel(WithDeployedState())
			old.Source = types.StringNull()
			old.Build = testBuildObject(types.StringValue("missing-directory"), types.StringValue("old"))
			plan := old
			attrs := plan.Build.Attributes()
			if change == "policy" || change == "both" || change == "mixed" {
				attrs["build_on_plan"] = types.BoolValue(true)
			}
			if change == "revision" || change == "mixed" {
				attrs["revision"] = types.StringValue("new")
			}
			if change == "unknown revision" {
				attrs["revision"] = types.StringUnknown()
			}
			if change == "directory" {
				attrs["source_dir"] = types.StringValue("other")
			}
			if change == "architecture" {
				plan.Architecture = types.StringValue("arm64")
			}
			if change == "values" {
				WithValues(testDynamicStringValues(map[string]string{"domain": "new"}))(&plan)
			}
			if change == "timeout" || change == "both" {
				WithTimeout("20m")(&plan)
			}
			plan.Build = types.ObjectValueMust(packageBuildAttrTypes, attrs)
			stateOnly, err := isStateOnlyUpdate(buildTestPlan(t, r, plan), buildTestState(t, r, old))
			require.NoError(t, err)
			want := change == "policy" || change == "timeout" || change == "both"
			require.Equal(t, want, stateOnly)
			if want {
				resp := runUpdateLifecycleTest(t, r, plan, old)
				require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
				stored := requirePackageState(t, resp.State)
				require.Equal(t, plan.Build, stored.Build)
				require.Equal(t, plan.Timeouts, stored.Timeouts)
				require.Equal(t, old.Metadata, stored.Metadata)
			}
		})
	}
}

func TestPackageBuildPlanPolicy(t *testing.T) {
	for _, scenario := range []string{"omitted", "false provider enabled", "true provider disabled", "destroy", "unknown directory", "unknown revision", "unknown build", "unchanged", "state only"} {
		t.Run(scenario, func(t *testing.T) {
			p := &MockPackager{}
			r := NewPackageResource(&udsProviderConfig{ValidatePackagesOnPlan: scenario == "false provider enabled", DefaultArchitecture: "amd64"}, p, nil, &MockCluster{}).(*PackageResource)
			model := NewTestPackageResourceModel(WithDeployedState(), WithOptionalComponents([]string{}))
			model.Source = types.StringNull()
			model.SignatureVerification = defaultSignatureVerification
			model.Build = testBuildObject(types.StringValue(t.TempDir()), types.StringValue("constant"))
			attrs := model.Build.Attributes()
			attrs["build_on_plan"] = types.BoolValue(scenario != "omitted" && scenario != "false provider enabled")
			if scenario == "unknown directory" {
				attrs["source_dir"] = types.StringUnknown()
			}
			if scenario == "unknown revision" {
				attrs["revision"] = types.StringUnknown()
			}
			model.Build = types.ObjectValueMust(packageBuildAttrTypes, attrs)
			if scenario == "unknown build" {
				model.Build = types.ObjectUnknown(packageBuildAttrTypes)
			}
			plan := buildTestPlan(t, r, model)
			state := tfsdk.State{Schema: plan.Schema}
			if scenario == "unchanged" || scenario == "state only" {
				state = buildTestState(t, r, model)
				if scenario == "state only" {
					WithTimeout("45m")(&model)
					plan = buildTestPlan(t, r, model)
				}
			}
			if scenario == "destroy" {
				plan.Raw = tftypes.NewValue(plan.Raw.Type(), nil)
			}
			wantBuild := scenario == "true provider disabled" || scenario == "unchanged" || scenario == "state only"
			var output string
			if wantBuild {
				pkg := betaLifecycleTestPackage()
				pkg.Metadata.Name = "test-pkg"
				pkgLayout := newTestPackageLayout(t, pkg)
				p.On("Create", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) { output = args.String(2) }).Return("plan-built.tar.zst", nil).Once()
				p.On("LoadPackage", mock.Anything, "plan-built.tar.zst", mock.Anything).Return(pkgLayout, nil).Once()
			}
			resp := resource.ModifyPlanResponse{Plan: plan}
			r.ModifyPlan(context.Background(), resource.ModifyPlanRequest{Config: buildTestConfig(t, r, model), Plan: plan, State: state}, &resp)
			require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
			if wantBuild {
				require.NoDirExists(t, output)
				stored := requirePackageState(t, tfsdk.State{Schema: resp.Plan.Schema, Raw: resp.Plan.Raw})
				require.Equal(t, model.Metadata, stored.Metadata)
				require.Equal(t, model.Build, stored.Build)
			}
			p.AssertExpectations(t)
		})
	}
}

type observedBuildPackager struct {
	udsPackager.Packager

	outputDir string
}

func (p *observedBuildPackager) Create(ctx context.Context, dir, output string, opts zarfPackager.CreateOptions) (string, error) {
	p.outputDir = output
	return p.Packager.Create(ctx, dir, output, opts)
}

func TestPackageBuildMissingCommandBlocksPlanAndApply(t *testing.T) {
	for _, phase := range []string{"plan", "apply"} {
		t.Run(phase, func(t *testing.T) {
			_, err := exec.LookPath("uds-jit-deliberately-unavailable-command-8e4937")
			require.Error(t, err)
			p := &observedBuildPackager{Packager: udsPackager.NewPackager()}
			cluster := &MockCluster{}
			r := NewPackageResource(&udsProviderConfig{DefaultArchitecture: "amd64", ZarfCachePath: t.TempDir()}, p, nil, cluster).(*PackageResource)
			model := newCreateLifecycleModel()
			model.Source = types.StringNull()
			attrs := testBuildObject(types.StringValue("../packager/testdata/missing-command"), types.StringValue("manual")).Attributes()
			attrs["build_on_plan"] = types.BoolValue(true)
			model.Build = types.ObjectValueMust(packageBuildAttrTypes, attrs)
			model.SignatureVerification = defaultSignatureVerification
			if phase == "plan" {
				plan := buildTestPlan(t, r, model)
				resp := resource.ModifyPlanResponse{Plan: plan}
				r.ModifyPlan(context.Background(), resource.ModifyPlanRequest{Config: buildTestConfig(t, r, model), Plan: plan}, &resp)
				require.True(t, resp.Diagnostics.HasError())
				require.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "uds-jit-deliberately-unavailable-command-8e4937")
			} else {
				resp := runCreateLifecycleTest(t, r, model)
				require.True(t, resp.Diagnostics.HasError())
				require.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "uds-jit-deliberately-unavailable-command-8e4937")
			}
			require.NotEmpty(t, p.outputDir)
			require.NoDirExists(t, p.outputDir)
			require.Empty(t, cluster.Calls, "build failure must not reach cluster preflight or mutation")
			_, err = exec.LookPath("uds-jit-deliberately-unavailable-command-8e4937")
			require.Error(t, err)
			archives, err := filepath.Glob("../packager/testdata/missing-command/*.tar*")
			require.NoError(t, err)
			require.Empty(t, archives)
		})
	}
}

func TestPackageBuildRefreshAndDeleteWithoutDirectory(t *testing.T) {
	for _, scenario := range []string{"read", "delete", "absent", "lookup failure", "identity conflict"} {
		t.Run(scenario, func(t *testing.T) {
			pkg := betaLifecycleTestPackage()
			pkgLayout := newTestPackageLayout(t, pkg)
			deployed := newLifecycleDeployedPackage(t, pkgLayout)
			if scenario == "identity conflict" {
				deployed.Name = "wrong-name"
			}
			clientset := fake.NewSimpleClientset()
			if scenario != "absent" {
				secret := newPackageStateSecret(t, deployed)
				// Keep the lookup key correct while corrupting the record identity.
				canonical := newLifecycleDeployedPackage(t, pkgLayout)
				secret.Name = canonical.GetSecretName()
				_, err := clientset.CoreV1().Secrets(zarfState.ZarfNamespaceName).Create(context.Background(), secret, metav1.CreateOptions{})
				require.NoError(t, err)
			}
			cluster := &MockCluster{}
			if scenario == "lookup failure" {
				cluster.On("NewWithWait", mock.Anything).Return((*zarfCluster.Cluster)(nil), errors.New("lookup failed"))
			} else {
				cluster.On("NewWithWait", mock.Anything).Return(&zarfCluster.Cluster{Clientset: clientset}, nil)
			}
			p := &MockPackager{}
			model := NewTestPackageResourceModel(WithDeployedState())
			model.ID = types.StringValue(pkg.Metadata.Name)
			model.Name = types.StringValue(pkg.Metadata.Name)
			model.Source = types.StringNull()
			model.Build = testBuildObject(types.StringValue(filepath.Join(t.TempDir(), "does-not-exist")), types.StringValue("constant"))
			WithValues(testDynamicStringValues(map[string]string{"domain": "example.test"}))(&model)
			WithSensitiveValues(testDynamicStringValues(map[string]string{"token": "secret"}))(&model)
			r := NewPackageResource(nil, p, nil, cluster).(*PackageResource)
			state := buildTestState(t, r, model)
			if scenario == "delete" {
				p.On("Remove", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
					require.Equal(t, zarfValue.Values{"domain": "example.test", "token": "secret"}, args.Get(2).(zarfPackager.RemoveOptions).Values)
				}).Return(nil).Once()
				var resp resource.DeleteResponse
				r.Delete(context.Background(), resource.DeleteRequest{State: state}, &resp)
				require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
			} else {
				resp := resource.ReadResponse{State: state}
				r.Read(context.Background(), resource.ReadRequest{State: state}, &resp)
				require.Equal(t, scenario == "lookup failure" || scenario == "identity conflict", resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
				if scenario == "absent" {
					require.True(t, resp.State.Raw.IsNull())
				} else {
					stored := requirePackageState(t, resp.State)
					require.Equal(t, model.Build, stored.Build)
					require.True(t, stored.Source.IsNull())
					if resp.Diagnostics.HasError() {
						require.True(t, state.Raw.Equal(resp.State.Raw))
					}
				}
			}
			p.AssertNotCalled(t, "Create", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			p.AssertNotCalled(t, "Deploy", mock.Anything, mock.Anything, mock.Anything)
			p.AssertExpectations(t)
		})
	}
}

func TestPackageBuildFailedUpdatePreservesRevision(t *testing.T) {
	pkg := betaLifecycleTestPackage()
	pkgLayout := newTestPackageLayout(t, pkg)
	deployed := newLifecycleDeployedPackage(t, pkgLayout)
	clientset := fake.NewSimpleClientset(newPackageStateSecret(t, deployed))
	cluster := &MockCluster{}
	cluster.On("NewWithWait", mock.Anything).Return(&zarfCluster.Cluster{Clientset: clientset}, nil)
	p := &MockPackager{}
	p.On("Create", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", errors.New("build failed")).Once()
	r := NewPackageResource(nil, p, nil, cluster).(*PackageResource)
	model := NewTestPackageResourceModel(WithDeployedState())
	model.ID = types.StringValue(pkg.Metadata.Name)
	model.Name = types.StringValue(pkg.Metadata.Name)
	model.Source = types.StringNull()
	model.Build = testBuildObject(types.StringValue(t.TempDir()), types.StringValue("old"))
	state := buildTestState(t, r, model)
	model.Build = testBuildObject(model.Build.Attributes()["source_dir"].(types.String), types.StringValue("new"))
	resp := resource.UpdateResponse{State: state}
	r.Update(context.Background(), resource.UpdateRequest{Plan: buildTestPlan(t, r, model), State: state}, &resp)
	require.True(t, resp.Diagnostics.HasError())
	require.True(t, state.Raw.Equal(resp.State.Raw))
	p.AssertExpectations(t)
}

func TestPackageBuildImportDoesNotInferBuildInputs(t *testing.T) {
	r := NewPackageResource(nil, &MockPackager{}, nil, &MockCluster{}).(*PackageResource)
	var schema resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schema)
	resp := resource.ImportStateResponse{State: tfsdk.State{Schema: schema.Schema}}
	resp.State.Raw = tftypes.NewValue(schema.Schema.Type().TerraformType(context.Background()), nil)
	r.ImportState(context.Background(), resource.ImportStateRequest{ID: "package-name"}, &resp)
	require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
	stored := requirePackageState(t, resp.State)
	require.Equal(t, types.StringValue("package-name"), stored.ID)
	require.True(t, stored.Build.IsNull())
	require.True(t, stored.Source.IsNull())
}

func TestPackageBuildPlanValidation(t *testing.T) {
	for _, scenario := range []string{"valid", "invalid package", "optional", "invalid values", "partial values", "invalid known with unknown leaf", "beta vars", "beta sensitive vars", "alpha vars", "name mismatch"} {
		t.Run(scenario, func(t *testing.T) {
			source := t.TempDir()
			version := "v1beta1"
			if scenario == "alpha vars" {
				version = "v1alpha1"
			}
			definition := fmt.Sprintf("apiVersion: zarf.dev/%s\nkind: ZarfPackageConfig\nmetadata:\n  name: plan-validation\nvalues:\n  files: [values.yaml]\n  schema: values.schema.json\ncomponents:\n  - name: empty\n", version)
			if scenario == "invalid package" {
				definition = "invalid: ["
			}
			require.NoError(t, os.WriteFile(filepath.Join(source, "zarf.yaml"), []byte(definition), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(source, "values.yaml"), []byte("domain: example.test\ncount: 1\n"), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(source, "values.schema.json"), []byte(`{"type":"object","properties":{"domain":{"type":"string"},"count":{"type":"integer"}},"additionalProperties":false}`), 0o600))
			r := NewPackageResource(&udsProviderConfig{ZarfCachePath: t.TempDir()}, nil, nil, &MockCluster{}).(*PackageResource)
			model := NewTestPackageResourceModel()
			model.Source = types.StringNull()
			attrs := testBuildObject(types.StringValue(source), types.StringValue("constant")).Attributes()
			attrs["build_on_plan"] = types.BoolValue(true)
			model.Build = types.ObjectValueMust(packageBuildAttrTypes, attrs)
			if scenario == "optional" {
				WithOptionalComponents([]string{"missing"})(&model)
			}
			if scenario == "invalid values" {
				WithValues(testDynamicStringValues(map[string]string{"count": "not-an-integer"}))(&model)
			}
			if scenario == "partial values" || scenario == "invalid known with unknown leaf" {
				count := attr.Value(types.NumberValue(big.NewFloat(2)))
				countType := attr.Type(types.NumberType)
				if scenario == "invalid known with unknown leaf" {
					count = types.StringValue("invalid")
					countType = types.StringType
				}
				model.Values = types.DynamicValue(types.ObjectValueMust(map[string]attr.Type{"domain": types.StringType, "count": countType}, map[string]attr.Value{"domain": types.StringUnknown(), "count": count}))
			}
			vars := []VariableModel{{Name: types.StringValue("TOKEN"), Value: types.StringValue("deployment-only")}}
			if scenario == "beta vars" || scenario == "alpha vars" {
				WithVars(vars)(&model)
			}
			if scenario == "beta sensitive vars" {
				WithSensitiveVars(vars)(&model)
			}
			var prior *deployedPackageIdentity
			if scenario == "name mismatch" {
				prior = &deployedPackageIdentity{Name: "other"}
			}
			checks := r.runPackagePlanChecks(context.Background(), model, prior)
			switch scenario {
			case "valid", "alpha vars", "partial values":
				require.Equal(t, packagePlanCheckResult{}, checks)
			case "invalid package":
				require.Error(t, checks.LoadErr)
			case "optional":
				require.Error(t, checks.OptComponentsErr)
			case "invalid values", "invalid known with unknown leaf":
				require.Error(t, checks.ValuesErr)
			case "beta vars", "beta sensitive vars":
				require.Error(t, checks.VariablesErr)
			case "name mismatch":
				require.Error(t, checks.CanonicalNameErr)
			}
		})
	}
}

func TestPackageBuildApplyVersionedInputs(t *testing.T) {
	for _, scenario := range []string{"beta vars", "beta sensitive vars", "alpha vars", "beta values"} {
		t.Run(scenario, func(t *testing.T) {
			pkg := betaLifecycleTestPackage()
			if scenario == "alpha vars" {
				pkg.APIVersion = "zarf.dev/v1alpha1"
			}
			pkgLayout := newTestPackageLayout(t, pkg)
			deployed := newLifecycleDeployedPackage(t, pkgLayout, zarfState.DeployedComponent{Name: "optional"})
			clientset := fake.NewSimpleClientset()
			cluster := &MockCluster{}
			p := &MockPackager{}
			old := NewTestPackageResourceModel(WithDeployedState(), WithOptionalComponents([]string{"optional"}))
			old.ID = types.StringValue(pkg.Metadata.Name)
			old.Name = types.StringValue(pkg.Metadata.Name)
			plan := old
			plan.Source = types.StringNull()
			plan.Build = testBuildObject(types.StringValue(t.TempDir()), types.StringValue("new"))
			WithOptionalComponents([]string{})(&plan)
			WithValues(testDynamicStringValues(map[string]string{"domain": "example.test"}))(&plan)
			WithSensitiveValues(testDynamicStringValues(map[string]string{"token": "secret"}))(&plan)
			vars := []VariableModel{{Name: types.StringValue("TOKEN"), Value: types.StringValue("deployment-only")}}
			if scenario == "beta vars" || scenario == "alpha vars" {
				WithVars(vars)(&plan)
			}
			if scenario == "beta sensitive vars" {
				WithSensitiveVars(vars)(&plan)
			}
			p.On("Create", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) { require.Empty(t, args.Get(3).(zarfPackager.CreateOptions).SetVariables) }).Return("built.tar.zst", nil).Once()
			p.On("LoadPackage", mock.Anything, "built.tar.zst", mock.Anything).Return(pkgLayout, nil).Once()
			valid := scenario == "alpha vars" || scenario == "beta values"
			if valid {
				cluster.On("NewWithWait", mock.Anything).Return(&zarfCluster.Cluster{Clientset: clientset}, nil)
				p.On("Remove", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
					require.Equal(t, zarfValue.Values{"domain": "example.test", "token": "secret"}, args.Get(2).(zarfPackager.RemoveOptions).Values)
				}).Return(nil).Once()
				p.On("Deploy", mock.Anything, pkgLayout, mock.Anything).Run(func(args mock.Arguments) {
					opts := args.Get(2).(zarfPackager.DeployOptions)
					require.Equal(t, zarfValue.Values{"domain": "example.test", "token": "secret"}, opts.Values)
					if scenario == "alpha vars" {
						require.Equal(t, "deployment-only", opts.SetVariables["TOKEN"])
					}
				}).Return(zarfPackager.DeployResult{}, nil).Once()
			}
			r := NewPackageResource(nil, p, nil, cluster).(*PackageResource)
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			_, err := r.deployAsNewOrUpdate(ctx, plan, old, deployedPackageIdentity{ID: old.ID.ValueString(), Name: pkg.Metadata.Name, Package: deployed})
			require.Equal(t, !valid, err != nil, "%v", err)
			if !valid {
				p.AssertNotCalled(t, "Remove", mock.Anything, mock.Anything, mock.Anything)
				p.AssertNotCalled(t, "Deploy", mock.Anything, mock.Anything, mock.Anything)
			}
			p.AssertExpectations(t)
		})
	}
}

func TestPackageBuildPlanDoesNotDetectFileChanges(t *testing.T) {
	source := t.TempDir()
	p := &observedBuildPackager{Packager: udsPackager.NewPackager()}
	r := NewPackageResource(&udsProviderConfig{DefaultArchitecture: "amd64", ZarfCachePath: t.TempDir()}, p, nil, &MockCluster{}).(*PackageResource)
	model := NewTestPackageResourceModel(WithDeployedState(), WithOptionalComponents([]string{}))
	model.Source = types.StringNull()
	attrs := testBuildObject(types.StringValue(source), types.StringValue("constant")).Attributes()
	attrs["build_on_plan"] = types.BoolValue(true)
	model.Build = types.ObjectValueMust(packageBuildAttrTypes, attrs)
	model.SignatureVerification = defaultSignatureVerification
	state := buildTestState(t, r, model)
	for _, version := range []string{"1.0.0", "2.0.0"} {
		definition := fmt.Sprintf(`kind: ZarfPackageConfig
metadata:
  name: test-pkg
  version: %s
components:
  - name: action
    actions:
      onCreate:
        before:
          - cmd: echo build >> build-count
`, version)
		require.NoError(t, os.WriteFile(filepath.Join(source, "zarf.yaml"), []byte(definition), 0o600))
		plan := buildTestPlan(t, r, model)
		resp := resource.ModifyPlanResponse{Plan: plan}
		r.ModifyPlan(context.Background(), resource.ModifyPlanRequest{Config: buildTestConfig(t, r, model), Plan: plan, State: state}, &resp)
		require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
		require.True(t, state.Raw.Equal(resp.Plan.Raw), "validation must not publish fresh build metadata into the plan")
		require.NoDirExists(t, p.outputDir)
	}
	count, err := os.ReadFile(filepath.Join(source, "build-count"))
	require.NoError(t, err)
	require.Equal(t, strings.Repeat("build\n", 2), string(count))
}
