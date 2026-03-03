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
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"gopkg.in/yaml.v2"

	udsCluster "github.com/defenseunicorns/terraform-provider-uds/internal/cluster"
	"github.com/defenseunicorns/terraform-provider-uds/internal/fileutil"
	udsPackager "github.com/defenseunicorns/terraform-provider-uds/internal/packager"
	udsValidator "github.com/defenseunicorns/terraform-provider-uds/internal/provider/validator"

	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	zarfPackager "github.com/zarf-dev/zarf/src/pkg/packager"
	zarfFilters "github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	zarfLayout "github.com/zarf-dev/zarf/src/pkg/packager/layout"
	zarfState "github.com/zarf-dev/zarf/src/pkg/state"
	zarfUtils "github.com/zarf-dev/zarf/src/pkg/utils"
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
	ID                      types.String `tfsdk:"id"`
	Source                  types.String `tfsdk:"source"`
	Architecture            types.String `tfsdk:"architecture"`
	Timeout                 types.String `tfsdk:"timeout"`
	PublicKey               types.String `tfsdk:"public_key"`
	SkipSignatureValidation types.Bool   `tfsdk:"skip_signature_validation"`
	VerifySignature         types.Bool   `tfsdk:"verify_signature"`
	Namespace               types.String `tfsdk:"namespace"`

	Components    types.Set `tfsdk:"component"`      // Set of ComponentModel objects
	Vars          types.Set `tfsdk:"vars"`           // Set of VariableModel objects
	SensitiveVars types.Set `tfsdk:"sensitive_vars"` // Set of VariableModel objects

	// readonly metadata
	Name           types.String `tfsdk:"name"`
	Kind           types.String `tfsdk:"kind"` // Kind reflects the type of UDS package; either ZarfInit or ZarfPackage
	Version        types.String `tfsdk:"version"`
	Metadata       types.Object `tfsdk:"metadata"`
	ConnectStrings types.Set    `tfsdk:"connect_strings"` // Set of ConnectString objects
	// export_vars is a user-provided set of variable names that should be exported
	// from the package during deploy. These names instruct the provider to look up
	// values that were set during action transforms or by the package runtime and
	// make them available to other packages.
	ExportVars types.Set `tfsdk:"export_vars"`

	// exported_vars is a computed, read-only map containing the values of variables
	// requested via `export_vars`. The provider populates this map after a
	// successful deploy. It is intentionally marked sensitive to avoid leaking
	// potentially-secret runtime values in plan/state output. Callers should
	// reference this map via module outputs (e.g. `module.foo.exported_vars["NAME"]`).
	ExportedVars types.Map `tfsdk:"exported_vars"`

	// When true, the provider will tolerate a missing deployed package during
	// refresh/read operations and will keep the Terraform state instead of
	// removing the resource. This is useful for packages that only execute
	// actions (no persistent cluster objects) and therefore may not have a
	// persisted deployed-package record in Zarf. Default: false
	TolerateMissingDeployed types.Bool `tfsdk:"tolerate_missing_deployed"`
}

// ComponentModel represents a UDS package component configuration.
type ComponentModel struct {
	Name      types.String `tfsdk:"name"`
	Overrides types.Set    `tfsdk:"override"` // Set of ComponentChartValuesModel objects
}

// ComponentChartValuesModel represents a helm chart override values configuration for a package component.
type ComponentChartValuesModel struct {
	ChartName       types.String `tfsdk:"chart_name"`
	Values          types.Set    `tfsdk:"values"`           // Set of HelmChartPathValueModel objects
	SensitiveValues types.Set    `tfsdk:"sensitive_values"` // Set of HelmChartPathValueModel objects
}

// HelmChartPathValueModel represents a path/value pair for setting helm chart values
type HelmChartPathValueModel struct {
	Path  types.String `tfsdk:"path"`
	Value types.String `tfsdk:"value"`
}

