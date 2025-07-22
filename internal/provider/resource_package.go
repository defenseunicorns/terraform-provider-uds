// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package provider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	udsPackager "github.com/defenseunicorns/terraform-provider-uds/internal/packager"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	zarfConfig "github.com/zarf-dev/zarf/src/config"
	zarfCluster "github.com/zarf-dev/zarf/src/pkg/cluster"
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
// If packager is nil, a new DefaultZarfPackager will be created.
func NewPackageResource(packager udsPackager.ZarfPackager) resource.Resource {
	if packager == nil {
		packager = udsPackager.NewZarfPackager()
	}
	return &PackageResource{
		packager: packager,
	}
}

// PackageResource defines the resource implementation.
type PackageResource struct {
	providerData *customProviderData
	packager     udsPackager.ZarfPackager
}

// PackageResourceModel describes the resource data model.
type PackageResourceModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	Components types.List   `tfsdk:"components"`
	Timeout    types.String `tfsdk:"timeout"`
	// Kind reflects the type of Zarf package; either ZarfInit or ZarfPackage
	Kind       types.String `tfsdk:"kind"`
	Path       types.String `tfsdk:"path"`
	Repository types.String `tfsdk:"repository"`
	Ref        types.String `tfsdk:"ref"`

	Key types.String `tfsdk:"key"`

	// readonly metadata
	Metadata  types.Object    `tfsdk:"metadata"`
	Overrides []OverrideModel `tfsdk:"overrides"`

	Architecture types.String `tfsdk:"architecture"`
}

type OverrideModel struct {
	ComponentName types.String       `tfsdk:"component_name"`
	ChartName     types.String       `tfsdk:"chart_name"`
	ValuesFiles   []types.String     `tfsdk:"values_files"`
	Values        []OverrideValue    `tfsdk:"values"`
	Variables     []OverrideVariable `tfsdk:"variables"`
}

type OverrideValue struct {
	Path  types.String `tfsdk:"path"`
	Value types.String `tfsdk:"value"`
}

type OverrideVariable struct {
	Name        types.String `tfsdk:"name"`
	Path        types.String `tfsdk:"path"`
	Description types.String `tfsdk:"description"`
	Default     types.String `tfsdk:"default"`
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
				MarkdownDescription: "The name of the Zarf Package",
			},
			"path": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Path to tar file of the package",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					// Validate at least this attribute or other_attr should be configured.
					stringvalidator.ExactlyOneOf(path.Expressions{
						path.MatchRoot("repository"),
						path.MatchRoot("path"),
					}...),
				},
			},
			"repository": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "url to the repository of the package",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"ref": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Red of the package that was deployed",
			},
			"key": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Path to the public key for signed Zarf Packages",
			},
			"architecture": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Architecture of the Zarf package",
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
		},
	}
}

