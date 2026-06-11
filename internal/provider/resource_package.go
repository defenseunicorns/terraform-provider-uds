// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package provider

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"gopkg.in/yaml.v2"

	udsCluster "github.com/defenseunicorns/terraform-provider-uds/internal/cluster"
	udsPackager "github.com/defenseunicorns/terraform-provider-uds/internal/packager"
	udsValidator "github.com/defenseunicorns/terraform-provider-uds/internal/provider/validator"

	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	zarfPackager "github.com/zarf-dev/zarf/src/pkg/packager"
	zarfFilters "github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	zarfLayout "github.com/zarf-dev/zarf/src/pkg/packager/layout"
	zarfSigning "github.com/zarf-dev/zarf/src/pkg/signing"
	zarfState "github.com/zarf-dev/zarf/src/pkg/state"
	zarfZoci "github.com/zarf-dev/zarf/src/pkg/zoci"
	zarfTypes "github.com/zarf-dev/zarf/src/types"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &PackageResource{}
	_ resource.ResourceWithImportState = &PackageResource{}
	_ resource.ResourceWithModifyPlan  = &PackageResource{}
)

const (
	clusterTimeoutMinutes = 5
	defaultPackageTimeout = "15m"
)

// NewPackageResource creates a new instance of the package resource.
func NewPackageResource(providerConfig *udsProviderConfig, packager udsPackager.Packager, packageComponentFilter udsPackager.PackageComponentFilter, cluster udsCluster.Cluster) resource.Resource {
	if providerConfig == nil {
		providerConfig = &udsProviderConfig{}
	}
	if packager == nil {
		packager = udsPackager.NewPackager()
	}
	if packageComponentFilter == nil {
		packageComponentFilter = udsPackager.NewPackageComponentFilter()
	}
	if cluster == nil {
		cluster = udsCluster.NewCluster()
	}
	return &PackageResource{
		providerConfig: providerConfig,
		packager:       packager,
		packageFilter:  packageComponentFilter,
		cluster:        cluster,
	}
}

// PackageResourceModel describes the resource data model.
type PackageResourceModel struct {
	ID                    types.String `tfsdk:"id"`
	Source                types.String `tfsdk:"source"`
	Architecture          types.String `tfsdk:"architecture"`
	Timeout               types.String `tfsdk:"timeout"`
	SignatureVerification types.Object `tfsdk:"signature_verification"`
	Namespace             types.String `tfsdk:"namespace"`

	Components         types.Set `tfsdk:"component"`           // Set of ComponentModel objects (TODO: remove when component block is removed)
	OptionalComponents types.Set `tfsdk:"optional_components"` // Set of string component names (alpha)
	Vars               types.Set `tfsdk:"vars"`                // Set of VariableModel objects
	SensitiveVars      types.Set `tfsdk:"sensitive_vars"`      // Set of VariableModel objects

	// readonly metadata
	Name           types.String `tfsdk:"name"`
	Kind           types.String `tfsdk:"kind"` // Kind reflects the type of UDS package; either ZarfInit or ZarfPackage
	Version        types.String `tfsdk:"version"`
	Metadata       types.Object `tfsdk:"metadata"`
	ConnectStrings types.Set    `tfsdk:"connect_strings"` // Set of ConnectString objects

	// zarf package variables are written automatically
	// into the computed `set_variables` map.
	// All variables exported from the package are persisted to this map and
	// treated as sensitive in state regardless of their original sensitivity.
	SetVariables types.Map `tfsdk:"set_variables"`
}

// ComponentModel represents a UDS package component configuration.
// TODO: remove when component block is removed
type ComponentModel struct {
	Name      types.String `tfsdk:"name"`
	Overrides types.Set    `tfsdk:"override"` // Set of ComponentChartValuesModel objects
}

// ComponentChartValuesModel represents a helm chart override values configuration for a package component.
// TODO: remove when component block is removed
type ComponentChartValuesModel struct {
	ChartName       types.String `tfsdk:"chart_name"`
	Values          types.Set    `tfsdk:"values"`           // Set of HelmChartPathValueModel objects
	SensitiveValues types.Set    `tfsdk:"sensitive_values"` // Set of HelmChartPathValueModel objects
}

// HelmChartPathValueModel represents a path/value pair for setting helm chart values
// TODO: remove when component block is removed
type HelmChartPathValueModel struct {
	Path  types.String `tfsdk:"path"`
	Value types.String `tfsdk:"value"`
}

// VariableModel represents a name/value pair for setting UDS package variables
type VariableModel struct {
	Name  types.String `tfsdk:"name"`
	Value types.String `tfsdk:"value"`
}

// KeylessVerificationModel holds Sigstore/OIDC keyless signature verification configuration.
type KeylessVerificationModel struct {
	CertificateIdentity         types.String `tfsdk:"certificate_identity"`
	CertificateIdentityRegexp   types.String `tfsdk:"certificate_identity_regexp"`
	CertificateOIDCIssuer       types.String `tfsdk:"certificate_oidc_issuer"`
	CertificateOIDCIssuerRegexp types.String `tfsdk:"certificate_oidc_issuer_regexp"`
	TrustedRoot                 types.String `tfsdk:"trusted_root"`
	InsecureIgnoreTlog          types.Bool   `tfsdk:"insecure_ignore_tlog"`
	UseSignedTimestamps         types.Bool   `tfsdk:"use_signed_timestamps"`
}

var keylessVerificationAttrTypes = map[string]attr.Type{
	"certificate_identity":           types.StringType,
	"certificate_identity_regexp":    types.StringType,
	"certificate_oidc_issuer":        types.StringType,
	"certificate_oidc_issuer_regexp": types.StringType,
	"trusted_root":                   types.StringType,
	"insecure_ignore_tlog":           types.BoolType,
	"use_signed_timestamps":          types.BoolType,
}

// SignatureVerificationModel holds all signature verification configuration for a UDS package.
type SignatureVerificationModel struct {
	Verify    types.Bool   `tfsdk:"verify"`
	PublicKey types.String `tfsdk:"public_key"`
	Keyless   types.Object `tfsdk:"keyless"`
}

var signatureVerificationAttrTypes = map[string]attr.Type{
	"verify":     types.BoolType,
	"public_key": types.StringType,
	"keyless":    types.ObjectType{AttrTypes: keylessVerificationAttrTypes},
}

var connectStringAttrTypes = map[string]attr.Type{
	"name":        types.StringType,
	"description": types.StringType,
}

var defaultSignatureVerification = types.ObjectValueMust(
	signatureVerificationAttrTypes,
	map[string]attr.Value{
		"verify":     types.BoolValue(true),
		"public_key": types.StringNull(),
		"keyless":    types.ObjectNull(keylessVerificationAttrTypes),
	},
)

// Metadata sets the resource type name.
func (r *PackageResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_package"
}