// VariableModel represents a name/value pair for setting UDS package variables
type VariableModel struct {
	Name  types.String `tfsdk:"name"`
	Value types.String `tfsdk:"value"`
}

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
			"public_key": schema.StringAttribute{
				MarkdownDescription: "Raw public key value to validate against a signed UDS package.",
				Optional:            true,
			},
			// TODO: Remove skip_signature_validation attribute in subsequent release
			"skip_signature_validation": schema.BoolAttribute{
				MarkdownDescription: "Skip validating the signature of a signed UDS package.",
				DeprecationMessage:  "This attribute is deprecated. Use `verify_signature` instead. The `skip_signature_validation` attribute will be removed in a future version.",
				Computed:            true,
				Optional:            true,
				Default:             booldefault.StaticBool(false),
			},
			"verify_signature": schema.BoolAttribute{
				MarkdownDescription: "Verify the signature of a UDS package. When enabled, a signed package with an invalid or missing signature will fail to deploy. When disabled, the package will continue to deploy with signature verification issues logged as warnings.",
				Computed:            true,
				Optional:            true,
				Default:             booldefault.StaticBool(true),
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
			"export_vars": schema.SetAttribute{
				MarkdownDescription: "Set of variable names to export from the package deploy.",
				Optional:            true,
				ElementType:         types.StringType,
			},
			"exported_vars": schema.MapAttribute{
				MarkdownDescription: "Read-only map of exported variable names to values.",
				Computed:            true,
				ElementType:         types.StringType,
				Sensitive:           true,
			},
			"tolerate_missing_deployed": schema.BoolAttribute{
				MarkdownDescription: "When true, keep the Terraform state if the deployed package record is not found instead of removing the resource.",
				Optional:            true,
			},
		},
		Blocks: map[string]schema.Block{
			"component": schema.SetNestedBlock{
				MarkdownDescription: "Component configuration to include/exclude in the UDS package deployment.",
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
	// Hook used in tests to override deployed package lookup behavior.
	getDeployedPackageFunc func(ctx context.Context, name string, namespace string) (zarfState.DeployedPackage, bool, error)
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
	validateSignatureVerificationAttributes(model, resp)
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
		resp.Diagnostics.AddError(
			"Error creating package",
			"Could not create resource, unexpected error: "+err.Error(),
		)
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

	var deployedPackage zarfState.DeployedPackage
	var found bool
	if r.getDeployedPackageFunc != nil {
		deployedPackage, found, err = r.getDeployedPackageFunc(timeoutCtx, packageName, packageNamespace)
	} else {
		deployedPackage, found, err = r.getDeployedPackage(timeoutCtx, packageName, packageNamespace)
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting deployed package",
			"Failed to get deployed package: "+err.Error(),
		)
		return
	}
	if !found {
		// If the resource is configured to tolerate missing deployed records,
		// keep the Terraform state and emit a warning. This is useful for
		// packages that only perform actions and do not create persistent
		// deployed-package records in Zarf.
		if !data.TolerateMissingDeployed.IsNull() && data.TolerateMissingDeployed.ValueBool() {
			resp.Diagnostics.AddWarning(
				"Deployed package not found (tolerated)",
				"Could not find deployed package with name "+packageName+"; keeping Terraform state because `tolerate_missing_deployed` is set",
			)
			return
		}

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
		resp.Diagnostics.AddError(
			"Error updating package",
			"Could not update package, unexpected error: "+err.Error(),
		)
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

	publicKeyPath, err := getTempPublicKeyPath(data.PublicKey.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Public key path error",
			"Could not get/create public key path: "+err.Error(),
		)
		return
	}
	defer func() {
		if publicKeyPath != "" {
			os.Remove(publicKeyPath)
		}
	}()

	// TODO(erickson): Do we need configurable remote options?
	remoteOpts := zarfTypes.RemoteOptions{
		PlainHTTP:             r.providerConfig.InsecureForceHTTP,
		InsecureSkipTLSVerify: r.providerConfig.InsecureSkipTLSVerification,
	}

	loadOpts := zarfPackager.LoadOptions{
		Filter:               r.packageFilter.ForRemove([]string{}),
		Architecture:         getArchitecture(data, *r.providerConfig),
		PublicKeyPath:        publicKeyPath,
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

	// Migrate signature verification attributes
	syncSignatureVerificationAttributes(ctx, &config, &plan)

	resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
}

// syncSignatureVerificationAttributes syncs skip_signature_validation and verify_signature attributes
// to ensure they are consistent in the plan.
func syncSignatureVerificationAttributes(ctx context.Context, config *PackageResourceModel, plan *PackageResourceModel) {
	effective := getEffectiveSignatureVerification(*config)

	plan.VerifySignature = types.BoolValue(effective)
	plan.SkipSignatureValidation = types.BoolValue(!effective)

	tflog.Debug(ctx, "Synchronized signature verification attributes", map[string]any{
		"verify_signature":          plan.VerifySignature.ValueBool(),
		"skip_signature_validation": plan.SkipSignatureValidation.ValueBool(),
	})
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

	// generate a temporary public key file if needed
	publicKeyPath, err := getTempPublicKeyPath(model.PublicKey.ValueString())
	if err != nil {
		return nil, err
	}
	defer func() {
		if publicKeyPath != "" {
			os.Remove(publicKeyPath)
		}
	}()

	// TODO(erickson): add support for Shasum, CachePath, OCIConcurrency?
	loadOpt := zarfPackager.LoadOptions{
		Filter:               zarfFilters.Empty(),
		Architecture:         getArchitecture(model, *r.providerConfig),
		PublicKeyPath:        publicKeyPath,
		VerificationStrategy: layout.VerifyNever,
		RemoteOptions:        r.getRemoteOptions(),
		CachePath:            r.providerConfig.ZarfCachePath,
	}

	pkgLayout, err := r.packager.LoadPackage(ctx, packageSource, loadOpt)
	if err != nil {
		return nil, err
	}

	// Verify package signature
	enforceSignatureVerification := getEffectiveSignatureVerification(model)
	verifyOpts := zarfUtils.DefaultVerifyBlobOptions()
	verifyOpts.KeyRef = publicKeyPath
	err = pkgLayout.VerifyPackageSignature(ctx, verifyOpts)
	if err != nil {
		// Error only if package is signed and enforcing signature verification
		if enforceSignatureVerification && pkgLayout.IsSigned() {
			return nil, err
		}

		// Only warn if package is unsigned or not enforcing signature verification
		tflog.Warn(ctx, "package signature could not be verified", map[string]any{"error": err.Error()})
	}

	return pkgLayout, nil
}

// getEffectiveSignatureVerification determines whether to verify package signatures based on the
// verify_signature and (deprecated) skip_signature_validation attributes.
func getEffectiveSignatureVerification(model PackageResourceModel) bool {
	// Use verify_signature if set and skip_signature_validation not set
	if !model.VerifySignature.IsNull() && !model.VerifySignature.IsUnknown() {
		return model.VerifySignature.ValueBool()
	}

	// Use skip_signature_validation if set and verify_signature not set
	if !model.SkipSignatureValidation.IsNull() && !model.SkipSignatureValidation.IsUnknown() {
		return !model.SkipSignatureValidation.ValueBool()
	}

	// Verify by default
	return true
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
		err = errors.Join(err, pkgLayout.Cleanup())
	}()

	packageNamespace := plan.Namespace.ValueString()
	packageName := pkgLayout.Pkg.Metadata.Name

	// Ensure a package with the same name and namespace is not already deployed
	clusterTimeoutCtx, cancel := withClusterTimeout(ctx)
	defer cancel()

	_, found, err := r.getDeployedPackage(clusterTimeoutCtx, packageName, packageNamespace)
	if err != nil {
		return plan, err
	}
	if found {
		return plan, fmt.Errorf("package with namespace '%s' and name '%s' already exists", plan.Namespace.ValueString(), packageName)
	}

	return r.upsert(ctx, plan)
}

