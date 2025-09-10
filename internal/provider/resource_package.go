// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package provider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

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

	udsCluster "github.com/defenseunicorns/terraform-provider-uds/internal/cluster"
	udsPackager "github.com/defenseunicorns/terraform-provider-uds/internal/packager"
	udsValidator "github.com/defenseunicorns/terraform-provider-uds/internal/provider/validator"

	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	zarfConfig "github.com/zarf-dev/zarf/src/config"
	zarfPackager "github.com/zarf-dev/zarf/src/pkg/packager"
	zarfFilters "github.com/zarf-dev/zarf/src/pkg/packager/filters"
	zarfState "github.com/zarf-dev/zarf/src/pkg/state"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &PackageResource{}
	_ resource.ResourceWithImportState = &PackageResource{}
)

// NewPackageResource creates a new instance of the package resource.
func NewPackageResource(providerData *customProviderData, packager udsPackager.Packager, packageComponentFilter udsPackager.PackageComponentFilter) resource.Resource {
	if providerData == nil {
		providerData = &customProviderData{}
	}
	if packager == nil {
		packager = udsPackager.NewPackager()
	}
	if packageComponentFilter == nil {
		packageComponentFilter = udsPackager.NewPackageComponentFilter()
	}
	return &PackageResource{
		providerData:  providerData,
		packager:      packager,
		packageFilter: packageComponentFilter,
	}
}

// PackageResource defines the resource implementation.
type PackageResource struct {
	providerData  *customProviderData
	packager      udsPackager.Packager
	cluster       udsCluster.Cluster
	packageFilter udsPackager.PackageComponentFilter
}

// PackageResourceModel describes the resource data model.
type PackageResourceModel struct {
	ID                      types.String `tfsdk:"id"`
	Name                    types.String `tfsdk:"name"`
	Source                  types.String `tfsdk:"source"`
	Architecture            types.String `tfsdk:"architecture"`
	Timeout                 types.String `tfsdk:"timeout"`
	Key                     types.String `tfsdk:"key"`
	SkipSignatureValidation types.Bool   `tfsdk:"skip_signature_validation"`
	Namespace               types.String `tfsdk:"namespace"`

	Component     []ComponentModel `tfsdk:"component"`
	Overrides     []OverrideModel  `tfsdk:"overrides"`
	Vars          []VariableModel  `tfsdk:"vars"`
	SensitiveVars []VariableModel  `tfsdk:"sensitive_vars"`

	// readonly metadata
	Metadata types.Object `tfsdk:"metadata"`
	Kind     types.String `tfsdk:"kind"` // Kind reflects the type of Zarf package; either ZarfInit or ZarfPackage
	Version  types.String `tfsdk:"version"`
}

// ComponentModel represents a UDS package component configuration.
type ComponentModel struct {
	Name types.String `tfsdk:"name"`
	// TODO(erickson): Move chart overrides into component model
}

// VariableModel represents a Zarf Variable name and value pairing.
type VariableModel struct {
	Name  types.String `tfsdk:"name"`
	Value types.String `tfsdk:"value"`
}

// OverrideModel represents configuration overrides for a component.
type OverrideModel struct {
	ComponentName types.String       `tfsdk:"component_name"`
	ChartName     types.String       `tfsdk:"chart_name"`
	ValuesFiles   []types.String     `tfsdk:"values_files"`
	Values        []OverrideValue    `tfsdk:"values"`
	Variables     []OverrideVariable `tfsdk:"variables"`
}

// OverrideValue represents a single value override with a path and value.
type OverrideValue struct {
	Path  types.String `tfsdk:"path"`
	Value types.String `tfsdk:"value"`
}

// OverrideVariable represents a variable override with configuration options.
type OverrideVariable struct {
	Name        types.String `tfsdk:"name"`
	Path        types.String `tfsdk:"path"`
	Description types.String `tfsdk:"description"`
	Default     types.String `tfsdk:"default"`
}

// Metadata sets the resource type name.
func (r *PackageResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_package"
}

