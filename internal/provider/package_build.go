// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"

	udsPackager "github.com/defenseunicorns/terraform-provider-uds/internal/packager"
)

type packageBuildError struct{ cause error }

func (e *packageBuildError) Error() string { return e.cause.Error() }
func (e *packageBuildError) Unwrap() error { return e.cause }

func isBuildMode(model PackageResourceModel) bool {
	return !model.Build.IsNull() && !model.Build.IsUnknown()
}

func shouldBuildOnPlan(model PackageResourceModel) bool {
	if !isBuildMode(model) {
		return false
	}
	attrs := model.Build.Attributes()
	for _, name := range []string{"source_dir", "revision", "build_on_plan"} {
		if attrs[name].IsNull() || attrs[name].IsUnknown() {
			return false
		}
	}
	return attrs["build_on_plan"].(types.Bool).ValueBool() && !model.Architecture.IsUnknown()
}

func packageInputPath(model PackageResourceModel) path.Path {
	if isBuildMode(model) {
		return path.Root("build").AtName("source_dir")
	}
	return path.Root("source")
}

// acquirePackageLayout translates resource inputs at the Framework boundary.
// Build orchestration and storage ownership remain in the packager helper.
func (r *PackageResource) acquirePackageLayout(ctx context.Context, model PackageResourceModel) (*layout.PackageLayout, func() error, error) {
	if isBuildMode(model) {
		input, err := r.packageBuildInput(ctx, model)
		if err != nil {
			return nil, nil, &packageBuildError{cause: err}
		}
		pkg, cleanup, err := udsPackager.BuildAndLoad(ctx, r.packager, input)
		if err != nil {
			return nil, nil, &packageBuildError{cause: err}
		}
		return pkg, cleanup, nil
	}
	pkg, err := r.loadPackageLayoutFromSource(ctx, model)
	if err != nil {
		return nil, nil, err
	}
	return pkg, pkg.Cleanup, nil
}

func (r *PackageResource) packageBuildInput(ctx context.Context, model PackageResourceModel) (udsPackager.BuildInput, error) {
	var build PackageBuildModel
	if diags := model.Build.As(ctx, &build, basetypes.ObjectAsOptions{}); diags.HasError() {
		return udsPackager.BuildInput{}, fmt.Errorf("unable to decode build inputs")
	}
	return udsPackager.BuildInput{
		SourceDir:             build.SourceDir.ValueString(),
		Architecture:          getArchitecture(model, *r.providerConfig),
		CachePath:             r.providerConfig.ZarfCachePath,
		PlainHTTP:             r.providerConfig.InsecureForceHTTP,
		InsecureSkipTLSVerify: r.providerConfig.InsecureSkipTLSVerification,
	}, nil
}

// validatePackageInput receives raw configuration, not defaulted plan values.
func validatePackageInput(model PackageResourceModel, resp *resource.ValidateConfigResponse) {
	if model.Source.IsNull() && model.Build.IsNull() {
		resp.Diagnostics.AddAttributeError(path.Root("source"), "Missing package input", "Specify exactly one of `source` or `build`.")
	} else if !model.Source.IsNull() && !model.Source.IsUnknown() && !model.Build.IsNull() && !model.Build.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("build"), "Conflicting package inputs", "Specify exactly one of `source` or `build`, not both.")
	}
	if !model.Build.IsNull() && !model.Build.IsUnknown() {
		for _, name := range []string{"source_dir", "revision"} {
			if model.Build.Attributes()[name].IsNull() {
				resp.Diagnostics.AddAttributeError(path.Root("build").AtName(name), "Missing build input", "Both `build.source_dir` and `build.revision` are required.")
			}
		}
	}
}
