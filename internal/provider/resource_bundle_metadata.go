// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package provider

import (
	"context"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &BundleMetadataResource{}

func NewBundleMetadataResource() resource.Resource {
	return &BundleMetadataResource{}
}

// BundleMetadataResource defines the resource implementation.
type BundleMetadataResource struct{}

// BundleMetadataResourceModel describes the resource data model.
type BundleMetadataResourceModel struct {
	ID      types.String `tfsdk:"id"`
	Version types.String `tfsdk:"version"`
	// Kind reflects the type of package; typically always UDSBundle
	Kind         types.String `tfsdk:"kind"`
	Description  types.String `tfsdk:"description"`
	URL          types.String `tfsdk:"url"`
	Architecture types.String `tfsdk:"architecture"`
}

func (r *BundleMetadataResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_bundle_metadata"
}

func (r *BundleMetadataResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "UDS Bundle Metadata resource. This resource is used to track the metadata for a UDS Bundle.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Generated unique identifier",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"version": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Generic string set by a package author to track the package version",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"kind": schema.StringAttribute{
				Computed:            true,
				Optional:            true,
				MarkdownDescription: "Kind of Zarf package; ZarfInitConfig or ZarfPackageConfig",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Additional information about this package",
			},
			"url": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Link to package information when online",
			},
			"architecture": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The target cluster architecture for this package",
			},
		},
	}
}

func (r *BundleMetadataResource) Configure(_ context.Context, _ resource.ConfigureRequest, _ *resource.ConfigureResponse) {
}

func (r *BundleMetadataResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data BundleMetadataResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	data.ID = types.StringValue(uuid.NewString())
	data.Kind = types.StringValue("UDSBundle")

	tflog.Trace(ctx, "created uds bundle metadata package resource")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *BundleMetadataResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data BundleMetadataResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *BundleMetadataResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data BundleMetadataResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *BundleMetadataResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data BundleMetadataResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}
}
