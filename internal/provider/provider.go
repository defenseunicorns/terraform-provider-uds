// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package provider implements the UDS Terraform provider.
package provider

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	zarfConfig "github.com/zarf-dev/zarf/src/config"
)

// configSource represents where a configuration value came from
type configSource string

const (
	configSourceProvider                         configSource = "provider block"
	configSourceEnvironment                      configSource = "environment variable"
	configSourceDefault                          configSource = "default value"
	configErrorSummaryUnknownProviderConfigValue string       = "Unknown Provider Config Value"
	configErrorSummaryInvalidProviderConfigValue string       = "Invalid Provider Config Value"
)

type udsProviderConfig struct {
	LocalPathOverride           string // TODO(erickson): Revisit if this is needed
	DefaultArchitecture         string
	InsecureForceHTTP           bool
	InsecureSkipTLSVerification bool
	ZarfCachePath               string
	ForceHelmSSAConflicts       bool
}

type udsProviderModel struct {
	DefaultArchitecture   types.String `tfsdk:"default_architecture"`
	InsecureForceHTTP     types.Bool   `tfsdk:"insecure_force_http"`
	InsecureSkipTLSVerify types.Bool   `tfsdk:"insecure_skip_tls_verification"`
	ZarfCachePath         types.String `tfsdk:"zarf_cache_path"`
	ForceHelmSSAConflicts types.Bool   `tfsdk:"force_helm_ssa_conflicts"`
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
			"default_architecture": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Default system architecture of the target cluster. Valid values are `amd64` or `arm64`. Defaults to the local system architecture. Can also be configured with the `UDS_DEFAULT_ARCHITECTURE` environment variable.",
			},
			"insecure_force_http": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Force remote package fetching over HTTP instead of HTTPS. Defaults to `false`. Can also be configured with the `UDS_INSECURE_FORCE_HTTP` environment variable.",
			},
			"insecure_skip_tls_verification": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Skip TLS certificate verification when fetching remote packages over HTTPS. Defaults to `false`. Can also be configured with the `UDS_INSECURE_SKIP_TLS_VERIFICATION` environment variable.",
			},
			"zarf_cache_path": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Filesystem path to the local Zarf cache directory. Defaults to `~/.zarf-cache`. Can also be configured with the `UDS_ZARF_CACHE_PATH` environment variable.",
			},
			"force_helm_ssa_conflicts": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Force Helm to take ownership of conflicting fields during Server-Side Apply operations during package deployment. Use when external tools (kubectl, HPAs, etc.) have modified resources. Defaults to `false`. Can also be configured with the `UDS_FORCE_HELM_SSA_CONFLICTS` environment variable.",
			},
		},
	}
}