func (r *PackageResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	r.providerData = req.ProviderData.(*customProviderData)

	// Initialize the packager if it wasn't set in NewPackageResource
	if r.packager == nil {
		r.packager = udsPackager.NewZarfPackager()
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

	c, err := zarfCluster.NewWithWait(timeoutCtx)
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
	data.Ref = types.StringValue(pkgUpdate.Data.Metadata.Version)

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
	c, err := zarfCluster.NewWithWait(timeoutCtx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Could not connect to cluster",
			"Error connecting to cluster:"+err.Error(),
		)
		return
	}

	filter := zarfFilters.Combine(
		zarfFilters.ByLocalOS(runtime.GOOS),
	)
	loadOpts := zarfPackager.LoadOptions{
		Architecture: getArchitecture(data, *r.providerData),
		Filter:       filter,
	}
	pkg, err := zarfPackager.GetPackageFromSourceOrCluster(ctx, c, packageSource, "", loadOpts)
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
	if err := zarfPackager.Remove(ctx, pkg, removeOpt); err != nil {
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

func flattenOverrides(overrides []OverrideModel) map[string]map[string]map[string]interface{} {
	result := make(map[string]map[string]map[string]interface{})

	for _, override := range overrides {
		component := override.ComponentName.ValueString()
		chart := override.ChartName.ValueString()

		// Initialize nested maps if they don't exist
		if _, exists := result[component]; !exists {
			result[component] = make(map[string]map[string]interface{})
		}
		if _, exists := result[component][chart]; !exists {
			result[component][chart] = make(map[string]interface{})
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

// Inserts a nested value based on the dot-separated path
func insertNestedValue(root map[string]interface{}, path, value string) {
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
			if nestedMap, ok := next.(map[string]interface{}); ok {
				current = nestedMap
			} else {
				// Overwrite if the existing value is not a map
				newMap := make(map[string]interface{})
				current[part] = newMap
				current = newMap
			}
		} else {
			// Initialize a new map if it doesn't exist
			newMap := make(map[string]interface{})
			current[part] = newMap
			current = newMap
		}
	}
}

// Deletes a nested value based on the dot-separated path
func deleteNestedValue(root map[string]interface{}, path string) {
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
		nestedMap, ok := next.(map[string]interface{})
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
	sourcePath, err := getPackageSource(plan, *r.providerData)
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
		SkipSignatureValidation: false, // TODO(erickson): Make this configurable?
		RemoteOptions:           remoteOpts,
	}

	pkgLayout, err := zarfPackager.LoadPackage(ctx, sourcePath, loadOpt)
	if err != nil {
		return plan, err
	}
	defer func() {
		err = errors.Join(err, pkgLayout.Cleanup())
	}()

	// TODO(erickson): Add support for Retries, OCIConcurrency, NamespaceOverride?
	deployOpts := zarfPackager.DeployOptions{
		AdoptExistingResources: false,
		Timeout:                timeout,
		RemoteOptions:          remoteOpts,
		GitServer: zarfState.GitServerInfo{
			PushUsername: zarfState.ZarfGitPushUser,
		},
		RegistryInfo: zarfState.RegistryInfo{
			PushUsername: zarfState.ZarfRegistryPushUser,
		},
		ValuesOverridesMap: valuesMap,
	}
	components := "" // TODO(erickson): Placeholder for (Optional) components currently not working. Fixed in another branch
	filter := zarfFilters.Combine(
		zarfFilters.ByLocalOS(runtime.GOOS),
		zarfFilters.ForDeploy(components, false),
	)
	pkgLayout.Pkg.Components, err = filter.Apply(pkgLayout.Pkg)
	if err != nil {
		return plan, err
	}

	tflog.Debug(ctx, "starting deploy")
	_, err = zarfPackager.Deploy(ctx, pkgLayout, deployOpts)
	if err != nil {
		return plan, err
	}
	tflog.Debug(ctx, "ending deploy")

	plan.ID = types.StringValue(plan.Path.ValueString())
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

	// explicitly set the version
	plan.Ref = types.StringValue(pkgLayout.Pkg.Metadata.Version)
	return plan, err
}

func getArchitecture(pkg PackageResourceModel, providerData customProviderData) string {
	if providerData.BundleArch != "" {
		return providerData.BundleArch
	}
	return pkg.Architecture.ValueString()
}

func getPackageSource(pkg PackageResourceModel, providerData customProviderData) (string, error) {
	packageTarballName := getPackageName(pkg, providerData.BundleArch)
	sourcePath := ""

	// Determine the proper sourcePath depending on provided overrides from UDS-CLI
	if providerData != (customProviderData{}) {
		// Check if UDS CLI sent overrides we need to use
		sourcePath = filepath.Join(providerData.LocalPathOverride, packageTarballName)
	} else if pkg.Repository.ValueString() != "" {
		// Generate the oci schema based string from the provided repository
		sourcePath = fmt.Sprintf("oci://%s:%s", pkg.Repository.ValueString(), pkg.Ref.ValueString())
	} else if pkg.Path.ValueString() != "" {
		// Generate a path to the zarf package tarball
		sourcePath = pkg.Path.ValueString()
		info, err := os.Stat(sourcePath)
		if err != nil {
			return "", err
		}
		if info.IsDir() {
			sourcePath = filepath.Join(sourcePath, packageTarballName)
		}
	}
	return sourcePath, nil
}

func getPackageName(pkg PackageResourceModel, archOverride string) string {
	tarballName := ""
	packageName := pkg.Name.ValueString()
	tarballArch := pkg.Architecture.ValueString()
	tarballRef := pkg.Ref.ValueString()

	if archOverride != "" {
		tarballArch = archOverride
	}

	// zarf-init packages are 'special' and don't have the 'zarf-package' prefix
	tarballNameTemplate := "zarf-package-%s-%s-%s.tar.zst"
	if packageName == "init" {
		tarballNameTemplate = "zarf-%s-%s-%s.tar.zst"
	}

	tarballName = fmt.Sprintf(tarballNameTemplate,
		packageName,
		tarballArch,
		tarballRef)

	return tarballName
}