// Schema defines the schema for the resource.
func (r *PackageResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Deploys a UDS Package.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Identifier for the deployed UDS package.",
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the UDS Package.",
				Computed:            true,
			},
			"source": schema.StringAttribute{
				MarkdownDescription: "OCI distribution reference (including oci:// scheme) or local file path (absolute or relative) to the package.",
				Required:            true,
				Validators: []validator.String{
					udsValidator.PackageSourceValidator(),
				},
			},
			"architecture": schema.StringAttribute{
				MarkdownDescription: "System architecture of the target cluster. Defaults to the provider default architecture.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.OneOf("amd64", "arm64"),
				},
			},
			"version": schema.StringAttribute{
				MarkdownDescription: "Version of the deployed UDS package.",
				Computed:            true,
			},
			"signature_verification": schema.SingleNestedAttribute{
				MarkdownDescription: "Signature verification configuration. Omit to use defaults (verification enabled, no key).",
				Optional:            true,
				Computed:            true,
				Default:             objectdefault.StaticValue(defaultSignatureVerification),
				Attributes: map[string]schema.Attribute{
					"verify": schema.BoolAttribute{
						MarkdownDescription: "When true, verify the signature of a signed UDS package. When false, skip package signature verification.",
						Optional:            true,
						Computed:            true,
						Default:             booldefault.StaticBool(true),
					},
					"public_key": schema.StringAttribute{
						MarkdownDescription: "Raw public key value to validate against a key-signed UDS package. Mutually exclusive with `keyless`.",
						Optional:            true,
					},
					"keyless": schema.SingleNestedAttribute{
						MarkdownDescription: "Keyless (Sigstore/OIDC) signature verification configuration. Mutually exclusive with `public_key`.",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"certificate_identity": schema.StringAttribute{
								MarkdownDescription: "Required identity claim in the signing certificate. Mutually exclusive with `certificate_identity_regexp`.",
								Optional:            true,
							},
							"certificate_identity_regexp": schema.StringAttribute{
								MarkdownDescription: "Regex-based alternative to `certificateIdentity` for pattern matching. Mutually exclusive with `certificate_identity`.",
								Optional:            true,
							},
							"certificate_oidc_issuer": schema.StringAttribute{
								MarkdownDescription: "Required OIDC issuer claim in the signing certificate. Mutually exclusive with `certificate_oidc_issuer_regexp`.",
								Optional:            true,
							},
							"certificate_oidc_issuer_regexp": schema.StringAttribute{
								MarkdownDescription: "Regex-based variant of `certificateOIDCIssuer`. Mutually exclusive with `certificate_oidc_issuer`.",
								Optional:            true,
							},
							"trusted_root": schema.StringAttribute{
								MarkdownDescription: "Sigstore TrustedRoot JSON content for keyless signature verification. Omit to use Zarf's embedded TrustedRoot.",
								Optional:            true,
							},
							"insecure_ignore_tlog": schema.BoolAttribute{
								MarkdownDescription: "Skip Rekor transparency log inclusion verification. Set to true only for air-gapped or private Sigstore infrastructure.",
								Optional:            true,
								Computed:            true,
								Default:             booldefault.StaticBool(false),
							},
							"use_signed_timestamps": schema.BoolAttribute{
								MarkdownDescription: "Verify RFC3161 signed timestamps in the Sigstore verification bundle. Auto-enabled when the bundle contains TSA timestamp data.",
								Optional:            true,
								Computed:            true,
								Default:             booldefault.StaticBool(false),
							},
						},
					},
				},
			},
			"timeout": schema.StringAttribute{
				MarkdownDescription: "Timeout for the deploy operation.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(defaultPackageTimeout),
				Validators: []validator.String{
					udsValidator.DurationGreaterThanValidator(0),
				},
			},
			"kind": schema.StringAttribute{
				MarkdownDescription: "Kind of UDS package; ZarfInitConfig or ZarfPackageConfig.",
				Computed:            true,
			},
			"metadata": &schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Metadata retrieved from the UDS package (zarf.yaml).",
				Attributes: map[string]schema.Attribute{
					"name": &schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "Name of the UDS package. Used to identify the deployed UDS package.",
					},
					"description": &schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "Description of the UDS package, from the zarf.yaml file.",
					},
					"version": &schema.StringAttribute{
						Computed:            true,
						MarkdownDescription: "Version of the UDS package, from the zarf.yaml file.",
					},
				},
			},
			"vars": schema.SetNestedAttribute{
				MarkdownDescription: "UDS package variables to set.",
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							MarkdownDescription: "Name of the variable to set.",
							Required:            true,
						},
						"value": schema.StringAttribute{
							MarkdownDescription: "Value for the variable to set.",
							Required:            true,
						},
					},
				},
				Validators: []validator.Set{
					func() validator.Set {
						v, _ := udsValidator.NewBlockStringAttributeUniquenessValidator("var", "name")
						return v
					}(),
				},
			},
			"sensitive_vars": schema.SetNestedAttribute{
				MarkdownDescription: "Sensitive UDS package variables to set.",
				Optional:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							MarkdownDescription: "Name of the variable to set.",
							Required:            true,
						},
						"value": schema.StringAttribute{
							MarkdownDescription: "Value for the variable to set.",
							Required:            true,
							Sensitive:           true,
						},
					},
				},
				Validators: []validator.Set{
					func() validator.Set {
						v, _ := udsValidator.NewBlockStringAttributeUniquenessValidator("sensitive_var", "name")
						return v
					}(),
				},
			},
			"namespace": schema.StringAttribute{
				MarkdownDescription: "[Alpha] Namespace in which to deploy the UDS package.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"optional_components": schema.SetAttribute{
				MarkdownDescription: "[Alpha] Set of optional package component names to install. Case-sensitive. Mutually exclusive with `component` blocks — specifying both is a validation error. When omitted or set to an empty list, only required package components are installed.",
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
			},
			"connect_strings": schema.SetNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Connect strings for connecting to services deployed by the package.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Name of the service/connection.",
						},
						"description": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Description of the service/compute-resource that this connect string is for.",
						},
					},
				},
			},
			"set_variables": schema.MapAttribute{
				MarkdownDescription: "Computed map of zarf variables set for this package.",
				Computed:            true,
				ElementType:         types.StringType,
				Sensitive:           true,
			},
		},
		Blocks: map[string]schema.Block{
			// TODO: remove when component block is removed
			"component": schema.SetNestedBlock{
				MarkdownDescription: "Selects an optional package component to install and configure helm chart overrides for it. Mutually exclusive with `optional_components`. When no `component` blocks are specified, only required package components are installed.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Name of the component.",
						},
					},
					Blocks: map[string]schema.Block{
						"override": schema.SetNestedBlock{
							MarkdownDescription: "Helm chart overrides for the component.",
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"chart_name": schema.StringAttribute{
										Required:            true,
										MarkdownDescription: "Name of the Helm chart to set values for.",
									},
									"values": schema.SetNestedAttribute{
										MarkdownDescription: "Set of path values to set for the chart.",
										Optional:            true,
										NestedObject: schema.NestedAttributeObject{
											Attributes: map[string]schema.Attribute{
												"path": schema.StringAttribute{
													MarkdownDescription: "The dot-notation path in the chart values to set.",
													Required:            true,
												},
												"value": schema.StringAttribute{
													MarkdownDescription: "The raw YAML value to set at the specified path.",
													Required:            true,
												},
											},
										},
										Validators: []validator.Set{
											func() validator.Set {
												v, _ := udsValidator.NewBlockStringAttributeUniquenessValidator("values", "path")
												return v
											}(),
										},
									},
									"sensitive_values": schema.SetNestedAttribute{
										MarkdownDescription: "Set of sensitive key-value overrides for the chart.",
										Optional:            true,
										NestedObject: schema.NestedAttributeObject{
											Attributes: map[string]schema.Attribute{
												"path": schema.StringAttribute{
													MarkdownDescription: "The dot-notation path in the chart values to set.",
													Required:            true,
												},
												"value": schema.StringAttribute{
													MarkdownDescription: "The raw YAML sensitive value to set at the specified path.",
													Required:            true,
													Sensitive:           true,
												},
											},
										},
										Validators: []validator.Set{
											func() validator.Set {
												v, _ := udsValidator.NewBlockStringAttributeUniquenessValidator("sensitive_values", "path")
												return v
											}(),
										},
									},
								},
							},
							Validators: []validator.Set{
								func() validator.Set {
									v, _ := udsValidator.NewBlockStringAttributeUniquenessValidator("override", "chart_name")
									return v
								}(),
							},
						},
					},
				},
				Validators: []validator.Set{
					func() validator.Set {
						v, _ := udsValidator.NewBlockStringAttributeUniquenessValidator("component", "name")
						return v
					}(),
				},
			},
		},
	}
}

// PackageResource defines the resource implementation.
type PackageResource struct {
	providerConfig *udsProviderConfig
	packager       udsPackager.Packager
	cluster        udsCluster.Cluster
	packageFilter  udsPackager.PackageComponentFilter
}

// ValidateConfig ensures validation between interdependant fields within a PackageResourceModel.
func (r *PackageResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var model PackageResourceModel

	diags := req.Config.Get(ctx, &model)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	validateUniqueVarNames(model, resp)
	validateSignatureVerificationAttributes(ctx, model, resp)
	// TODO: remove when component block is removed
	validateComponentBlockOptionalComponentsMutualExclusivity(model, resp)
}

// Configure configures the resource with provider data.
func (r *PackageResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	r.providerConfig = req.ProviderData.(*udsProviderConfig)

	// Initialize the packager if it wasn't set in NewPackageResource
	if r.packager == nil {
		r.packager = udsPackager.NewPackager()
	}

	if r.cluster == nil {
		r.cluster = udsCluster.NewCluster()
	}
}

