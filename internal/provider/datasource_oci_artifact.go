// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/defenseunicorns/terraform-provider-uds/internal/oci"
	udsValidator "github.com/defenseunicorns/terraform-provider-uds/internal/provider/validator"
)

var _ datasource.DataSource = &OCIArtifactDataSource{}

// OCIArtifactDataSource fetches content from an OCI artifact stored in a container registry.
type OCIArtifactDataSource struct {
	providerConfig *udsProviderConfig
}

// OCIArtifactDataSourceModel describes the data source data model.
type OCIArtifactDataSourceModel struct {
	ID        types.String `tfsdk:"id"`
	Reference types.String `tfsdk:"reference"`
	File      types.String `tfsdk:"file"`
	MediaType types.String `tfsdk:"media_type"`
	Content   types.String `tfsdk:"content"`
	Digest    types.String `tfsdk:"digest"`
}

// NewOCIArtifactDataSource creates a new instance of the OCI artifact data source.
func NewOCIArtifactDataSource() datasource.DataSource {
	return &OCIArtifactDataSource{}
}

// Metadata sets the data source type name.
func (d *OCIArtifactDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_oci_artifact"
}

// Schema defines the schema for the data source.
func (d *OCIArtifactDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Fetches content from an OCI artifact stored in a container registry.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier for this data source.",
			},
			"reference": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "OCI reference to the artifact (e.g., `oci://registry.example.com/repo:tag`).",
				Validators: []validator.String{
					udsValidator.OCIReferenceValidator(),
				},
			},
			"file": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Name of a specific file within the artifact to fetch. Matched against the `org.opencontainers.image.title` layer annotation. If not specified, the first layer is returned.",
			},
			"media_type": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Filter layers by media type. If specified, only layers matching this media type are considered.",
			},
			"content": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The fetched artifact content as a string.",
			},
			"digest": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The digest of the fetched content.",
			},
		},
	}
}

// Configure configures the data source with provider data.
func (d *OCIArtifactDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	d.providerConfig = req.ProviderData.(*udsProviderConfig)
}

func (d *OCIArtifactDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var model OCIArtifactDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	reference := model.Reference.ValueString()
	tflog.Info(ctx, "Fetching OCI artifact", map[string]interface{}{
		"reference": reference,
	})

	fetchOpts := oci.FetchOptions{
		PlainHTTP:             d.providerConfig.InsecureForceHTTP,
		InsecureSkipTLSVerify: d.providerConfig.InsecureSkipTLSVerification,
	}
	if !model.File.IsNull() {
		fetchOpts.File = model.File.ValueString()
	}
	if !model.MediaType.IsNull() {
		fetchOpts.MediaType = model.MediaType.ValueString()
	}

	content, digest, err := oci.FetchArtifact(ctx, reference, fetchOpts)
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to fetch OCI artifact",
			fmt.Sprintf("Error fetching artifact from %q: %s", reference, err),
		)
		return
	}

	model.ID = types.StringValue(reference)
	model.Content = types.StringValue(string(content))
	model.Digest = types.StringValue(digest)

	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}
