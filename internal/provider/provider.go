// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package provider implements the UDS Terraform provider.
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

type customProviderData struct {
	LocalPathOverride string
	BundleArch        string
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

func (p *udsProvider) Schema(context.Context, provider.SchemaRequest, *provider.SchemaResponse) {
}

func (p *udsProvider) Configure(_ context.Context, _ provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	customData := customProviderData{
		LocalPathOverride: os.Getenv("UDS_LOCAL_PATH_OVERRIDE"),
		BundleArch:        os.Getenv("UDS_BUNDLE_ARCH"),
	}

	resp.DataSourceData = &customData
	resp.ResourceData = &customData
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