// Schema defines the schema for the resource.
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
				MarkdownDescription: "The name of the Zarf Package",
			},
			"source": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "OCI distribution reference (including oci:// scheme) or local file path (absolute or relative) to the package",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					udsValidator.PackageSourceValidator(),
				},
			},
			"architecture": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Architecture of the Zarf package",
				// TODO(erickson): Add validator for architecture values?
				//Validators: []validator.String{
				//	stringvalidator.OneOf("amd64", "arm64"),
				//},
			},
			"version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Version of the package that was deployed",
			},
			"key": schema.StringAttribute{
				Description: "Path or URL to the public key for signed Zarf Packages.",
				Optional:    true,
			},
			"skip_signature_validation": schema.BoolAttribute{
				Description: "Skip validating the signature of a signed Zarf package.",
				Computed:    true,
				Optional:    true,
				Default:     booldefault.StaticBool(false),
			},
			"timeout": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("30m"),
				MarkdownDescription: "Timeout for the deploy operation",
				// TODO(erickson): Add duration validator
			},
			"kind": schema.StringAttribute{
				Computed: true,
				// Optional:            true,
				MarkdownDescription: "Kind of Zarf package; ZarfInitConfig or ZarfPackageConfig",
			},
			"metadata": &schema.SingleNestedAttribute{
				Computed:    true,
				Description: "Metadata retrieved from the zarf.yaml in the package",
				Attributes: map[string]schema.Attribute{
					"name": &schema.StringAttribute{
						Computed:    true,
						Description: "Name of the zarf package. Used to identify the installed package",
					},
					"description": &schema.StringAttribute{
						Computed:    true,
						Description: "Description of the zarf package, from the zarf.yaml file",
					},
					"version": &schema.StringAttribute{
						Computed:    true,
						Description: "Version of the zarf package, from the zarf.yaml file",
					},
				},
			},
			"vars": schema.ListNestedAttribute{
				Description: "List of Zarf variables for the package.",
				Optional:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "Name of the Zarf Variable being set.",
							Required:    true,
						},
						"value": schema.StringAttribute{
							Description: "Value for the Zarf Variable.",
							Required:    true,
						},
					},
				},
				Validators: []validator.List{
					func() validator.List {
						v, _ := udsValidator.NewBlockStringAttributeUniquenessValidator("var", "name")
						return v
					}(),
				},
			},
			"sensitive_vars": schema.ListNestedAttribute{
				Description: "Sensitive Zarf Varlues",
				Optional:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "Name of the Zarf Variable being set.",
							Required:    true,
						},
						"value": schema.StringAttribute{
							Description: "Value for the Zarf Variable.",
							Required:    true,
							Sensitive:   true,
						},
					},
				},
				Validators: []validator.List{
					func() validator.List {
						v, _ := udsValidator.NewBlockStringAttributeUniquenessValidator("sensitive_var", "name")
						return v
					}(),
				},
			},
			// overrides
			"overrides": schema.ListNestedAttribute{
				Description: "List of overrides for Helm charts.",
				Optional:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"component_name": schema.StringAttribute{
							Description: "Name of the component being overridden.",
							Required:    true,
						},
						"chart_name": schema.StringAttribute{
							Description: "Name of the Helm chart being overridden.",
							Required:    true,
						},
						"values_files": schema.ListAttribute{
							Description: "List of values files to include in the override.",
							Optional:    true,
							ElementType: types.StringType,
						},
						"values": schema.ListNestedAttribute{
							Description: "List of values to override in the chart.",
							Optional:    true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"path": schema.StringAttribute{
										Description: "Path of the value to override.",
										Required:    true,
									},
									"value": schema.StringAttribute{
										Description: "Value to set at the given path.",
										Required:    true,
									},
								},
							},
						},
						"variables": schema.ListNestedAttribute{
							Description: "List of variables for the Helm chart.",
							Optional:    true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"name": schema.StringAttribute{
										Description: "Name of the variable.",
										Required:    true,
									},
									"path": schema.StringAttribute{
										Description: "Path of the variable in the Helm chart.",
										Required:    true,
									},
									"description": schema.StringAttribute{
										Description: "Description of the variable.",
										Optional:    true,
									},
									"default": schema.StringAttribute{
										Description: "Default value for the variable.",
										Optional:    true,
									},
								},
							},
						},
					},
				},
			},
			"namespace": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "[Alpha] Namespace used to override the default for package deployment.",
			},
		},
		Blocks: map[string]schema.Block{
			"component": schema.ListNestedBlock{
				MarkdownDescription: "Component configuration to include/exclude in the package deployment",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Required:    true,
							Description: "Name of the component",
						},
					},
				},
				Validators: []validator.List{
					func() validator.List {
						v, _ := udsValidator.NewBlockStringAttributeUniquenessValidator("component", "name")
						return v
					}(),
				},
			},
		},
	}
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
}