// Create creates the resource and sets the initial Terraform state.
func (r *PackageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	tflog.Info(ctx, "Creating Package Resource")

	// Retrieve values from plan
	var plan PackageResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var err error
	plan, err = r.deployAsNew(ctx, plan)
	if err != nil {
		var optErr *optionalComponentsValidationError
		if errors.As(err, &optErr) {
			resp.Diagnostics.AddAttributeError(path.Root("optional_components"), "Invalid optional components", err.Error())
		} else {
			resp.Diagnostics.AddError("Error creating package", "Could not create resource, unexpected error: "+err.Error())
		}
		return
	}

	// Set state to fully populated data
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *PackageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if req.State.Raw.IsNull() {
		return
	}

	var data PackageResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeoutCtx, cancel := withClusterTimeout(ctx)
	defer cancel()

	packageNamespace, packageName, err := parsePackageID(data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error parsing package ID",
			"Failed to parse package ID: "+err.Error(),
		)
		return
	}

	deployedPackage, found, err := r.getDeployedPackage(timeoutCtx, packageName, packageNamespace)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting deployed package",
			"Failed to get deployed package: "+err.Error(),
		)
		return
	}
	if !found {
		resp.Diagnostics.AddWarning(
			"Deployed package not found",
			"Could not find deployed package with namespace "+packageNamespace+" and name "+packageName+" - removing resource",
		)
		resp.State.RemoveResource(ctx)
		return
	}

	// Populate/set resource computed values from deployed package info so that they can be saved to state
	data.Name = types.StringValue(deployedPackage.Name)
	data.Version = types.StringValue(deployedPackage.Data.Metadata.Version)
	data.Kind = types.StringValue(string(deployedPackage.Data.Kind))
	data.Architecture = types.StringValue(deployedPackage.Data.Metadata.Architecture)
	if deployedPackage.NamespaceOverride != "" {
		data.Namespace = types.StringValue(deployedPackage.NamespaceOverride)
	}
	if data.Timeout.IsNull() {
		data.Timeout = types.StringValue(defaultPackageTimeout)
	}

	// populate the package metadata type.
	// TODO(clint): this is ugly and I got it from https://developer.hashicorp.com/terraform/plugin/framework/handling-data/types/custom
	// There are probably a few optimizations or cleanups to be done here.
	// TODO(clint): this can be combined with the same code from the Create
	// method.
	elementTypes := map[string]attr.Type{
		"name":        types.StringType,
		"description": types.StringType,
		"version":     types.StringType,
	}
	elements := map[string]attr.Value{
		"name":        types.StringValue(deployedPackage.Name),
		"description": types.StringValue(deployedPackage.Data.Metadata.Description),
		"version":     types.StringValue(deployedPackage.Data.Metadata.Version),
	}
	pkgMetadata, diags := types.ObjectValue(elementTypes, elements)
	if diags.HasError() {
		resp.Diagnostics.AddError(
			"Error converting type to package metadata in read",
			"Could not convert: "+fmt.Sprintf("%v", diags),
		)
		return
	}
	data.Metadata = pkgMetadata

	// Populate connect_strings from deployed package
	connectStrings, err := getConnectStringsFromDeployedPackage(deployedPackage)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error converting connect strings",
			"Could not convert connect strings: "+err.Error(),
		)
		return
	}
	data.ConnectStrings = connectStrings

	// Bi-directional drift detection for optional_components (alpha):
	// Use deployed package metadata to determine which deployed components are optional.
	updatedOptionals, optDiags := refreshOptionalComponentsFromDeployedPackage(deployedPackage, data.OptionalComponents)
	resp.Diagnostics.Append(optDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.OptionalComponents = updatedOptionals

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *PackageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	tflog.Info(ctx, "Updating Package")

	// Retrieve values from plan
	var plan PackageResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Check if there are any components in the already existing plan that need to be removed
	var oldPlan PackageResourceModel
	diags = req.State.Get(ctx, &oldPlan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var err error
	plan, err = r.deployAsNewOrUpdate(ctx, plan, oldPlan, resp)
	if err != nil {
		var optErr *optionalComponentsValidationError
		if errors.As(err, &optErr) {
			resp.Diagnostics.AddAttributeError(path.Root("optional_components"), "Invalid optional components", err.Error())
		} else {
			resp.Diagnostics.AddError("Error updating package", "Could not update package, unexpected error: "+err.Error())
		}
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}

	// Set state to fully populated data
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *PackageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data PackageResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	// convert the terraform timeout to a time.Duration
	deleteTimeout, err := time.ParseDuration(data.Timeout.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error determine timeout",
			"Could not determine timeout: "+err.Error(),
		)
		return
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	c, err := r.cluster.NewWithWait(timeoutCtx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Could not connect to cluster",
			"Error connecting to cluster: "+err.Error(),
		)
		return
	}

	tmpDir, err := os.MkdirTemp("", "uds-package-verify-*")
	if err != nil {
		resp.Diagnostics.AddError(
			"Temp dir error",
			"Could not create temp dir for package verification: "+err.Error(),
		)
		return
	}
	defer os.RemoveAll(tmpDir)

	verifyBlobOpts, err := buildVerifyBlobOptions(ctx, data, tmpDir)
	if err != nil {
		resp.Diagnostics.AddError(
			"Verification config error",
			"Could not build verification options: "+err.Error(),
		)
		return
	}

	// TODO(erickson): Do we need configurable remote options?
	remoteOpts := zarfTypes.RemoteOptions{
		PlainHTTP:             r.providerConfig.InsecureForceHTTP,
		InsecureSkipTLSVerify: r.providerConfig.InsecureSkipTLSVerification,
	}

	loadOpts := zarfPackager.LoadOptions{
		Filter:               r.packageFilter.ForRemove([]string{}),
		Architecture:         getArchitecture(data, *r.providerConfig),
		VerifyBlobOptions:    verifyBlobOpts,
		VerificationStrategy: layout.VerifyNever,
		RemoteOptions:        remoteOpts,
		CachePath:            r.providerConfig.ZarfCachePath,
	}

	packageSource := data.Name.ValueString()
	pkg, err := r.packager.GetPackageFromSourceOrCluster(ctx, c, packageSource, data.Namespace.ValueString(), loadOpts)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error loading package",
			"Could not load package: "+err.Error(),
		)
		return
	}

	removeOpt := zarfPackager.RemoveOptions{
		NamespaceOverride: data.Namespace.ValueString(),
		Cluster:           c,
		Timeout:           deleteTimeout,
	}
	if err := r.packager.Remove(ctx, pkg, removeOpt); err != nil {
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

// ModifyPlan handles plan modifications for computed attributes that depend on provider configuration
func (r *PackageResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Only modify if we have a plan (not a destroy operation)
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan PackageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var config PackageResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If architecture is not explicitly set in config, set it to provider default
	if config.Architecture.IsNull() && r.providerConfig != nil {
		defaultArch := r.providerConfig.DefaultArchitecture
		tflog.Debug(ctx, "ModifyPlan: Setting architecture to provider default", map[string]any{
			"DefaultArchitecture": defaultArch,
			"PlanArchitecture":    plan.Architecture.ValueString(),
		})
		plan.Architecture = types.StringValue(defaultArch)
	}

	if r.providerConfig == nil || r.providerConfig.ValidatePackagesOnPlan {
		checks := r.runPackagePlanChecks(ctx, plan)
		if checks.LoadErr != nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("source"),
				"Failed to load package",
				checks.LoadErr.Error(),
			)
			return
		}
		if checks.SigErr != nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("signature_verification"),
				"Package signature verification failed",
				checks.SigErr.Error(),
			)
			return
		}
		if checks.OptComponentsErr != nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("optional_components"),
				"Invalid optional components",
				checks.OptComponentsErr.Error(),
			)
			return
		}
	}

	plan = normalizeOptionalComponentsPlan(config, plan)

	// When component selection changes, mark deployment-derived computed outputs as unknown
	// so Terraform doesn't hold the provider to prior known values that may change after apply.
	if !req.State.Raw.IsNull() {
		var state PackageResourceModel
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if !plan.OptionalComponents.Equal(state.OptionalComponents) ||
			!plan.Components.Equal(state.Components) {
			plan.ConnectStrings = types.SetUnknown(types.ObjectType{AttrTypes: connectStringAttrTypes})
			plan.SetVariables = types.MapUnknown(types.StringType)
		}
	}

	resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
}

// ImportState imports the resource state from an external system.
func (r *PackageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *PackageResource) getRemoteOptions() zarfTypes.RemoteOptions {
	return zarfTypes.RemoteOptions{
		PlainHTTP:             r.providerConfig.InsecureForceHTTP,
		InsecureSkipTLSVerify: r.providerConfig.InsecureSkipTLSVerification,
	}
}

