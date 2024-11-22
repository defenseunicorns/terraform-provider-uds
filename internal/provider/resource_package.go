// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package provider

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	goyaml "github.com/goccy/go-yaml"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/mholt/archiver/v3"

	zarfV1alpha1 "github.com/zarf-dev/zarf/src/api/v1alpha1"
	zarfConfig "github.com/zarf-dev/zarf/src/config"
	zarfCluster "github.com/zarf-dev/zarf/src/pkg/cluster"
	zarfPackager "github.com/zarf-dev/zarf/src/pkg/packager"
	zarfUtils "github.com/zarf-dev/zarf/src/pkg/utils"
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
	// Kind reflects the type of Zarf package; either ZarfInit or ZarfPackage
	Kind types.String `tfsdk:"kind"`

	// readonly metadata
	Metadata types.Object `tfsdk:"metadata"`
	// others, probably read-only as well, from the ZarfPackage type:
	// Variables
	// Components
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

			"kind": schema.StringAttribute{
				Computed:            true,
				Optional:            true,
				MarkdownDescription: "Kind of Zarf package; ZarfInitConfig or ZarfPackageConfig",
			},

			"metadata": &schema.SingleNestedAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Metadata retrieved from the zarf.yaml in the package",
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
						Computed:    true,
						Description: "Version of the zarf package, from the zarf.yaml file",
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

	// load package metadata so we know what kind of package to create
	loadOpt := loadOptions{
		Source: data.Name.String(),
	}

	layout, err := loadPackage(ctx, loadOpt)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error loading package metadata",
			"Could not load package metadata from file: "+err.Error(),
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
	if layout.Pkg.Kind == zarfV1alpha1.ZarfInitConfig {
		pkgConfig.InitOpts = zarfTypes.ZarfInitOptions{
			GitServer: zarfTypes.GitServerInfo{
				PushUsername: zarfTypes.ZarfGitPushUser,
			},
			RegistryInfo: zarfTypes.RegistryInfo{
				PushUsername: zarfTypes.ZarfRegistryPushUser,
			},
		}
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

	data.ID = types.StringValue(data.Name.ValueString())
	data.Kind = types.StringValue(string(layout.Pkg.Kind))

	// populate the package metadata type.
	// TODO(clint): this is ugly and I got it from https://developer.hashicorp.com/terraform/plugin/framework/handling-data/types/custom
	// There are probably a few optimizations or cleanups to be done here.
	elementTypes := map[string]attr.Type{
		"name":        types.StringType,
		"description": types.StringType,
		"version":     types.StringType,
	}
	elements := map[string]attr.Value{
		"name":        types.StringValue(layout.Pkg.Metadata.Name),
		"description": types.StringValue(layout.Pkg.Metadata.Description),
		"version":     types.StringValue(layout.Pkg.Metadata.Version),
	}
	objectValue, diags := types.ObjectValue(elementTypes, elements)

	if diags.HasError() {
		resp.Diagnostics.AddError(
			"Error converting type to package metadata",
			"Could not convert: "+fmt.Sprintf("%v", diags),
		)
		return
	}
	data.Metadata = objectValue

	// Documentation: https://terraform.io/plugin/log
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

	// remove the package from state if it's not found in the list
	// TODO(clint): solve the mystery of why does the attribute come back with
	// extra quotes in the string?
	foundPackage := slices.ContainsFunc(deployedZarfPackages, func(zarfPackage zarfTypes.DeployedPackage) bool {
		return zarfPackage.Name == strings.Trim(data.Metadata.Attributes()["name"].String(), "\"")
	})
	if !foundPackage {
		resp.Diagnostics.AddWarning(
			"Package not found",
			"Could not find package in deployed packages; removing resource",
		)
		resp.State.RemoveResource(ctx)
		return
	}

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

//TODO(clint): the below methods could/should be moved to a util type file. They
//were all lifted from Zarf Packager2 internal package and modified to fit our
//needs here.

// packageLayout manages the layout for a package.
// Lifted from Zarf
type packageLayout struct {
	dirPath string
	Pkg     zarfV1alpha1.ZarfPackage
}

// loadOptions are the options for LoadPackage.
type loadOptions struct {
	Source string
	// Shasum                  string
	// PublicKeyPath           string
	// SkipSignatureValidation bool
	// Filter                  filters.ComponentFilterStrategy
}

// packageLayoutOptions are the options used when loading a package.
type packageLayoutOptions struct {
	PublicKeyPath           string
	SkipSignatureValidation bool
	IsPartial               bool
}

func loadPackage(ctx context.Context, opt loadOptions) (*packageLayout, error) {
	tmpDir, err := zarfUtils.MakeTempDir("")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpDir)
	// TODO(clint): tarpath temp dir isn't ever really used?
	// tarPath := filepath.Join(tmpDir, "data.tar.zst")
	tarPath := opt.Source

	layoutOpt := packageLayoutOptions{
		// PublicKeyPath:           opt.PublicKeyPath,
		// SkipSignatureValidation: opt.SkipSignatureValidation,
		// IsPartial:               isPartial,
	}
	pkgLayout, err := loadFromTar(ctx, tarPath, layoutOpt)
	if err != nil {
		return nil, err
	}
	return pkgLayout, nil
}