// Configure configures the resource with provider data.
func (r *PackageResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	r.providerData = req.ProviderData.(*customProviderData)

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
	plan, err = r.upsert(ctx, plan)
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

	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	c, err := r.cluster.NewWithWait(timeoutCtx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Could not connect to cluster",
			"Error connecting to cluster:"+err.Error(),
		)
		resp.State.RemoveResource(ctx)
		return
	}

	deployedZarfPackages, err := c.GetDeployedZarfPackages(timeoutCtx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Deployed packages could not be retrieved",
			"Error getting deployed packages:"+err.Error(),
		)
		resp.State.RemoveResource(ctx)
		return
	}

	// TODO: (clint) sometimes we deploy successfully but this returns empty,
	// retry might be appropriate or there may be a better way to detect this
	if len(deployedZarfPackages) == 0 {
		// try again before actually removing the resource
		time.Sleep(time.Second * 2)
		deployedZarfPackages, err = c.GetDeployedZarfPackages(timeoutCtx)
		if err != nil || len(deployedZarfPackages) == 0 {
			resp.Diagnostics.AddWarning(
				"No Packages found",
				"Could not find any packages deployed; removing resource",
			)
			resp.State.RemoveResource(ctx)
		}
		return
	}

	// Populate a matrix of all the deployed packages
	// TODO(clint): use this information to update our local state with the
	// metadata from the package we're managing
	packageData := make(map[string]zarfState.DeployedPackage)
	for _, pkg := range deployedZarfPackages {
		// var components []string

		// for _, component := range pkg.DeployedComponents {
		// 	components = append(components, component.Name)
		// }

		// packageData = append(packageData, []string{
		// 	pkg.Name, pkg.Data.Metadata.Version, fmt.Sprintf("%v", components),
		// })
		packageData[pkg.Name] = pkg
	}

	// TODO(clint): verify the package name is in this list. Right now the
	// results here are things like "init" instead of the name we supplied, so
	// we need to either dig into the package and find the metadata name, or
	// find another way to identify and ask Zarf/Kubernetes to give us the
	// package name.
	pkgUpdate, ok := packageData[strings.Trim(data.Metadata.Attributes()["name"].String(), "\"")]
	if !ok {
		resp.Diagnostics.AddWarning(
			"Package not found",
			"Could not find package in deployed packages; removing resource",
		)
		resp.State.RemoveResource(ctx)
		return
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
		"name":        types.StringValue(pkgUpdate.Name),
		"description": types.StringValue(pkgUpdate.Data.Metadata.Description),
		"version":     types.StringValue(pkgUpdate.Data.Metadata.Version),
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
	data.Version = types.StringValue(pkgUpdate.Data.Metadata.Version)

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
	componentsToRemove := getOptionalComponentsToRemove(plan, oldPlan)

	// Remove identified components
	if len(componentsToRemove) > 0 {
		r.removeComponents(ctx, plan, componentsToRemove, resp)
	}

	var err error
	plan, err = r.upsert(ctx, plan)
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
	packageSource, err := getPackageSource(data, *r.providerData)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error determine package source",
			"Could not determine package source: "+err.Error(),
		)
		return
	}

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
			"Error connecting to cluster:"+err.Error(),
		)
		return
	}

	filter := r.packageFilter.ForRemove([]string{})
	loadOpts := zarfPackager.LoadOptions{
		Architecture: getArchitecture(data, *r.providerData),
		Filter:       filter,
		CachePath:    zarfConfig.ZarfDefaultCachePath,
	}
	pkg, err := r.packager.GetPackageFromSourceOrCluster(ctx, c, packageSource, "", loadOpts)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error loading package",
			"Could not load package: "+err.Error(),
		)
		return
	}

	removeOpt := zarfPackager.RemoveOptions{
		Cluster: c,
		Timeout: deleteTimeout,
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

// ImportState imports the resource state from an external system.
func (r *PackageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func flattenOverrides(overrides []OverrideModel) map[string]map[string]map[string]any {
	result := make(map[string]map[string]map[string]any)

	for _, override := range overrides {
		component := override.ComponentName.ValueString()
		chart := override.ChartName.ValueString()

		// Initialize nested maps if they don't exist
		if _, exists := result[component]; !exists {
			result[component] = make(map[string]map[string]any)
		}
		if _, exists := result[component][chart]; !exists {
			result[component][chart] = make(map[string]any)
		}

		chartMap := result[component][chart]

		// Flatten Values
		for _, v := range override.Values {
			chartMap[v.Path.ValueString()] = v.Value.ValueString()
		}

		// Flatten Variables into Nested Maps
		for _, variable := range override.Variables {
			defaultValue := variable.Default.ValueString()
			path := variable.Path.ValueString()

			if defaultValue != "" {
				insertNestedValue(chartMap, path, defaultValue)
			} else {
				// Handle deletion if the default value is empty
				deleteNestedValue(chartMap, path)
			}
		}
	}

	return result
}

// getOptionalComponentsToRemove compares two Package plans and returns a list of components that was specified in the
// 'oldPlan' but not specified in the newer plan.
func getOptionalComponentsToRemove(plan PackageResourceModel, oldPlan PackageResourceModel) []string {
	var componentsToRemove []string

	// Collect all component names in the new plan
	newPlanComponents := make(map[string]struct{}, len(plan.Component))
	for _, component := range plan.Component {
		newPlanComponents[component.Name.ValueString()] = struct{}{}
	}

	// Check which old components are missing in the new plan
	for _, component := range oldPlan.Component {
		name := component.Name.ValueString()
		if _, found := newPlanComponents[name]; !found {
			componentsToRemove = append(componentsToRemove, name)
		}
	}

	return componentsToRemove
}

func (r *PackageResource) removeComponents(ctx context.Context, plan PackageResourceModel, componentsToRemove []string, resp *resource.UpdateResponse) {
	if len(componentsToRemove) == 0 {
		return
	}
	namespaceOverride := plan.Namespace.ValueString()

	filter := r.packageFilter.ForRemove(componentsToRemove)

	// load the zarf package & cluster
	loadOpts := zarfPackager.LoadOptions{
		Architecture: getArchitecture(plan, *r.providerData),
		Filter:       filter,
		CachePath:    zarfConfig.ZarfDefaultCachePath,
	}

	packageSource, err := getPackageSource(plan, *r.providerData)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting package source",
			"Could not get package source: "+err.Error(),
		)
		return
	}

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

	pkg, err := r.packager.GetPackageFromSourceOrCluster(ctx, zarfCluster, packageSource, namespaceOverride, loadOpts)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error loading package",
			"Could not load package: "+err.Error(),
		)
		return
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