func (r *PackageResource) getPackageLayoutFromSource(ctx context.Context, model PackageResourceModel) (*zarfLayout.PackageLayout, error) {
	packageSource, err := getPackageSource(model, *r.providerConfig)
	if err != nil {
		return nil, err
	}

	tmpDir, err := os.MkdirTemp("", "uds-package-verify-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir for package verification: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	verifyBlobOpts, err := buildVerifyBlobOptions(ctx, model, tmpDir)
	if err != nil {
		return nil, err
	}

	// TODO(erickson): add support for Shasum, CachePath, OCIConcurrency?
	loadOpt := zarfPackager.LoadOptions{
		Filter:               zarfFilters.Empty(),
		Architecture:         getArchitecture(model, *r.providerConfig),
		VerifyBlobOptions:    verifyBlobOpts,
		VerificationStrategy: layout.VerifyNever,
		RemoteOptions:        r.getRemoteOptions(),
		CachePath:            r.providerConfig.ZarfCachePath,
	}

	pkgLayout, err := r.packager.LoadPackage(ctx, packageSource, loadOpt)
	if err != nil {
		return nil, err
	}

	// Verify package signature
	enforceSignatureVerification := getEffectiveSignatureVerification(ctx, model)
	verifyOpts := zarfSigning.DefaultVerifyBlobOptions()
	if verifyBlobOpts != nil {
		verifyOpts = *verifyBlobOpts
	}
	verifyErr := pkgLayout.VerifyPackageSignature(ctx, verifyOpts)
	if err := handleVerifyResult(ctx, verifyErr, pkgLayout.IsSigned(), enforceSignatureVerification); err != nil {
		return nil, err
	}

	return pkgLayout, nil
}

// handleVerifyResult interprets the error from VerifyPackageSignature and either
// returns an error (to block deploy) or emits a contextual warning and returns nil.
func handleVerifyResult(ctx context.Context, err error, isSigned bool, enforce bool) error {
	if err == nil {
		return nil
	}
	if !isSigned {
		tflog.Warn(ctx, "package is unsigned; skipping signature verification",
			map[string]any{"error": err.Error()})
		return nil
	}
	if enforce {
		return err
	}
	tflog.Warn(ctx, "signature verification failed; proceeding because verify=false",
		map[string]any{"error": err.Error()})
	return nil
}

// getEffectiveSignatureVerification returns whether signature verification is enforced.
// Absent signature_verification block defaults to enforced/enabled.
func getEffectiveSignatureVerification(ctx context.Context, model PackageResourceModel) bool {
	// schema default ensures this is never null in practice; guard for safety (e.g. import, tests)
	if model.SignatureVerification.IsNull() || model.SignatureVerification.IsUnknown() {
		return true
	}
	var sig SignatureVerificationModel
	if diags := model.SignatureVerification.As(ctx, &sig, basetypes.ObjectAsOptions{}); diags.HasError() {
		return true
	}
	if sig.Verify.IsNull() || sig.Verify.IsUnknown() {
		return true
	}
	return sig.Verify.ValueBool()
}

func (r *PackageResource) removeComponents(ctx context.Context, plan PackageResourceModel, componentsToRemove []string, resp *resource.UpdateResponse) {
	if len(componentsToRemove) == 0 {
		return
	}

	namespaceOverride := plan.Namespace.ValueString()

	// get a reference to the k8s cluster
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	zarfCluster, err := r.cluster.NewWithWait(timeoutCtx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error loading Zarf cluster",
			"Could not Zarf cluster: "+err.Error(),
		)
		return
	}

	// get a reference to the ZarfPackage
	packageSource, err := getPackageSource(plan, *r.providerConfig)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting package source",
			"Could not get package source: "+err.Error(),
		)
		return
	}
	loadOpts := zarfPackager.LoadOptions{
		Architecture: getArchitecture(plan, *r.providerConfig),
		Filter:       r.packageFilter.ForRemove(componentsToRemove),
		CachePath:    r.providerConfig.ZarfCachePath,
	}
	pkg, err := r.packager.GetPackageFromSourceOrCluster(ctx, zarfCluster, packageSource, namespaceOverride, loadOpts)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error loading package",
			"Could not load package: "+err.Error(),
		)
		return
	}

	// Check if any of the components provided are 'required' and remove them from the list of components we are removing
	// NOTE: Just because a component block is removed from the resource spec doesn't mean it wasn't deployed. Zarf components that are marked as 'required' should not be removed
	foundRequired := false
	newComponentsToRemove := []string{}
	for _, componentName := range componentsToRemove {
		zComponent, found := findPackageComponent(pkg.Components, componentName)
		if found && zComponent.Required != nil && *zComponent.Required {
			// we are trying to remove a required component, don't do that...
			foundRequired = true
			continue
		}
		newComponentsToRemove = append(newComponentsToRemove, componentName)
	}

	// Fetch a new zarfPackage from the cluster, with a new filter of components
	if foundRequired {
		loadOpts := zarfPackager.LoadOptions{
			Architecture: getArchitecture(plan, *r.providerConfig),
			Filter:       r.packageFilter.ForRemove(newComponentsToRemove),
			CachePath:    r.providerConfig.ZarfCachePath,
		}

		pkg, err = r.packager.GetPackageFromSourceOrCluster(ctx, zarfCluster, packageSource, namespaceOverride, loadOpts)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error loading package",
				"Could not load package: "+err.Error(),
			)
			return
		}
	}

	// Remove the component from the cluster
	deleteTimeout, err := time.ParseDuration(plan.Timeout.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error parsing timeout duration",
			"Could not parse timeout duration: "+err.Error(),
		)
		return

	}
	removeOpt := zarfPackager.RemoveOptions{
		Cluster:           zarfCluster,
		Timeout:           deleteTimeout,
		NamespaceOverride: namespaceOverride,
	}
	if err := r.packager.Remove(ctx, pkg, removeOpt); err != nil {
		resp.Diagnostics.AddError(
			"Error removing components from package",
			"Could not remove components from package: "+err.Error(),
		)
		return
	}
}

func (r *PackageResource) getDeployedPackage(ctx context.Context, name string, namespace string) (zarfState.DeployedPackage, bool, error) {
	c, err := r.cluster.NewWithWait(ctx)
	if err != nil {
		return zarfState.DeployedPackage{}, false, fmt.Errorf("error connecting to cluster: %w", err)
	}

	var options []zarfState.DeployedPackageOptions
	if namespace != "" {
		options = append(options, zarfState.WithPackageNamespaceOverride(namespace))
	}

	pkg, err := c.GetDeployedPackage(ctx, name, options...)
	if err != nil {
		// "secrets <name> not found" means the package simply doesn't exist
		if strings.HasPrefix(err.Error(), "secrets ") && strings.HasSuffix(err.Error(), " not found") {
			return zarfState.DeployedPackage{}, false, nil
		}
		return zarfState.DeployedPackage{}, false, err
	}

	// Check if package was actually found (GetDeployedPackage returns a pointer)
	if pkg == nil {
		return zarfState.DeployedPackage{}, false, nil
	}

	return *pkg, true, nil
}

func (r *PackageResource) deployAsNew(ctx context.Context, plan PackageResourceModel) (PackageResourceModel, error) {

	// Get the package name from the source package metadata
	pkgLayout, err := r.getPackageLayoutFromSource(ctx, plan)
	if err != nil {
		return plan, err
	}
	defer func() {
		if cleanupErr := pkgLayout.Cleanup(); cleanupErr != nil {
			tflog.Warn(ctx, "failed to cleanup package layout", map[string]any{"error": cleanupErr.Error()})
		}
	}()

	packageNamespace := plan.Namespace.ValueString()
	packageName := pkgLayout.Pkg.Metadata.Name

	// Ensure a package with the same name and namespace is not already deployed
	clusterTimeoutCtx, cancel := withClusterTimeout(ctx)
	defer cancel()

	_, found, err := r.getDeployedPackage(clusterTimeoutCtx, packageName, packageNamespace)
	if err != nil {
		tflog.Warn(ctx, "could not check for existing package, proceeding with deploy", map[string]interface{}{"error": err.Error()})
	}
	if found {
		return plan, fmt.Errorf("package with namespace '%s' and name '%s' already exists", plan.Namespace.ValueString(), packageName)
	}

	return r.upsert(ctx, plan)
}

// TODO(erickson): Remove response parameter and return an error after refactoring removeComponents to do the same
func (r *PackageResource) deployAsNewOrUpdate(ctx context.Context, plan PackageResourceModel, oldPlan PackageResourceModel, resp *resource.UpdateResponse) (PackageResourceModel, error) {

	// TODO(erickson): Need to revisit this logic. If deploy errors, can we recover?
	// Generate list of components to remove after the update is complete.
	// Combines legacy component-block removals with optional_components removals.
	// Removal happens before upsert because removing a required component removes the entire package.
	componentsToRemove := getMissingComponents(plan, oldPlan) // TODO: remove when component block is removed
	seen := make(map[string]struct{}, len(componentsToRemove))
	for _, name := range componentsToRemove {
		seen[name] = struct{}{}
	}
	for _, name := range getMissingOptionalComponents(plan, oldPlan) {
		if _, found := seen[name]; !found {
			componentsToRemove = append(componentsToRemove, name)
		}
	}
	if len(componentsToRemove) > 0 {
		r.removeComponents(ctx, plan, componentsToRemove, resp)
		if resp.Diagnostics.HasError() {
			return plan, nil
		}
	}

	return r.upsert(ctx, plan)
}