// TODO(erickson): Remove response parameter and return an error after refactoring removeComponents to do the same
func (r *PackageResource) deployAsNewOrUpdate(ctx context.Context, plan PackageResourceModel, oldPlan PackageResourceModel, resp *resource.UpdateResponse) (PackageResourceModel, error) {

	// TODO(erickson): Need to revisit this logic. If deploy errors, can we recover?
	// Generate list of components to remove after the update is complete
	// These components are components that were defined in the old plan but not present in the current plan
	// We are removing them after the 'update' because if a 'required' component is removed it removes the entire package
	componentsToRemove := getMissingComponents(plan, oldPlan)
	if len(componentsToRemove) > 0 {
		r.removeComponents(ctx, plan, componentsToRemove, resp)
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
		err = errors.Join(err, pkgLayout.Cleanup())
	}()

	_, optionalComponents, err := getRequiredAndOptionalPackageComponentsNames(plan, pkgLayout)
	if err != nil {
		return plan, err
	}

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
		IsInteractive: false,
	}

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

	// If the plan asked for exported variables, build the exported_vars map from the deploy result.
	if !plan.ExportVars.IsNull() && !plan.ExportVars.IsUnknown() {
		var exportNames []string
		if diags := plan.ExportVars.ElementsAs(ctx, &exportNames, false); diags.HasError() {
			// Could not parse export names; set empty map
			plan.ExportedVars, _ = types.MapValue(types.StringType, map[string]attr.Value{})
		} else {
			exportedMap := make(map[string]attr.Value)
			for _, name := range exportNames {
				found := false
				// try set variables from VariableConfig
				if deployResult.VariableConfig != nil {
					if sv, ok := deployResult.VariableConfig.GetSetVariable(name); ok {
						exportedMap[name] = types.StringValue(sv.Value)
						found = true
					} else if sv2, ok2 := deployResult.VariableConfig.GetSetVariable(strings.ToUpper(name)); ok2 {
						exportedMap[name] = types.StringValue(sv2.Value)
						found = true
					}
				}

				// try deploy values (may be structured); marshal non-strings to YAML
				if !found && deployResult.Values != nil {
					if v, ok := deployResult.Values[name]; ok {
						switch t := v.(type) {
						case string:
							exportedMap[name] = types.StringValue(t)
						default:
							b, _ := yaml.Marshal(t)
							exportedMap[name] = types.StringValue(strings.TrimSpace(string(b)))
						}
						found = true
					} else if v, ok := deployResult.Values[strings.ToUpper(name)]; ok {
						switch t := v.(type) {
						case string:
							exportedMap[name] = types.StringValue(t)
						default:
							b, _ := yaml.Marshal(t)
							exportedMap[name] = types.StringValue(strings.TrimSpace(string(b)))
						}
						found = true
					}
				}
			}

			plan.ExportedVars, _ = types.MapValue(types.StringType, exportedMap)
		}
	} else {
		// ensure exported_vars is an empty map when not requested
		plan.ExportedVars, _ = types.MapValue(types.StringType, map[string]attr.Value{})
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

// withClusterTimeout returns a context with a timeout
func withClusterTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, clusterTimeoutMinutes*time.Minute)
}

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