// Inserts a nested value based on the dot-separated path
func insertNestedValue(root map[string]any, path, value string) {
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

// Deletes a nested value based on the dot-separated path
func deleteNestedValue(root map[string]any, path string) {
	parts := strings.Split(path, ".")
	current := root

	for i, part := range parts {
		if i == len(parts)-1 {
			delete(current, part)
			return
		}

		next, exists := current[part]
		if !exists {
			return // Path doesn't exist, nothing to delete
		}

		// Ensure type safety
		nestedMap, ok := next.(map[string]any)
		if !ok {
			return // Invalid structure, cannot proceed
		}
		current = nestedMap
	}
}

func (r *PackageResource) upsert(ctx context.Context, plan PackageResourceModel) (PackageResourceModel, error) {
	// Set the prefix for `./zarf` actions since we have to vendor zarf, otherwise Zarf actions will not run.
	// Confirm `zarf package deploy` since we're running in automation
	zarfConfig.ActionsCommandZarfPrefix = "zarf"
	zarfConfig.CommonOptions.Confirm = true

	// convert the terraform timeout to a time.Duration
	timeout, err := time.ParseDuration(plan.Timeout.ValueString())
	if err != nil {
		return plan, err
	}

	valuesMap := flattenOverrides(plan.Overrides)
	packageSource, err := getPackageSource(plan, *r.providerData)
	if err != nil {
		return plan, err
	}

	// TODO(erickson): Do we need configurable remote options?
	remoteOpts := zarfPackager.RemoteOptions{
		PlainHTTP:             zarfConfig.CommonOptions.PlainHTTP,
		InsecureSkipTLSVerify: zarfConfig.CommonOptions.InsecureSkipTLSVerify,
	}

	// TODO(erickson): Add support for Shasum, CachePath, OCIConcurrency?
	loadOpt := zarfPackager.LoadOptions{
		Filter:                  zarfFilters.Empty(),
		Architecture:            getArchitecture(plan, *r.providerData),
		PublicKeyPath:           plan.Key.ValueString(),
		SkipSignatureValidation: plan.SkipSignatureValidation.ValueBool(),
		RemoteOptions:           remoteOpts,
		CachePath:               zarfConfig.ZarfDefaultCachePath,
	}

	pkgLayout, err := r.packager.LoadPackage(ctx, packageSource, loadOpt)
	if err != nil {
		return plan, err
	}
	defer func() {
		err = errors.Join(err, pkgLayout.Cleanup())
	}()

	var componentErrors []error
	optionalComponents := []string{}
	for _, component := range plan.Component {
		pkgComponent, found := findPackageComponent(pkgLayout.Pkg.Components, component.Name.ValueString())
		if !found {
			componentErrors = append(componentErrors, fmt.Errorf("component %s not found in package", component.Name.ValueString()))
			continue
		}
		if len(componentErrors) == 0 && pkgComponent.Required == nil || !*pkgComponent.Required {
			optionalComponents = append(optionalComponents, component.Name.ValueString())
		}
	}

	if len(componentErrors) > 0 {
		return plan, errors.Join(componentErrors...)
	}

	setVariables := make(map[string]string)
	for _, zarfVar := range plan.Vars {
		setVariables[zarfVar.Name.ValueString()] = zarfVar.Value.ValueString()
	}
	for _, sensitiveVar := range plan.SensitiveVars {
		setVariables[sensitiveVar.Name.ValueString()] = sensitiveVar.Value.ValueString()
	}

	// TODO(erickson): Add support for Retries, OCIConcurrency?
	deployOpts := zarfPackager.DeployOptions{
		SetVariables:           setVariables,
		AdoptExistingResources: false,
		Timeout:                timeout,
		RemoteOptions:          remoteOpts,
		NamespaceOverride:      plan.Namespace.ValueString(),
		GitServer: zarfState.GitServerInfo{
			PushUsername: zarfState.ZarfGitPushUser,
		},
		RegistryInfo: zarfState.RegistryInfo{
			PushUsername: zarfState.ZarfRegistryPushUser,
		},
		ValuesOverridesMap: valuesMap,
	}

	filter := r.packageFilter.ForDeploy(optionalComponents)
	pkgLayout.Pkg.Components, err = filter.Apply(pkgLayout.Pkg)
	if err != nil {
		return plan, err
	}

	// Log components to enable for package deployment based on filter
	tflog.Debug(ctx, fmt.Sprintf("%d components to include for package deployment:", len(pkgLayout.Pkg.Components)))
	for _, component := range pkgLayout.Pkg.Components {
		requiredStr := "nil"
		if component.Required != nil {
			requiredStr = fmt.Sprintf("%t", *component.Required)
		}
		tflog.Debug(ctx, fmt.Sprintf("include component: name=%s, required=%s, default=%t",
			component.Name, requiredStr, component.Default))
	}

	tflog.Debug(ctx, "starting deploy")
	_, err = r.packager.Deploy(ctx, pkgLayout, deployOpts)
	if err != nil {
		return plan, err
	}
	tflog.Debug(ctx, "ending deploy")

	// Populate/set resource computed values
	plan.ID = types.StringValue(computePackageID(plan.Namespace.ValueString(), pkgLayout.Pkg.Metadata.Name))
	plan.Version = types.StringValue(pkgLayout.Pkg.Metadata.Version)
	plan.Kind = types.StringValue(string(pkgLayout.Pkg.Kind))

	// populate the package metadata type.
	// TODO(clint): this is ugly and I got it from https://developer.hashicorp.com/terraform/plugin/framework/handling-data/types/custom
	// There are probably a few optimizations or cleanups to be done here.
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
	pkgMetaData, diags := types.ObjectValue(elementTypes, elements)

	if diags.HasError() {
		return plan, err
	}
	plan.Metadata = pkgMetaData

	return plan, err
}

func getArchitecture(pkg PackageResourceModel, providerData customProviderData) string {
	if providerData.BundleArch != "" {
		return providerData.BundleArch
	}
	return pkg.Architecture.ValueString()
}

func getPackageSource(pkg PackageResourceModel, providerData customProviderData) (string, error) {
	_ = providerData // TODO: Will be used for future local cache package download/lookup logic
	source := pkg.Source.ValueString()

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

	add("vars", model.Vars)
	add("sensitive_vars", model.SensitiveVars)

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