func (r *PackageResource) upsert(ctx context.Context, plan PackageResourceModel) (PackageResourceModel, error) {
	// convert the terraform timeout to a time.Duration
	timeout, err := time.ParseDuration(plan.Timeout.ValueString())
	if err != nil {
		return plan, err
	}

	pkgLayout, err := r.getPackageLayoutFromSource(ctx, plan)
	if err != nil {
		return plan, err
	}
	defer func() {
		if cleanupErr := pkgLayout.Cleanup(); cleanupErr != nil {
			tflog.Warn(ctx, "failed to cleanup package layout", map[string]any{"error": cleanupErr.Error()})
		}
	}()

	// Alpha: if optional_components is set, use it directly; otherwise fall back to component blocks.
	// TODO: remove branch when component block is removed; always use optional_components.
	var optionalComponents []string
	if !plan.OptionalComponents.IsNull() && !plan.OptionalComponents.IsUnknown() {
		if diags := plan.OptionalComponents.ElementsAs(ctx, &optionalComponents, false); diags.HasError() {
			return plan, fmt.Errorf("failed to read optional_components: %v", diags)
		}
		if len(optionalComponents) > 0 {
			if err := validateOptionalComponentsAgainstPackage(optionalComponents, pkgLayout.Pkg.Components); err != nil {
				return plan, err
			}
		}
	} else {
		_, optionalComponents, err = getRequiredAndOptionalPackageComponentsNames(plan, pkgLayout)
		if err != nil {
			return plan, err
		}
	}

	// TODO: remove when component block is removed (components extraction, flattenComponentOverrides, and ValuesOverridesMap)
	var components []ComponentModel
	if !plan.Components.IsNull() && !plan.Components.IsUnknown() {
		plan.Components.ElementsAs(ctx, &components, false)
	}

	valuesOverridesMap, err := flattenComponentOverrides(ctx, components)
	if err != nil {
		return plan, err
	}

	// TODO(erickson): Add support for Retries, OCIConcurrency?
	deployOpts := zarfPackager.DeployOptions{
		AdoptExistingResources: false,
		SetVariables:           buildSetVariableMap(plan),
		ValuesOverridesMap:     valuesOverridesMap,
		RemoteOptions:          r.getRemoteOptions(),
		NamespaceOverride:      plan.Namespace.ValueString(),
		Timeout:                timeout,
		GitServer: zarfState.GitServerInfo{
			PushUsername: zarfState.ZarfGitPushUser,
		},
		RegistryInfo: zarfState.RegistryInfo{
			PushUsername: zarfState.ZarfRegistryPushUser,
		},
		IsInteractive:  false,
		ForceConflicts: r.providerConfig.ForceHelmSSAConflicts,
	}

	originalPkgComponents := pkgLayout.Pkg.Components
	filter := r.packageFilter.ForDeploy(optionalComponents)
	pkgLayout.Pkg.Components, err = filter.Apply(pkgLayout.Pkg)
	if err != nil {
		return plan, err
	}

	tflog.Debug(ctx, "starting deploy")
	deployResult, err := r.packager.Deploy(ctx, pkgLayout, deployOpts)
	if err != nil {
		return plan, err
	}
	tflog.Debug(ctx, "ending deploy")

	// Populate computed `set_variables` from variables returned by the
	// package deploy's VariableConfig.
	varsMap := make(map[string]attr.Value)

	if deployResult.VariableConfig != nil {
		for name, sv := range deployResult.VariableConfig.GetSetVariableMap() {
			if sv == nil {
				continue
			}
			// normalize variable name keys to all lowercase
			key := strings.ToLower(name)
			varsMap[key] = types.StringValue(sv.Value)
		}
	}

	var d diag.Diagnostics
	plan.SetVariables, d = types.MapValue(types.StringType, varsMap)
	if d.HasError() {
		return plan, fmt.Errorf("failed to construct set_variables map: %v", d)
	}

	// Populate connect strings from deploy result
	connectStrings, err := getConnectStringsFromDeployResult(deployResult)
	if err != nil {
		tflog.Warn(ctx, "failed to create connect strings set", map[string]interface{}{"error": err})
	}

	// Populate/set resource computed values so that they can be saved to state
	plan.ID = types.StringValue(computePackageID(plan.Namespace.ValueString(), pkgLayout.Pkg.Metadata.Name))
	plan.Name = types.StringValue(pkgLayout.Pkg.Metadata.Name)
	plan.Version = types.StringValue(pkgLayout.Pkg.Metadata.Version)
	plan.Kind = types.StringValue(string(pkgLayout.Pkg.Kind))
	plan.Architecture = types.StringValue(getArchitecture(plan, *r.providerConfig))
	plan.ConnectStrings = connectStrings

	pkgMetaData, err := newPackageMetadata(pkgLayout)
	if err != nil {
		return plan, err
	}
	plan.Metadata = pkgMetaData

	// Record actually deployed optional components in state when optional_components is in use.
	// Skipped for the component block path (null), which does not track optional_components in state.
	if !plan.OptionalComponents.IsNull() {
		optionals := deployedOptionalComponents(deployResult.DeployedComponents, originalPkgComponents)
		optionalVals := make([]attr.Value, len(optionals))
		for i, name := range optionals {
			optionalVals[i] = types.StringValue(name)
		}
		plan.OptionalComponents, d = types.SetValue(types.StringType, optionalVals)
		if d.HasError() {
			return plan, fmt.Errorf("failed to set optional_components from deployed components: %v", d)
		}
	}

	return plan, err
}

// convertYAMLToJSONCompatible converts YAML types (map[any]any) to JSON-compatible types (map[string]any)
func convertYAMLToJSONCompatible(o any) any {
	switch x := o.(type) {
	case map[any]any:
		m := map[string]any{}
		for k, v := range x {
			m[fmt.Sprint(k)] = convertYAMLToJSONCompatible(v)
		}
		return m
	case []any:
		for i, v := range x {
			x[i] = convertYAMLToJSONCompatible(v)
		}
	}
	return o
}

// TODO: remove when component block is removed
// convertPathValuesToOverridesMap converts helm chart path values from the Terraform model to the overrides map structure
func convertPathValuesToOverridesMap(ctx context.Context, pathValues types.Set, chartMap map[string]any, seenPaths map[string]bool, componentName, chartName, valueType string) error {
	if pathValues.IsNull() || pathValues.IsUnknown() {
		return nil
	}

	var values []HelmChartPathValueModel
	diags := pathValues.ElementsAs(ctx, &values, false)
	if diags.HasError() {
		return fmt.Errorf("failed to extract %s: %v", valueType, diags)
	}

	for _, val := range values {
		path := val.Path.ValueString()
		if _, exists := seenPaths[path]; exists {
			return fmt.Errorf("path '%s' is defined multiple times in overrides for chart '%s' of component '%s'", path, chartName, componentName)
		}
		seenPaths[path] = true

		var yamlVal any
		err := yaml.Unmarshal([]byte(val.Value.ValueString()), &yamlVal)
		if err != nil {
			return fmt.Errorf("failed to parse YAML value %s: %w", val.Value.ValueString(), err)
		}

		// Convert YAML types to JSON-compatible types
		yamlVal = convertYAMLToJSONCompatible(yamlVal)

		// Handle dot-separated keys by creating nested structure
		insertNestedValue(chartMap, path, yamlVal)
	}
	return nil
}

// TODO: remove when component block is removed
func flattenComponentOverrides(ctx context.Context, components []ComponentModel) (map[string]map[string]map[string]any, error) {
	seen := make(map[string]map[string]map[string]bool)
	result := make(map[string]map[string]map[string]any)

	for _, component := range components {
		componentName := component.Name.ValueString()

		if _, exists := seen[componentName]; exists {
			return map[string]map[string]map[string]any{}, fmt.Errorf("component '%s' is defined multiple times", componentName)
		}
		seen[componentName] = make(map[string]map[string]bool)

		// Skip if overrides is null or unknown
		if component.Overrides.IsNull() || component.Overrides.IsUnknown() {
			continue
		}

		// Extract overrides from Set
		var overrides []ComponentChartValuesModel
		diags := component.Overrides.ElementsAs(ctx, &overrides, false)
		if diags.HasError() {
			return map[string]map[string]map[string]any{}, fmt.Errorf("failed to extract overrides for component '%s': %v", componentName, diags)
		}

		// Process each chart values block within the overrides
		for _, chart := range overrides {
			chartName := chart.ChartName.ValueString()

			if _, exists := seen[componentName][chartName]; exists {
				return map[string]map[string]map[string]any{}, fmt.Errorf("chart '%s' is defined multiple times in component '%s'", chartName, componentName)
			}
			seen[componentName][chartName] = make(map[string]bool)

			// Skip if both values and sensitive_values are null or unknown
			if chart.Values.IsNull() && chart.SensitiveValues.IsNull() {
				continue
			}
			if chart.Values.IsUnknown() && chart.SensitiveValues.IsUnknown() {
				continue
			}

			// Initialize component and chart maps if they don't exist
			if _, exists := result[componentName]; !exists {
				result[componentName] = make(map[string]map[string]any)
			}
			if _, exists := result[componentName][chartName]; !exists {
				result[componentName][chartName] = make(map[string]any)
			}
			chartMap := result[componentName][chartName]

			// Process chart values (regular key-value pairs)
			if err := convertPathValuesToOverridesMap(ctx, chart.Values, chartMap, seen[componentName][chartName], componentName, chartName, "values"); err != nil {
				return map[string]map[string]map[string]any{}, err
			}

			// Process sensitive chart values (sensitive key-value pairs)
			if err := convertPathValuesToOverridesMap(ctx, chart.SensitiveValues, chartMap, seen[componentName][chartName], componentName, chartName, "sensitive values"); err != nil {
				return map[string]map[string]map[string]any{}, err
			}
		}
	}

	return result, nil
}