func (p *udsProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config udsProviderModel
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Provided configuration values must be known
	if config.DefaultArchitecture.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("default_architecture"),
			configErrorSummaryUnknownProviderConfigValue,
			"Unknown configuration value for the default architecture. Either target apply the source of the value first, set the value statically in the configuration, or use the UDS_DEFAULT_ARCHITECTURE environment variable.",
		)
	}
	if config.InsecureForceHTTP.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("insecure_force_http"),
			configErrorSummaryUnknownProviderConfigValue,
			"Unknown configuration value for the insecure force HTTP flag. Either target apply the source of the value first, set the value statically in the configuration, or use the UDS_INSECURE_FORCE_HTTP environment variable.",
		)
	}
	if config.InsecureSkipTLSVerify.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("insecure_skip_tls_verification"),
			configErrorSummaryUnknownProviderConfigValue,
			"Unknown configuration value for the insecure skip TLS verification flag. Either target apply the source of the value first, set the value statically in the configuration, or use the UDS_INSECURE_SKIP_TLS_VERIFICATION environment variable.",
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}
	if config.ZarfCachePath.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("zarf_cache_path"),
			configErrorSummaryUnknownProviderConfigValue,
			"Unknown configuration value for the zarf cache path. Either target apply the source of the value first, set the value statically in the configuration, or use the UDS_ZARF_CACHE_PATH environment variable.",
		)
	}
	if config.ForceHelmSSAConflicts.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("force_helm_ssa_conflicts"),
			configErrorSummaryUnknownProviderConfigValue,
			"Unknown configuration value for the force Helm SSA conflicts flag. Either target apply the source of the value first, set the value statically in the configuration, or use the UDS_FORCE_HELM_SSA_CONFLICTS environment variable.",
		)
	}

	defaultArchitecture, _, err := resolveProviderStringConfig(ctx, config.DefaultArchitecture, "UDS_DEFAULT_ARCHITECTURE", runtime.GOARCH, "default_architecture",
		strings.ToLower, // transformer
		func(value string) error { // validator
			if !isValidArchitecture(value) {
				return fmt.Errorf("architecture must be `amd64` or `arm64`, received %q", value)
			}
			return nil
		},
	)
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("default_architecture"), configErrorSummaryInvalidProviderConfigValue, err.Error())
		return
	}

	// Resolve insecure_force_http
	forceHTTP, _, err := resolveProviderBoolConfig(ctx, config.InsecureForceHTTP, "UDS_INSECURE_FORCE_HTTP", false, "insecure_force_http", nil)
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("insecure_force_http"), configErrorSummaryInvalidProviderConfigValue, err.Error())
		return
	}

	// Resolve insecure_skip_tls_verification
	skipTLS, _, err := resolveProviderBoolConfig(ctx, config.InsecureSkipTLSVerify, "UDS_INSECURE_SKIP_TLS_VERIFICATION", false, "insecure_skip_tls_verification", nil)
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("insecure_skip_tls_verification"), configErrorSummaryInvalidProviderConfigValue, err.Error())
		return
	}

	// Resolve force_helm_ssa_conflicts
	forceHelmSSAConflicts, _, err := resolveProviderBoolConfig(ctx, config.ForceHelmSSAConflicts, "UDS_FORCE_HELM_SSA_CONFLICTS", false, "force_helm_ssa_conflicts", nil)
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("force_helm_ssa_conflicts"), configErrorSummaryInvalidProviderConfigValue, err.Error())
		return
	}

	// Resolve zarf_cache_path
	zarfCachePath, _, err := resolveProviderStringConfig(
		ctx,
		config.ZarfCachePath,
		"UDS_ZARF_CACHE_PATH",
		zarfConfig.ZarfDefaultCachePath,
		"zarf_cache_path",
		nil,
		nil,
	)
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("zarf_cache_path"), configErrorSummaryInvalidProviderConfigValue, err.Error())
		return
	}
	zarfConfig.CommonOptions.CachePath = zarfCachePath
	zarfCachePath, err = zarfConfig.GetAbsCachePath()
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("zarf_cache_path"), configErrorSummaryInvalidProviderConfigValue, fmt.Sprintf("Could not resolve Zarf cache path: %s", err))
		return
	}
	zarfConfig.CommonOptions.CachePath = zarfCachePath

	providerCfg := udsProviderConfig{
		LocalPathOverride:           os.Getenv("UDS_LOCAL_PATH_OVERRIDE"),
		DefaultArchitecture:         defaultArchitecture,
		InsecureForceHTTP:           forceHTTP,
		InsecureSkipTLSVerification: skipTLS,
		ZarfCachePath:               zarfCachePath,
		ForceHelmSSAConflicts:       forceHelmSSAConflicts,
	}

	tflog.Info(ctx, "Provider configured", map[string]interface{}{
		"default_architecture":           defaultArchitecture,
		"insecure_force_http":            forceHTTP,
		"insecure_skip_tls_verification": skipTLS,
		"zarf_cache_path":                zarfCachePath,
		"force_helm_ssa_conflicts":       forceHelmSSAConflicts,
	})

	resp.DataSourceData = &providerCfg
	resp.ResourceData = &providerCfg
}

// resolveProviderStringConfig resolves a string configuration value from provider config, environment variable, or default.
// It applies an optional transformer function and validator function.
func resolveProviderStringConfig(
	ctx context.Context,
	configValue types.String,
	envVarName string,
	defaultValue string,
	configName string,
	transformer func(string) string,
	validator func(string) error,
) (value string, source configSource, err error) {
	if !configValue.IsNull() {
		value = configValue.ValueString()
		if transformer != nil {
			value = transformer(value)
		}
		source = configSourceProvider
	} else {
		envValue, found := os.LookupEnv(envVarName)
		if found {
			value = envValue
			if transformer != nil {
				value = transformer(value)
			}
			source = configSourceEnvironment
		} else {
			value = defaultValue
			source = configSourceDefault
		}
	}

	if validator != nil {
		if err := validator(value); err != nil {
			return "", source, err
		}
	}

	tflog.Debug(ctx, fmt.Sprintf("Configured %s from %s", configName, source), map[string]any{
		configName: value,
	})

	return value, source, nil
}

// resolveBoolConfig resolves a boolean configuration value from provider config, environment variable, or default.
// It applies an optional validator function.
func resolveProviderBoolConfig(
	ctx context.Context,
	configValue types.Bool,
	envVarName string,
	defaultValue bool,
	configName string,
	validator func(bool) error,
) (value bool, source configSource, err error) {
	if !configValue.IsNull() {
		value = configValue.ValueBool()
		source = configSourceProvider
	} else {
		envValue, found := os.LookupEnv(envVarName)
		if found {
			parsed, parseErr := strconv.ParseBool(envValue)
			if parseErr != nil {
				return false, configSourceEnvironment, fmt.Errorf("%s must be a boolean value, received %q", envVarName, envValue)
			}
			value = parsed
			source = configSourceEnvironment
		} else {
			value = defaultValue
			source = configSourceDefault
		}
	}

	if validator != nil {
		if err := validator(value); err != nil {
			return false, source, err
		}
	}

	tflog.Debug(ctx, fmt.Sprintf("Configured %s from %s", configName, source), map[string]any{
		configName: value,
	})

	return value, source, nil
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

func isValidArchitecture(arch string) bool {
	return arch == "amd64" || arch == "arm64"
}