// loadFromTar unpacks the give compressed package and loads it.
func loadFromTar(ctx context.Context, tarPath string, opt packageLayoutOptions) (*packageLayout, error) {
	dirPath, err := zarfUtils.MakeTempDir(zarfConfig.CommonOptions.TempDirectory)
	if err != nil {
		return nil, err
	}

	//TODO(clint): find out why the path here has leading and trailing quotes
	tarPath = strings.Trim(tarPath, "\"")
	err = archiver.Walk(tarPath, func(f archiver.File) error {
		if f.IsDir() {
			return nil
		}
		header, ok := f.Header.(*tar.Header)
		if !ok {
			return fmt.Errorf("expected header to be *tar.Header but was %T", f.Header)
		}
		// If path has nested directories we want to create them.
		dir := filepath.Dir(header.Name)
		if dir != "." {
			err := os.MkdirAll(filepath.Join(dirPath, dir), 0755)
			if err != nil {
				return err
			}
		}
		dst, err := os.Create(filepath.Join(dirPath, header.Name))
		if err != nil {
			return err
		}
		defer dst.Close()
		_, err = io.Copy(dst, f)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	p, err := loadFromDir(ctx, dirPath, opt)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// loadFromDir loads and validates a package from the given directory path.
// some unused vars here because at this time we're not doing some of the other
// validation things.
func loadFromDir(_ context.Context, dirPath string, _ packageLayoutOptions) (*packageLayout, error) {
	b, err := os.ReadFile(filepath.Join(dirPath, "zarf.yaml"))
	if err != nil {
		return nil, err
	}
	pkg, err := parseZarfPackage(b)
	if err != nil {
		return nil, err
	}
	pkgLayout := &packageLayout{
		dirPath: dirPath,
		Pkg:     pkg,
	}
	// err = validatePackageIntegrity(pkgLayout, opt.IsPartial)
	// if err != nil {
	// 	return nil, err
	// }
	// err = validatePackageSignature(ctx, pkgLayout, opt.PublicKeyPath, opt.SkipSignatureValidation)
	// if err != nil {
	// 	return nil, err
	// }
	return pkgLayout, nil
}

// parseZarfPackage parses the yaml passed as a byte slice and applies potential schema migrations.
func parseZarfPackage(b []byte) (zarfV1alpha1.ZarfPackage, error) {
	var pkg zarfV1alpha1.ZarfPackage
	err := goyaml.Unmarshal(b, &pkg)
	if err != nil {
		return zarfV1alpha1.ZarfPackage{}, err
	}
	// if len(pkg.Build.Migrations) > 0 {
	// 	for idx, component := range pkg.Components {
	// 		pkg.Components[idx], _ = deprecated.MigrateComponent(pkg.Build, component)
	// 	}
	// }
	return pkg, nil
}