// getMissingComponents compares two Package plans and returns a list of components that was specified in the
// 'oldPlan' but not specified in the newer plan.
// TODO: remove when component block is removed; use getMissingOptionalComponents exclusively
func getMissingComponents(plan PackageResourceModel, oldPlan PackageResourceModel) []string {
	var componentsToRemove []string

	// Extract components from Sets
	var newComponents []ComponentModel
	if !plan.Components.IsNull() && !plan.Components.IsUnknown() {
		plan.Components.ElementsAs(context.Background(), &newComponents, false)
	}

	var oldComponents []ComponentModel
	if !oldPlan.Components.IsNull() && !oldPlan.Components.IsUnknown() {
		oldPlan.Components.ElementsAs(context.Background(), &oldComponents, false)
	}

	// Collect all component names in the new plan
	newPlanComponents := make(map[string]struct{}, len(newComponents))
	for _, component := range newComponents {
		newPlanComponents[component.Name.ValueString()] = struct{}{}
	}

	// Check which old components are missing in the new plan
	for _, component := range oldComponents {
		name := component.Name.ValueString()
		if _, found := newPlanComponents[name]; !found {
			componentsToRemove = append(componentsToRemove, name)
		}
	}

	return componentsToRemove
}

// getMissingOptionalComponents returns optional components present in oldPlan but absent from plan.
// Returns nil when either model has null/unknown optional_components (legacy component block path).
func getMissingOptionalComponents(plan, oldPlan PackageResourceModel) []string {
	if oldPlan.OptionalComponents.IsNull() || oldPlan.OptionalComponents.IsUnknown() {
		return nil
	}
	if plan.OptionalComponents.IsNull() || plan.OptionalComponents.IsUnknown() {
		return nil
	}

	var newOptionals []string
	plan.OptionalComponents.ElementsAs(context.Background(), &newOptionals, false)

	var oldOptionals []string
	oldPlan.OptionalComponents.ElementsAs(context.Background(), &oldOptionals, false)

	newSet := make(map[string]struct{}, len(newOptionals))
	for _, name := range newOptionals {
		newSet[name] = struct{}{}
	}

	var toRemove []string
	for _, name := range oldOptionals {
		if _, found := newSet[name]; !found {
			toRemove = append(toRemove, name)
		}
	}
	return toRemove
}

// withClusterTimeout returns a context with a timeout
func withClusterTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, clusterTimeoutMinutes*time.Minute)
}

// TODO: remove when component block is removed
// Inserts a nested value based on the dot-separated path
func insertNestedValue(root map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	current := root

	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
			return
		}

		// Create intermediate maps if they don't exist
		if next, exists := current[part]; exists {
			// Ensure type safety
			if nestedMap, ok := next.(map[string]any); ok {
				current = nestedMap
			} else {
				// Overwrite if the existing value is not a map
				newMap := make(map[string]any)
				current[part] = newMap
				current = newMap
			}
		} else {
			// Initialize a new map if it doesn't exist
			newMap := make(map[string]any)
			current[part] = newMap
			current = newMap
		}
	}
}

func getArchitecture(pkg PackageResourceModel, providerConfig udsProviderConfig) string {
	if !pkg.Architecture.IsNull() && !pkg.Architecture.IsUnknown() {
		return pkg.Architecture.ValueString()
	}
	return providerConfig.DefaultArchitecture
}

func getPackageSource(pkg PackageResourceModel, providerConfig udsProviderConfig) (string, error) {
	_ = providerConfig // TODO: Will be used for future local cache package download/lookup logic
	source := pkg.Source.ValueString()

	if providerConfig.LocalPathOverride != "" {
		source = filepath.Join(providerConfig.LocalPathOverride, getPackageOverrideName(pkg))
	}

	if udsValidator.ValidateOCIReferencePackageSource(source) == nil {
		// TODO: Add future local cache package download/lookup logic
		return source, nil
	}
	if udsValidator.ValidateLocalFilePathPackageSource(source) == nil {
		info, err := os.Stat(source)
		if err != nil {
			return "", err
		}
		if !info.IsDir() {
			// TODO: Add future local cache package download/lookup logic
			return source, nil
		}
	}
	return "", fmt.Errorf("invalid package source: %s. Must be a valid OCI distribution reference (including oci:// scheme) or local file path (absolute or relative)", source)
}

// getPackageOverrideName generates a deterministic tarball filename for a package based on its original source value.
// Returns a filename in the format "zarf-package-{sha1-checksum}.tar.zst".
func getPackageOverrideName(pkg PackageResourceModel) string {

	sourceString := pkg.Source.ValueString()
	if !strings.HasPrefix(sourceString, "oci://") {
		sourceString = filepath.Base(sourceString)
	}
	// Compute SHA-1 checksum
	hash := sha1.Sum([]byte(sourceString))
	sourceChecksum := hex.EncodeToString(hash[:])

	tarballNameTemplate := "zarf-package-%s.tar.zst"
	tarballName := fmt.Sprintf(tarballNameTemplate, sourceChecksum)

	return tarballName
}

func findPackageComponent(components []v1alpha1.ZarfComponent, name string) (v1alpha1.ZarfComponent, bool) {
	for _, c := range components {
		if c.Name == name {
			return c, true
		}
	}
	return v1alpha1.ZarfComponent{}, false // Not found
}

func computePackageID(namespace string, pkgName string) string {
	if namespace == "" {
		return pkgName
	}
	return fmt.Sprintf("%s:%s", namespace, pkgName)
}

func parsePackageID(id string) (namespace, pkgName string, err error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", "", fmt.Errorf("package ID cannot be empty")
	}

	parts := strings.Split(id, ":")
	switch len(parts) {
	case 1:
		return "", parts[0], nil
	case 2:
		return parts[0], parts[1], nil
	default:
		return "", "", fmt.Errorf("invalid package ID %q: expected 'name' or 'namespace:name'", id)
	}
}

// validate that the 'vars' and 'sensitive_vars' all have unique names
func validateUniqueVarNames(model PackageResourceModel, resp *resource.ValidateConfigResponse) {
	// Map of normalized name -> where it appears
	seen := map[string][]path.Path{}

	// Helper to add entries from a slice
	add := func(listName string, items []VariableModel) {
		for i, v := range items {
			// Skip unknown/null names (can’t validate yet)
			if v.Name.IsNull() || v.Name.IsUnknown() {
				continue
			}
			k := v.Name.ValueString()
			if k == "" {
				continue
			}
			k = strings.ToLower(k) // duplicates are case insensitive

			p := path.Root(listName).AtListIndex(i).AtName("name")
			seen[k] = append(seen[k], p)
		}
	}

	// Extract vars from Set
	var vars []VariableModel
	if !model.Vars.IsNull() && !model.Vars.IsUnknown() {
		model.Vars.ElementsAs(context.Background(), &vars, false)
	}
	add("vars", vars)

	// Extract sensitive_vars from Set
	var sensitiveVars []VariableModel
	if !model.SensitiveVars.IsNull() && !model.SensitiveVars.IsUnknown() {
		model.SensitiveVars.ElementsAs(context.Background(), &sensitiveVars, false)
	}
	add("sensitive_vars", sensitiveVars)

	// raise errors for any duplicates
	for name, occurrences := range seen {
		if len(occurrences) <= 1 {
			continue
		}

		resp.Diagnostics.AddError(
			"Duplicate variable name",
			fmt.Sprintf("The variable name %q is defined more than once across `vars` and `sensitive_vars`. Names must be unique.", name),
		)
	}
}

// validateSignatureVerificationAttributes validates the signature_verification block.
func validateSignatureVerificationAttributes(ctx context.Context, model PackageResourceModel, resp *resource.ValidateConfigResponse) {
	if model.SignatureVerification.IsNull() || model.SignatureVerification.IsUnknown() {
		return
	}

	var sig SignatureVerificationModel
	resp.Diagnostics.Append(model.SignatureVerification.As(ctx, &sig, basetypes.ObjectAsOptions{})...)
	if resp.Diagnostics.HasError() {
		return
	}

	if sig.Keyless.IsNull() || sig.Keyless.IsUnknown() {
		return
	}

	if !sig.PublicKey.IsNull() && !sig.PublicKey.IsUnknown() && sig.PublicKey.ValueString() != "" {
		resp.Diagnostics.AddError(
			"Conflicting signature verification configuration",
			"Cannot set both `public_key` and `keyless`. Specify one or the other.",
		)
		return
	}

	var keyless KeylessVerificationModel
	resp.Diagnostics.Append(sig.Keyless.As(ctx, &keyless, basetypes.ObjectAsOptions{})...)
	if resp.Diagnostics.HasError() {
		return
	}

	hasIdentity := !keyless.CertificateIdentity.IsNull() && keyless.CertificateIdentity.ValueString() != ""
	hasIdentityRegexp := !keyless.CertificateIdentityRegexp.IsNull() && keyless.CertificateIdentityRegexp.ValueString() != ""
	hasIssuer := !keyless.CertificateOIDCIssuer.IsNull() && keyless.CertificateOIDCIssuer.ValueString() != ""
	hasIssuerRegexp := !keyless.CertificateOIDCIssuerRegexp.IsNull() && keyless.CertificateOIDCIssuerRegexp.ValueString() != ""

	if hasIdentity && hasIdentityRegexp {
		resp.Diagnostics.AddError("Conflicting keyless verification configuration",
			"`certificate_identity` and `certificate_identity_regexp` are mutually exclusive.")
	}
	if hasIssuer && hasIssuerRegexp {
		resp.Diagnostics.AddError("Conflicting keyless verification configuration",
			"`certificate_oidc_issuer` and `certificate_oidc_issuer_regexp` are mutually exclusive.")
	}
	if !hasIdentity && !hasIdentityRegexp {
		resp.Diagnostics.AddError("Invalid keyless verification configuration",
			"`keyless` requires `certificate_identity` or `certificate_identity_regexp`.")
	}
	if !hasIssuer && !hasIssuerRegexp {
		resp.Diagnostics.AddError("Invalid keyless verification configuration",
			"`keyless` requires `certificate_oidc_issuer` or `certificate_oidc_issuer_regexp`.")
	}
}

