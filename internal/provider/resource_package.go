// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	zarfTypes "github.com/zarf-dev/zarf/src/types"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &PackageResource{}
	_ resource.ResourceWithImportState = &PackageResource{}
)

func NewPackageResource() resource.Resource {
	return &PackageResource{}
}

// PackageResource defines the resource implementation.
type PackageResource struct{}

// PackageResourceModel describes the resource data model.
type PackageResourceModel struct {
	ID types.String `tfsdk:"id"`
	// Name       types.String `tfsdk:"name"`
	Components types.List   `tfsdk:"components"`
	Timeout    types.String `tfsdk:"timeout"`
	// Kind reflects the type of Zarf package; either ZarfInit or ZarfPackage
	Kind       types.String `tfsdk:"kind"`
	Path       types.String `tfsdk:"path"`
	Repository types.String `tfsdk:"repository"`
	Version    types.String `tfsdk:"version"`

	// readonly metadata
	Metadata types.Object `tfsdk:"metadata"`
	// others, probably read-only as well, from the ZarfPackage type:
	// Variables
	// Components

	//	map[string]map[string]map[string]interface{}
	Overrides []OverrideModel `tfsdk:"overrides"`
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
			"path": schema.StringAttribute{
				Optional:            true, // TODO(clint): make this optional and support local file
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
			"version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Version of the package that was deployed",
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
				Computed:            true,
				Optional:            true,
				MarkdownDescription: "Kind of Zarf package; ZarfInitConfig or ZarfPackageConfig",
			},

			"metadata": &schema.SingleNestedAttribute{
				// Optional:    true,
				Computed:    true,
				Description: "Metadata retrieved from the zarf.yaml in the package",
				// PlanModifiers: []planmodifier.Object{
				// 	objectplanmodifier.RequiresReplace(),
				// },
				Attributes: map[string]schema.Attribute{
					"name": &schema.StringAttribute{
						Optional:    true,
						Computed:    true,
						Description: "Name of the zarf package. Used to identify the installed package",
					},
					"description": &schema.StringAttribute{
						Computed:    true,
						Description: "Description of the zarf package, from the zarf.yaml file",
					},
					"version": &schema.StringAttribute{
						Optional:    true,
						Description: "Version of the zarf package, from the zarf.yaml file",
						// TODO(clint): make reading a different value here
						// force the recreation
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.RequiresReplace(),
						},
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

func (r *PackageResource) Configure(_ context.Context, _ resource.ConfigureRequest, _ *resource.ConfigureResponse) {
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

	thing := flattenOverrides(data.Overrides)
	deployOpts := zarfTypes.ZarfDeployOptions{
		Timeout:            timeout,
		ValuesOverridesMap: thing,
	}

	pkgOpts := zarfTypes.ZarfPackageOptions{
		// Load the package path from the terraform plan
		PackageSource: data.Path.ValueString(),

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
	pkgConfig.PkgOpts.PackageSource = data.Path.ValueString()

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
	pkgClient, err := zarfPackager.New(&pkgConfig, zarfPackager.WithContext(ctx))
	// Abort if we can't create the package client
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating package client",
			"Could not create client: "+err.Error(),
		)
		return
	}
	// Always clear the temp paths since we're running in automation
	defer pkgClient.ClearTempPaths()

	tflog.Debug(ctx, "starting deploy")
	if err := pkgClient.Deploy(ctx); err != nil {
		resp.Diagnostics.AddError(
			"Error deploying package",
			"Could not deploy package: "+err.Error(),
		)
		return
	}
	tflog.Debug(ctx, "ending deploy")

	data.ID = types.StringValue(data.Path.ValueString())
	data.Kind = types.StringValue(string(pkgConfig.Pkg.Kind))

	// populate the package metadata type.
	// TODO(clint): this is ugly and I got it from https://developer.hashicorp.com/terraform/plugin/framework/handling-data/types/custom
	// There are probably a few optimizations or cleanups to be done here.
	elementTypes := map[string]attr.Type{
		"name":        types.StringType,
		"description": types.StringType,
		"version":     types.StringType,
	}
	elements := map[string]attr.Value{
		"name":        types.StringValue(pkgConfig.Pkg.Metadata.Name),
		"description": types.StringValue(pkgConfig.Pkg.Metadata.Description),
		"version":     types.StringValue(pkgConfig.Pkg.Metadata.Version),
	}
	pkgMetaData, diags := types.ObjectValue(elementTypes, elements)

	if diags.HasError() {
		resp.Diagnostics.AddError(
			"Error converting type to package metadata",
			"Could not convert: "+fmt.Sprintf("%v", diags),
		)
		return
	}
	data.Metadata = pkgMetaData

	// explicitly set the version
	data.Version = types.StringValue(pkgConfig.Pkg.Metadata.Version)

	tflog.Trace(ctx, "created zarf package resource")

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

	c, err := zarfCluster.NewClusterWithWait(timeoutCtx)
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

	if len(deployedZarfPackages) == 0 {
		resp.Diagnostics.AddWarning(
			"No Packages found",
			"Could not find any packages deployed; removing resource",
		)
		resp.State.RemoveResource(ctx)
		return
	}

	// Populate a matrix of all the deployed packages
	// TODO(clint): use this information to update our local state with the
	// metadata from the package we're managing
	packageData := make(map[string]zarfTypes.DeployedPackage)
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

// TODO(clint): We probably wont have much of an actual Update method
// implementation; I imagine any drift we find will be flagged as a re-create
// scenario which would just redeploy the package.
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
		PackageSource: data.Path.ValueString(),
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

	// TODO(clint): copied this from uds-cli, but I'm not sure if it's needed
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

func flattenOverrides(overrides []OverrideModel) map[string]map[string]map[string]interface{} {
	result := make(map[string]map[string]map[string]interface{})

	for _, override := range overrides {
		// Initialize the nested maps if not already done
		if _, exists := result[override.ComponentName.ValueString()]; !exists {
			result[override.ComponentName.ValueString()] = make(map[string]map[string]interface{})
		}

		componentMap := result[override.ComponentName.ValueString()]
		if _, exists := componentMap[override.ChartName.ValueString()]; !exists {
			componentMap[override.ChartName.ValueString()] = make(map[string]interface{})
		}

		chartMap := componentMap[override.ChartName.ValueString()]

		// Flatten Values
		if override.Values != nil {
			for _, value := range override.Values {
				path := value.Path.ValueString()
				chartMap[path] = value.Value.ValueString()
			}
		}

		// Flatten Variables into Nested Map
		if override.Variables != nil {
			for _, variable := range override.Variables {
				path := variable.Path.ValueString()
				// Create nested maps based on the path
				pathParts := strings.Split(path, ".")
				currentMap := chartMap

				for i, part := range pathParts {
					if i == len(pathParts)-1 {
						// Last part of the path, set the value
						currentMap[part] = variable.Default.ValueString()
					} else {
						// Intermediate parts, create maps if necessary
						if _, exists := currentMap[part]; !exists {
							currentMap[part] = make(map[string]interface{})
						}
						currentMap = currentMap[part].(map[string]interface{})
					}
				}
			}
		}
	}

	return result
}
