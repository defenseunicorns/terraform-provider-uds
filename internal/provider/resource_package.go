// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package provider

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	// zarfCLI "github.com/zarf-dev/zarf/src/cmd"
	// zarfCLICommon "github.com/zarf-dev/zarf/src/cmd/common"
	// zarfConfig "github.com/zarf-dev/zarf/src/config"
	// "github.com/zarf-dev/zarf/src/pkg/packager"
	// zarfTypes "github.com/zarf-dev/zarf/src/types"

	zarfConfig "github.com/zarf-dev/zarf/src/config"
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
type PackageResource struct {
}

// PackageResourceModel describes the resource data model.
type PackageResourceModel struct {
	ConfigurableAttribute types.String `tfsdk:"configurable_attribute"`
	ID                    types.String `tfsdk:"id"`
	Name                  types.String `tfsdk:"name"`
}

func (r *PackageResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_package"
}

func (r *PackageResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "UDS Package resource",

		Attributes: map[string]schema.Attribute{
			"configurable_attribute": schema.StringAttribute{
				MarkdownDescription: "Example configurable attribute",
				Optional:            true,
			},
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

	zarfConfig.CommonOptions.Confirm = true
	pkgConfig := zarfTypes.PackagerConfig{}
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

	pkgConfig.DeployOpts = zarfTypes.ZarfDeployOptions{
		Timeout: 15 * time.Minute,
	}

	// Tell Zarf to confirm all actions. It feels weird/wrong to do this here,
	// but I'm not sure what the best way to do this is. It's what uds-cli does
	// as well: https://github.com/defenseunicorns/uds-cli/blob/1ccbb716831b73b05f74acd067be29d6f69f1733/src/pkg/bundle/deploy.go#L126
	zarfConfig.CommonOptions.Confirm = true
	pkgClient, err := zarfPackager.New(&pkgConfig)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating package client",
			"Could create client: "+err.Error(),
		)
		return
	}
	defer pkgClient.ClearTempPaths()
	tflog.Debug(ctx, "starting deploy")
	if err := pkgClient.Deploy(ctx); err != nil {
		resp.Diagnostics.AddError(
			"failed to deploy package",
			"failed to deploy package: "+err.Error(),
		)
		return
	}
	tflog.Debug(ctx, "ending deploy")

	// For the purposes of this example code, hardcoding a response value to
	// save into the Terraform state.
	// TODO(clint): don't hardcode the zarf-init-id
	data.ID = types.StringValue("zarf-init-id")

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

	// If applicable, this is a great opportunity to initialize any necessary
	// provider client data and make a call using it.
	// httpResp, err := r.client.Do(httpReq)
	// if err != nil {
	//     resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read example, got error: %s", err))
	//     return
	// }

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

	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *PackageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