// validateSignatureVerificationAttributes validates that verify_signature and (deprecated) skip_signature_validation
// don't have conflicting values when both are explicitly set.
func validateSignatureVerificationAttributes(model PackageResourceModel, resp *resource.ValidateConfigResponse) {
	// Only need to validate if both attributes are explicitly set
	if model.SkipSignatureValidation.IsNull() || model.SkipSignatureValidation.IsUnknown() || model.VerifySignature.IsNull() || model.VerifySignature.IsUnknown() {
		return
	}

	// Conflict when both are true or both are false
	skip := model.SkipSignatureValidation.ValueBool()
	verify := model.VerifySignature.ValueBool()
	if skip == verify {
		resp.Diagnostics.AddError(
			"Conflicting signature verification configuration",
			fmt.Sprintf(
				"The attributes 'skip_signature_validation=%t' and 'verify_signature=%t' have conflicting values. "+
					"Please use only 'verify_signature' (recommended) or ensure both attributes are consistent. "+
					"Note: 'skip_signature_validation' is deprecated and will be removed in a future version.",
				skip, verify,
			),
		)
	}
}

func getTempPublicKeyPath(publicKey string) (string, error) {
	if publicKey == "" {
		return "", nil
	}

	publicKeyPath, err := fileutil.CreateTempPublicKeyFile(publicKey)
	if err != nil {
		return "", err
	}

	return publicKeyPath, nil
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

	connectStringObjType := types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"name":        types.StringType,
			"description": types.StringType,
		},
	}

	connectStringList := make([]attr.Value, 0, len(connectStrings))
	for name, description := range connectStrings {
		obj, diags := types.ObjectValue(
			map[string]attr.Type{
				"name":        types.StringType,
				"description": types.StringType,
			},
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

func emptyConnectStringSet() types.Set {
	return types.SetValueMust(
		types.ObjectType{
			AttrTypes: map[string]attr.Type{
				"name":        types.StringType,
				"description": types.StringType,
			},
		},
		[]attr.Value{}, // empty slice
	)
}