// normalizeOptionalComponentsPlan sets plan.OptionalComponents based on what is in config:
// - Null config + no component blocks: empty set, enabling drift tracking in required-only mode.
// - Null config + component blocks present: null (legacy path; optional_components not tracked).
// - Non-null config: unchanged (config value drives the plan).
func normalizeOptionalComponentsPlan(config, plan PackageResourceModel) PackageResourceModel {
	if config.OptionalComponents.IsNull() {
		if !componentBlocksMayBePresent(config.Components) {
			plan.OptionalComponents = types.SetValueMust(types.StringType, []attr.Value{})
		} else {
			plan.OptionalComponents = types.SetNull(types.StringType) // TODO: remove branch when component block is removed
		}
	}
	return plan
}

// validateComponentBlockOptionalComponentsMutualExclusivity errors when optional_components is
// set alongside component blocks. The two paradigms are mutually exclusive.
// TODO: remove when component block is removed
func validateComponentBlockOptionalComponentsMutualExclusivity(model PackageResourceModel, resp *resource.ValidateConfigResponse) {
	if model.OptionalComponents.IsNull() {
		return
	}
	if !componentBlocksMayBePresent(model.Components) {
		return
	}
	resp.Diagnostics.AddAttributeError(
		path.Root("optional_components"),
		"Conflicting configuration",
		"`optional_components` cannot be specified together with `component` blocks. Use `optional_components` to select optional components instead.",
	)
}

// componentBlocksMayBePresent reports whether component blocks are present or
// may resolve to present once an unknown dynamic block becomes known.
// TODO: remove when component block is removed
func componentBlocksMayBePresent(components types.Set) bool {
	return !components.IsNull() && (components.IsUnknown() || len(components.Elements()) > 0)
}

// buildVerifyBlobOptions constructs signing.VerifyBlobOptions from the model.
// Writes any inline key or trusted-root content to files under tmpDir (caller owns cleanup).
// Returns nil when no verification material is configured.
func buildVerifyBlobOptions(ctx context.Context, model PackageResourceModel, tmpDir string) (*zarfSigning.VerifyBlobOptions, error) {
	// schema default ensures this is never null in practice; guard for safety (e.g. import, tests)
	if model.SignatureVerification.IsNull() || model.SignatureVerification.IsUnknown() {
		return nil, nil
	}

	var sig SignatureVerificationModel
	if diags := model.SignatureVerification.As(ctx, &sig, basetypes.ObjectAsOptions{}); diags.HasError() {
		return nil, fmt.Errorf("error reading signature_verification config")
	}

	if !sig.PublicKey.IsNull() && !sig.PublicKey.IsUnknown() && sig.PublicKey.ValueString() != "" {
		keyPath := filepath.Join(tmpDir, "key.pub")
		if err := os.WriteFile(keyPath, []byte(sig.PublicKey.ValueString()), 0600); err != nil {
			return nil, fmt.Errorf("failed to write public key file: %w", err)
		}
		opts := zarfSigning.DefaultVerifyBlobOptions()
		opts.Key = keyPath
		return &opts, nil
	}

	if !sig.Keyless.IsNull() && !sig.Keyless.IsUnknown() {
		var keyless KeylessVerificationModel
		if diags := sig.Keyless.As(ctx, &keyless, basetypes.ObjectAsOptions{}); diags.HasError() {
			return nil, fmt.Errorf("error reading keyless verification config")
		}

		opts := zarfSigning.DefaultVerifyBlobOptions()
		opts.CertVerify.CertIdentity = keyless.CertificateIdentity.ValueString()
		opts.CertVerify.CertIdentityRegexp = keyless.CertificateIdentityRegexp.ValueString()
		opts.CertVerify.CertOidcIssuer = keyless.CertificateOIDCIssuer.ValueString()
		opts.CertVerify.CertOidcIssuerRegexp = keyless.CertificateOIDCIssuerRegexp.ValueString()
		opts.CommonVerifyOptions.IgnoreTlog = keyless.InsecureIgnoreTlog.ValueBool()
		opts.CommonVerifyOptions.UseSignedTimestamps = keyless.UseSignedTimestamps.ValueBool()

		if !keyless.TrustedRoot.IsNull() && !keyless.TrustedRoot.IsUnknown() && keyless.TrustedRoot.ValueString() != "" {
			rootPath := filepath.Join(tmpDir, "trusted-root.json")
			if err := os.WriteFile(rootPath, []byte(keyless.TrustedRoot.ValueString()), 0600); err != nil {
				return nil, fmt.Errorf("failed to write trusted root file: %w", err)
			}
			opts.CommonVerifyOptions.TrustedRootPath = rootPath
		}

		return &opts, nil
	}

	return nil, nil
}

func buildSetVariableMap(model PackageResourceModel) map[string]string {
	setVariables := make(map[string]string)

	// Extract vars from Set
	var vars []VariableModel
	if !model.Vars.IsNull() && !model.Vars.IsUnknown() {
		model.Vars.ElementsAs(context.Background(), &vars, false)
		for _, v := range vars {
			setVariables[v.Name.ValueString()] = v.Value.ValueString()
		}
	}

	// Extract sensitive_vars from Set
	var sensitiveVars []VariableModel
	if !model.SensitiveVars.IsNull() && !model.SensitiveVars.IsUnknown() {
		model.SensitiveVars.ElementsAs(context.Background(), &sensitiveVars, false)
		for _, v := range sensitiveVars {
			setVariables[v.Name.ValueString()] = v.Value.ValueString()
		}
	}

	return setVariables
}

// TODO: remove when component block is removed
func getRequiredAndOptionalPackageComponentsNames(model PackageResourceModel, pkgLayout *zarfLayout.PackageLayout) (required []string, optional []string, err error) {
	var componentErrors []error
	requiredComponents := []string{}
	optionalComponents := []string{}

	// Extract components from Set
	var components []ComponentModel
	if !model.Components.IsNull() && !model.Components.IsUnknown() {
		model.Components.ElementsAs(context.Background(), &components, false)
	}

	for _, component := range components {
		pkgComponent, found := findPackageComponent(pkgLayout.Pkg.Components, component.Name.ValueString())
		if !found {
			componentErrors = append(componentErrors, fmt.Errorf("unknown package component %s", component.Name.ValueString()))
			continue
		}
		if pkgComponent.Required == nil || !*pkgComponent.Required {
			optionalComponents = append(optionalComponents, component.Name.ValueString())
		} else {
			requiredComponents = append(requiredComponents, component.Name.ValueString())
		}
	}

	if len(componentErrors) > 0 {
		return []string{}, []string{}, errors.Join(componentErrors...)
	}

	return requiredComponents, optionalComponents, nil
}

func newPackageMetadata(pkgLayout *zarfLayout.PackageLayout) (types.Object, error) {
	elementTypes := map[string]attr.Type{
		"name":        types.StringType,
		"description": types.StringType,
		"version":     types.StringType,
	}
	elements := map[string]attr.Value{
		"name":        types.StringValue(pkgLayout.Pkg.Metadata.Name),
		"description": types.StringValue(pkgLayout.Pkg.Metadata.Description),
		"version":     types.StringValue(pkgLayout.Pkg.Metadata.Version),
	}
	meta, diags := types.ObjectValue(elementTypes, elements)

	if diags.HasError() {
		diagErrors := make([]error, 0, len(diags.Errors()))
		for _, diag := range diags.Errors() {
			diagErrors = append(diagErrors, fmt.Errorf("%s: %s", diag.Summary(), diag.Detail()))
		}
		return meta, fmt.Errorf("failed to create package metadata: %w", errors.Join(diagErrors...))
	}

	return meta, nil
}

