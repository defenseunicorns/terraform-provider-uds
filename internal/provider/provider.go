// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package provider implements the UDS Terraform provider.
package provider

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type customProviderData struct {
	LocalPathOverride           string
	BundleArch                  string
	InsecureForceHTTP           bool
	InsecureSkipTLSVerification bool
}

type udsProviderModel struct {
	InsecureForceHTTP     types.Bool `tfsdk:"insecure_force_http"`
	InsecureSkipTLSVerify types.Bool `tfsdk:"insecure_skip_tls_verification"`
}

// New creates a new provider factory function that returns a provider instance.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &udsProvider{
			version: version,
		}
	}
}

var _ provider.Provider = (*udsProvider)(nil)

type udsProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

func (p *udsProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"insecure_force_http": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Force remote package fetching over HTTP instead of HTTPS. Defaults to `false`. Can also be configured with the `UDS_INSECURE_FORCE_HTTP` environment variable.",
			},
			"insecure_skip_tls_verification": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Skip TLS certificate verification when fetching remote packages over HTTPS. Defaults to `false`. Can also be configured with the `UDS_INSECURE_SKIP_TLS_VERIFICATION` environment variable.",
			},
		},
	}
}

func (p *udsProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	// Load the HTTP and TLS environment variables
	forceHTTP, err := envBool("UDS_INSECURE_FORCE_HTTP")
	if err != nil {
		resp.Diagnostics.AddError("Invalid environment variable value", err.Error())
	}
	skipTLS, err := envBool("UDS_INSECURE_SKIP_TLS_VERIFICATION")
	if err != nil {
		resp.Diagnostics.AddError("Invalid environment variable value", err.Error())
	}
	if resp.Diagnostics.HasError() {
		return
	}

	var config udsProviderModel
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The setting in the config takes presidense over settings from an env var
	if !config.InsecureForceHTTP.IsNull() && config.InsecureForceHTTP.IsUnknown() {
		forceHTTP = config.InsecureForceHTTP.ValueBool()
	}
	if !config.InsecureSkipTLSVerify.IsNull() {
		skipTLS = config.InsecureSkipTLSVerify.ValueBool()
	}

	customData := customProviderData{
		LocalPathOverride:           os.Getenv("UDS_LOCAL_PATH_OVERRIDE"),
		BundleArch:                  os.Getenv("UDS_BUNDLE_ARCH"),
		InsecureForceHTTP:           forceHTTP,
		InsecureSkipTLSVerification: skipTLS,
	}

	resp.DataSourceData = &customData
	resp.ResourceData = &customData
}

func envBool(key string) (bool, error) {
	value, exists := os.LookupEnv(key)
	if !exists || value == "" {
		return false, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean value, received %q", key, value)
	}
	return parsed, nil
}

func (p *udsProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		// Pass the packager to NewPackageResource
		func() resource.Resource { return NewPackageResource(nil, nil, nil, nil) },
		NewBundleMetadataResource,
	}
}

func (p *udsProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}

func (p *udsProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "uds"
}
