// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)


func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &udsProvider{
			version: version,
		}
	}
}

var _ provider.Provider = (*udsProvider)(nil)

type udsProvider struct{
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

func (p *udsProvider) Schema(context.Context, provider.SchemaRequest, *provider.SchemaResponse) {
}

func (p *udsProvider) Configure(context.Context, provider.ConfigureRequest, *provider.ConfigureResponse) {
}

func (p *udsProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewPackageResource,
	}
}

func (p *udsProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewPackageDataSource,
	}
}

func (p *udsProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "uds"
}