func getConnectStringsFromDeployResult(deployResult zarfPackager.DeployResult) (types.Set, error) {
	connectStrings := make(map[string]string)
	for _, component := range deployResult.DeployedComponents {
		for _, chart := range component.InstalledCharts {
			for k, v := range chart.ConnectStrings {
				connectStrings[k] = v.Description
			}
		}
	}
	return buildConnectStringsSet(connectStrings)
}

func getConnectStringsFromDeployedPackage(deployedPackage zarfState.DeployedPackage) (types.Set, error) {
	connectStrings := make(map[string]string)
	for name, connectString := range deployedPackage.ConnectStrings {
		connectStrings[name] = connectString.Description
	}
	return buildConnectStringsSet(connectStrings)
}

func buildConnectStringsSet(connectStrings map[string]string) (types.Set, error) {
	if len(connectStrings) == 0 {
		return emptyConnectStringSet(), nil
	}

	connectStringObjType := types.ObjectType{AttrTypes: connectStringAttrTypes}

	connectStringList := make([]attr.Value, 0, len(connectStrings))
	for name, description := range connectStrings {
		obj, diags := types.ObjectValue(
			connectStringAttrTypes,
			map[string]attr.Value{
				"name":        types.StringValue(name),
				"description": types.StringValue(description),
			},
		)
		if diags.HasError() {
			diagErrors := make([]error, 0, len(diags.Errors()))
			for _, diag := range diags.Errors() {
				diagErrors = append(diagErrors, fmt.Errorf("%s: %s", diag.Summary(), diag.Detail()))
			}
			return types.SetNull(connectStringObjType), errors.Join(diagErrors...)
		}
		connectStringList = append(connectStringList, obj)
	}

	setValue, diags := types.SetValue(connectStringObjType, connectStringList)
	if diags.HasError() {
		diagErrors := make([]error, 0, len(diags.Errors()))
		for _, diag := range diags.Errors() {
			diagErrors = append(diagErrors, fmt.Errorf("%s: %s", diag.Summary(), diag.Detail()))
		}
		return types.SetNull(connectStringObjType), errors.Join(diagErrors...)
	}
	return setValue, nil
}

// deployedOptionalComponents returns deployed component names that are not required
// in the package definition. Used for bi-directional drift detection in Read.
func deployedOptionalComponents(deployedComponents []zarfState.DeployedComponent, pkgComponents []v1alpha1.ZarfComponent) []string {
	optional := make([]string, 0)
	for _, dc := range deployedComponents {
		pkgComponent, found := findPackageComponent(pkgComponents, dc.Name)
		if found && (pkgComponent.Required == nil || !*pkgComponent.Required) {
			optional = append(optional, dc.Name)
		}
	}
	return optional
}

// refreshOptionalComponentsFromDeployedPackage computes the deployed optional components from cluster state.
// Returns current unchanged when it is null or unknown. No package source download is performed.
func refreshOptionalComponentsFromDeployedPackage(deployedPackage zarfState.DeployedPackage, current types.Set) (types.Set, diag.Diagnostics) {
	if current.IsNull() || current.IsUnknown() {
		return current, nil
	}
	optionals := deployedOptionalComponents(deployedPackage.DeployedComponents, deployedPackage.Data.Components)
	optionalVals := make([]attr.Value, len(optionals))
	for i, name := range optionals {
		optionalVals[i] = types.StringValue(name)
	}
	return types.SetValue(types.StringType, optionalVals)
}

func emptyConnectStringSet() types.Set {
	return types.SetValueMust(
		types.ObjectType{AttrTypes: connectStringAttrTypes},
		[]attr.Value{},
	)
}

// loadPackageLayoutForInspection loads the package for metadata inspection only, without signature verification.
// Use when only component metadata is needed (e.g., optional_components validation).
func (r *PackageResource) loadPackageLayoutForInspection(ctx context.Context, model PackageResourceModel) (*zarfLayout.PackageLayout, error) {
	packageSource, err := getPackageSource(model, *r.providerConfig)
	if err != nil {
		return nil, err
	}

	loadOpt := zarfPackager.LoadOptions{
		Filter:               zarfFilters.Empty(),
		Architecture:         getArchitecture(model, *r.providerConfig),
		VerificationStrategy: layout.VerifyNever,
		LayerTypes:           []zarfZoci.LayerType{zarfZoci.MetadataLayers},
		RemoteOptions:        r.getRemoteOptions(),
		CachePath:            r.providerConfig.ZarfCachePath,
	}

	return r.packager.LoadPackage(ctx, packageSource, loadOpt)
}

// packagePlanCheckResult holds per-check errors from runPackagePlanChecks so each can be
// attributed to the correct diagnostic path without expanding the function signature.
type packagePlanCheckResult struct {
	LoadErr          error
	SigErr           error
	OptComponentsErr error
}

// runPackagePlanChecks loads the package once and runs all plan-time validation checks.
// Skipped when source is unknown, packager/providerConfig are nil, or no checks are needed.
func (r *PackageResource) runPackagePlanChecks(ctx context.Context, plan PackageResourceModel) packagePlanCheckResult {
	if plan.Source.IsUnknown() || plan.Source.IsNull() {
		return packagePlanCheckResult{}
	}
	if r.packager == nil || r.providerConfig == nil {
		return packagePlanCheckResult{}
	}

	needsSigVerification := getEffectiveSignatureVerification(ctx, plan)
	var requestedOptionals []string
	needsOptionalValidation := !plan.OptionalComponents.IsNull() && !plan.OptionalComponents.IsUnknown() && len(plan.OptionalComponents.Elements()) > 0
	if needsOptionalValidation {
		plan.OptionalComponents.ElementsAs(ctx, &requestedOptionals, false)
	}

	if !needsSigVerification && !needsOptionalValidation {
		return packagePlanCheckResult{}
	}

	pkgLayout, err := r.loadPackageLayoutForInspection(ctx, plan)
	if pkgLayout != nil {
		defer pkgLayout.Cleanup()
	}
	if err != nil {
		return packagePlanCheckResult{LoadErr: err}
	}

	if needsSigVerification {
		tmpDir, err := os.MkdirTemp("", "uds-package-verify-*")
		if err != nil {
			return packagePlanCheckResult{SigErr: fmt.Errorf("failed to create temp dir for package verification: %w", err)}
		}
		defer os.RemoveAll(tmpDir)

		verifyBlobOpts, err := buildVerifyBlobOptions(ctx, plan, tmpDir)
		if err != nil {
			return packagePlanCheckResult{SigErr: err}
		}

		verifyOpts := zarfSigning.DefaultVerifyBlobOptions()
		if verifyBlobOpts != nil {
			verifyOpts = *verifyBlobOpts
		}
		verifyErr := pkgLayout.VerifyPackageSignature(ctx, verifyOpts)
		if sigErr := handleVerifyResult(ctx, verifyErr, pkgLayout.IsSigned(), true); sigErr != nil {
			return packagePlanCheckResult{SigErr: sigErr}
		}
	}

	if needsOptionalValidation {
		return packagePlanCheckResult{OptComponentsErr: validateOptionalComponentsAgainstPackage(requestedOptionals, pkgLayout.Pkg.Components)}
	}

	return packagePlanCheckResult{}
}

// optionalComponentsValidationError is a sentinel type so callers can distinguish
// optional_components validation failures and surface them as attribute-level diagnostics.
type optionalComponentsValidationError struct{ msg string }

func (e *optionalComponentsValidationError) Error() string { return e.msg }

// packageOptionalComponentNames returns names of non-required package components, sorted.
func packageOptionalComponentNames(pkgComponents []v1alpha1.ZarfComponent) []string {
	names := make([]string, 0)
	for _, c := range pkgComponents {
		if !c.IsRequired() {
			names = append(names, c.Name)
		}
	}
	sort.Strings(names)
	return names
}

// validateOptionalComponentsAgainstPackage checks that all requested names are valid optional
// components in the package. Returns an error listing invalid names and available optional names.
func validateOptionalComponentsAgainstPackage(requested []string, pkgComponents []v1alpha1.ZarfComponent) error {
	available := packageOptionalComponentNames(pkgComponents)
	availableSet := make(map[string]struct{}, len(available))
	for _, name := range available {
		availableSet[name] = struct{}{}
	}
	var invalid []string
	for _, name := range requested {
		if _, found := availableSet[name]; !found {
			invalid = append(invalid, name)
		}
	}
	if len(invalid) == 0 {
		return nil
	}
	if len(available) == 0 {
		return &optionalComponentsValidationError{msg: "this package defines no optional components"}
	}
	sort.Strings(invalid)
	quotedInvalid := make([]string, len(invalid))
	for i, name := range invalid {
		quotedInvalid[i] = fmt.Sprintf("%q", name)
	}
	quotedAvailable := make([]string, len(available))
	for i, name := range available {
		quotedAvailable[i] = fmt.Sprintf("%q", name)
	}
	return &optionalComponentsValidationError{msg: fmt.Sprintf("invalid optional_components: %s; available: %s",
		strings.Join(quotedInvalid, ", "), strings.Join(quotedAvailable, ", "))}
}
