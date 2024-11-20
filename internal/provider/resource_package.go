// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	// zarfCLI "github.com/zarf-dev/zarf/src/cmd"
	// zarfCLICommon "github.com/zarf-dev/zarf/src/cmd/common"
	// zarfConfig "github.com/zarf-dev/zarf/src/config"
	// "github.com/zarf-dev/zarf/src/pkg/packager"
	// zarfTypes "github.com/zarf-dev/zarf/src/types"

	zarfConfig "github.com/zarf-dev/zarf/src/config"
	"github.com/zarf-dev/zarf/src/pkg/cluster"
	zarfPackager "github.com/zarf-dev/zarf/src/pkg/packager"
	zarfTypes "github.com/zarf-dev/zarf/src/types"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &PackageResource{}
var _ resource.ResourceWithImportState = &PackageResource{}

func NewPackageResource() resource.Resource {
	return &PackageResource{}
}

// PackageResource defines the resource implementation.
type PackageResource struct{}

// PackageResourceModel describes the resource data model.
type PackageResourceModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	Components types.List   `tfsdk:"components"`
	Timeout    types.String `tfsdk:"timeout"`
}

func (r *PackageResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_package"
}

func (r *PackageResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "UDS Package resource",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Example identifier",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "File name of the package",
			},
			"components": schema.ListAttribute{
				Optional:            true,
				MarkdownDescription: "Explicit list of components to include in the package, if empty, all default components are included",
				ElementType: types.ListType{
					ElemType: types.StringType,
				},
			},
			// Set the default value to "30m" vs the default of "15m" since this runs in automation
			"timeout": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("30m"),
				MarkdownDescription: "Timeout for the deploy operation",
			},
		},
	}
}

func (r *PackageResource) Configure(_ context.Context, _ resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	// if req.ProviderData == nil {
	// 	return
	// }
}

func (r *PackageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data PackageResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Set the prefix for `./zarf` actions since we have to vendor zarf
	zarfConfig.ActionsCommandZarfPrefix = "zarf"

	// Confirm `zarf package deploy` since we're running in automation
	zarfConfig.CommonOptions.Confirm = true

	// convert the terraform timeout to a time.Duration
	timeout, err := time.ParseDuration(data.Timeout.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid timeout value",
			"Could not parse timeout duration: "+err.Error(),
		)
		return
	}

	deployOpts := zarfTypes.ZarfDeployOptions{
		Timeout: timeout,
	}

	pkgOpts := zarfTypes.ZarfPackageOptions{
		// Load the package path from the terraform plan
		PackageSource: data.Name.ValueString(),

		// Explicitly set the components to deploy if specified
		// OptionalComponents: strings.Join(data.Components.Elements(), ","),
	}

	// Initialize the package config
	pkgConfig := zarfTypes.PackagerConfig{
		DeployOpts: deployOpts,
		PkgOpts:    pkgOpts,
	}

	// read the zarf package name from the terraform plan. Right now this
	// expects a local file name, ex: "zarf-init-arm64-v0.43.0.tar.zst".
	// TODO(clint): make this more flexible so we detect if it's a zarf init
	// package or not, and only add the init opts if needed. Right now it's just
	// a spike that got me through the first hurdle.
	pkgConfig.PkgOpts.PackageSource = data.Name.ValueString()

	// default zarf init opts
	pkgConfig.InitOpts = zarfTypes.ZarfInitOptions{
		GitServer: zarfTypes.GitServerInfo{
			PushUsername: zarfTypes.ZarfGitPushUser,
		},
		RegistryInfo: zarfTypes.RegistryInfo{
			PushUsername: zarfTypes.ZarfRegistryPushUser,
		},
	}

	// Create the package client
	pkgClient, err := zarfPackager.New(&pkgConfig)
	// Always clear the temp paths since we're running in automation
	defer pkgClient.ClearTempPaths()
	// Abort if we can't create the package client
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating package client",
			"Could not create client: "+err.Error(),
		)
		return
	}

	tflog.Debug(ctx, "starting deploy")
	if err := pkgClient.Deploy(ctx); err != nil {
		resp.Diagnostics.AddError(
			"Error deploying package",
			"Could not deploy package: "+err.Error(),
		)
		return
	}
	tflog.Debug(ctx, "ending deploy")

	// For the purposes of this example code, hardcoding a response value to
	// save into the Terraform state.
	data.ID = types.StringValue(data.Name.ValueString())

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "created a resource")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PackageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data PackageResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// c, err := cluster.NewClusterWithWait(timeoutCtx)
	c, _ := cluster.NewClusterWithWait(timeoutCtx)
	// if err != nil {
	// TODO(clint): handle error here
	// }

	deployedZarfPackages, err := c.GetDeployedZarfPackages(timeoutCtx)
	// if err != nil && len(deployedZarfPackages) == 0 {
	if err != nil && len(deployedZarfPackages) == 0 {
		// TODO(clint): handle the nil/zero
		return
	}

	// Populate a matrix of all the deployed packages
	packageData := [][]string{}

	for _, pkg := range deployedZarfPackages {
		var components []string

		for _, component := range pkg.DeployedComponents {
			components = append(components, component.Name)
		}

		packageData = append(packageData, []string{
			pkg.Name, pkg.Data.Metadata.Version, fmt.Sprintf("%v", components),
		})
	}

	// TODO(clint): verify the package name is in this list. Right now the
	// results here are things like "init" instead of the name we supplied, so
	// we need to either dig into the package and find the metadata name, or
	// find another way to identify and ask Zarf/Kubernetes to give us the
	// package name.
	if len(packageData) == 0 {
		resp.Diagnostics.AddWarning(
			"Package not found",
			"Could not find package in deployed packages; removing resource",
		)
		resp.State.RemoveResource(ctx)
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PackageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data PackageResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *PackageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data PackageResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	opts := zarfTypes.ZarfPackageOptions{
		PackageSource: data.Name.ValueString(),
	}
	pkgConfig := zarfTypes.PackagerConfig{
		PkgOpts: opts,
	}

	pkgClient, err := zarfPackager.New(&pkgConfig)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating package client",
			"Could not create package client: "+err.Error(),
		)
		return
	}
	defer pkgClient.ClearTempPaths()

	if err := pkgClient.Remove(context.TODO()); err != nil {
		resp.Diagnostics.AddError(
			"Error removing package",
			"Could not remove package: "+err.Error(),
		)
		return
	}

	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *PackageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
