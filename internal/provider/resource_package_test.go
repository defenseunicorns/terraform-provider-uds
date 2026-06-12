// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package provider

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/defaults"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	zarfCluster "github.com/zarf-dev/zarf/src/pkg/cluster"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	zarfPackager "github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	zarfSigning "github.com/zarf-dev/zarf/src/pkg/signing"
	zarfState "github.com/zarf-dev/zarf/src/pkg/state"
	zarfValue "github.com/zarf-dev/zarf/src/pkg/value"
	"github.com/zarf-dev/zarf/src/pkg/variables"
	zarfZoci "github.com/zarf-dev/zarf/src/pkg/zoci"

	udsPackager "github.com/defenseunicorns/terraform-provider-uds/internal/packager"
)

type MockCluster struct {
	mock.Mock
}

func (m *MockCluster) NewWithWait(ctx context.Context) (*zarfCluster.Cluster, error) {
	args := m.Called(ctx)
	return args.Get(0).(*zarfCluster.Cluster), args.Error(1)
}

type MockPackager struct {
	mock.Mock
}

func (m *MockPackager) Deploy(ctx context.Context, pkgLayout *layout.PackageLayout, opts packager.DeployOptions) (packager.DeployResult, error) {
	args := m.Called(ctx, pkgLayout, opts)
	return args.Get(0).(packager.DeployResult), args.Error(1)
}

func (m *MockPackager) Remove(ctx context.Context, pkg v1alpha1.ZarfPackage, opts packager.RemoveOptions) error {
	args := m.Called(ctx, pkg, opts)
	return args.Error(0)
}

func (m *MockPackager) LoadPackage(ctx context.Context, source string, opts packager.LoadOptions) (*layout.PackageLayout, error) {
	args := m.Called(ctx, source, opts)
	return args.Get(0).(*layout.PackageLayout), args.Error(1)
}

func (m *MockPackager) GetPackageFromSourceOrCluster(ctx context.Context, cluster *zarfCluster.Cluster, src string, namespaceOverride string, opts zarfPackager.LoadOptions) (_ v1alpha1.ZarfPackage, err error) {
	args := m.Called(ctx, cluster, src, namespaceOverride, opts)
	return args.Get(0).(v1alpha1.ZarfPackage), args.Error(1)
}

type MockPackageComponentFilter struct {
	mock.Mock

	packageComponentFilter udsPackager.PackageComponentFilter
}

func (m *MockPackageComponentFilter) ForRemove(optionalComponents []string) filters.ComponentFilterStrategy {
	m.Called(optionalComponents)
	return m.getPackageComponentFilter().ForRemove(optionalComponents)
}

func (m *MockPackageComponentFilter) ForDeploy(optionalComponents []string) filters.ComponentFilterStrategy {
	m.Called(optionalComponents)
	return m.getPackageComponentFilter().ForDeploy(optionalComponents)
}

func (m *MockPackageComponentFilter) getPackageComponentFilter() udsPackager.PackageComponentFilter {
	if m.packageComponentFilter == nil {
		m.packageComponentFilter = udsPackager.NewPackageComponentFilter()
	}
	return m.packageComponentFilter
}

type MockLoadPackageResult struct {
	Layout *layout.PackageLayout
	Error  error
}

type PackageResourceModelDataOption func(*PackageResourceModel)

func WithSource(source string) PackageResourceModelDataOption {
	return func(model *PackageResourceModel) {
		model.Source = types.StringValue(source)
	}
}

func WithArchitecture(arch string) PackageResourceModelDataOption {
	return func(model *PackageResourceModel) {
		model.Architecture = types.StringValue(arch)
	}
}

// newTestSigVerification builds a SignatureVerificationModel for use in tests.
func newTestSigVerification(enabled bool, publicKey string, keyless *KeylessVerificationModel) SignatureVerificationModel {
	pubKeyVal := types.StringNull()
	if publicKey != "" {
		pubKeyVal = types.StringValue(publicKey)
	}
	sig := SignatureVerificationModel{
		Verify:    types.BoolValue(enabled),
		PublicKey: pubKeyVal,
		Keyless:   types.ObjectNull(keylessVerificationAttrTypes),
	}
	if keyless != nil {
		obj, diags := types.ObjectValueFrom(context.Background(), keylessVerificationAttrTypes, *keyless)
		if diags.HasError() {
			panic("newTestSigVerification: failed to build keyless object")
		}
		sig.Keyless = obj
	}
	return sig
}

// withSigVerification sets the signature_verification block from a SignatureVerificationModel.
func withSigVerification(sig SignatureVerificationModel) PackageResourceModelDataOption {
	return func(model *PackageResourceModel) {
		obj, diags := types.ObjectValueFrom(context.Background(), signatureVerificationAttrTypes, sig)
		if diags.HasError() {
			panic("withSigVerification: failed to build object")
		}
		model.SignatureVerification = obj
	}
}

func WithPublicKey(publicKey string) PackageResourceModelDataOption {
	return withSigVerification(newTestSigVerification(true, publicKey, nil))
}

func WithSignatureVerificationEnabled(enabled bool) PackageResourceModelDataOption {
	return withSigVerification(newTestSigVerification(enabled, "", nil))
}

func WithTimeout(timeout string) PackageResourceModelDataOption {
	return func(model *PackageResourceModel) {
		model.Timeout = types.StringValue(timeout)
	}
}

func WithNamespace(namespace string) PackageResourceModelDataOption {
	return func(model *PackageResourceModel) {
		model.Namespace = types.StringValue(namespace)
	}
}

func WithComponents(components []ComponentModel) PackageResourceModelDataOption {
	return func(model *PackageResourceModel) {
		model.Components = componentSliceToSet(components)
	}
}

func WithVars(vars []VariableModel) PackageResourceModelDataOption {
	return func(model *PackageResourceModel) {
		model.Vars = variableSliceToSet(vars)
	}
}

func WithSensitiveVars(sensitiveVars []VariableModel) PackageResourceModelDataOption {
	return func(model *PackageResourceModel) {
		model.SensitiveVars = variableSliceToSet(sensitiveVars)
	}
}

// WithOptionalComponents sets optional_components to an explicit set of names.
// Pass an empty slice to set an empty (non-null) set.
func WithOptionalComponents(names []string) PackageResourceModelDataOption {
	return func(model *PackageResourceModel) {
		vals := make([]attr.Value, len(names))
		for i, name := range names {
			vals[i] = types.StringValue(name)
		}
		model.OptionalComponents = types.SetValueMust(types.StringType, vals)
	}
}

func WithValues(values types.Dynamic) PackageResourceModelDataOption {
	return func(model *PackageResourceModel) {
		model.Values = values
	}
}

func WithSensitiveValues(values types.Dynamic) PackageResourceModelDataOption {
	return func(model *PackageResourceModel) {
		model.SensitiveValues = values
	}
}

// NewTestPackageResourceModel creates a PackageResourceModel with default values and applies data options
func NewTestPackageResourceModel(options ...PackageResourceModelDataOption) PackageResourceModel {
	model := PackageResourceModel{
		Source:                types.StringValue("oci://ghcr.io/defenseunicorns/packages/test:latest"),
		Architecture:          types.StringValue(runtime.GOARCH),
		SignatureVerification: types.ObjectNull(signatureVerificationAttrTypes),
		Timeout:               types.StringValue("10m"),
		Namespace:             types.StringValue(""),
		Components:            componentSliceToSet([]ComponentModel{}),
		OptionalComponents:    types.SetNull(types.StringType),
		Vars:                  variableSliceToSet([]VariableModel{}),
		SensitiveVars:         variableSliceToSet([]VariableModel{}),
		Values:                types.DynamicNull(),
		SensitiveValues:       types.DynamicNull(),
	}

	for _, option := range options {
		option(&model)
	}

	return model
}

type ComponentModelDataOption func(*ComponentModel)

func WithComponentOverrides(overrides []ComponentChartValuesModel) ComponentModelDataOption {
	return func(model *ComponentModel) {
		model.Overrides = componentChartValuesSliceToSet(overrides)
	}
}

type ComponentChartValuesModelDataOption func(*ComponentChartValuesModel)

func WithComponentChartName(chartName string) ComponentChartValuesModelDataOption {
	return func(model *ComponentChartValuesModel) {
		model.ChartName = types.StringValue(chartName)
	}
}

func WithComponentChartValues(values []HelmChartPathValueModel) ComponentChartValuesModelDataOption {
	return func(model *ComponentChartValuesModel) {
		model.Values = helmChartPathValueSliceToSet(values)
	}
}

func WithComponentChartSensitiveValues(values []HelmChartPathValueModel) ComponentChartValuesModelDataOption {
	return func(model *ComponentChartValuesModel) {
		model.SensitiveValues = helmChartPathValueSliceToSet(values)
	}
}

// NewTestComponentModel creates a ComponentModel with default values and applies data options
func NewTestComponentModel(name string, options ...ComponentModelDataOption) ComponentModel {
	model := ComponentModel{
		Name: types.StringValue(name),
	}

	for _, option := range options {
		option(&model)
	}

	return model
}

// SetVarEntry is a small helper for defining expected runtime set variables
// in a compact, tabular form for tests.
type SetVarEntry struct {
	Name      string
	Value     string
	Sensitive bool
}

// buildVCFromEntries constructs a *variables.VariableConfig from a slice of SetVarEntry.
func buildVCFromEntries(entries []SetVarEntry) *variables.VariableConfig {
	vc := variables.New("", nil, nil)
	for _, e := range entries {
		vc.SetVariable(e.Name, e.Value, e.Sensitive, false, v1alpha1.RawVariableType)
	}
	return vc
}

// buildVCFromMaps constructs a *variables.VariableConfig from two input maps:
// one marked non-sensitive and one marked sensitive. Keys are provided as they
// come from deploy results (may be mixed case); production code will
// normalize names to lowercase when exporting.
func buildVCFromMaps(nonSensitive map[string]string, sensitive map[string]string) *variables.VariableConfig {
	vc := variables.New("", nil, nil)
	for k, v := range nonSensitive {
		vc.SetVariable(k, v, false, false, v1alpha1.RawVariableType)
	}
	for k, v := range sensitive {
		vc.SetVariable(k, v, true, false, v1alpha1.RawVariableType)
	}
	return vc
}

// DeployedVar is a compact representation of a deployed set variable with its
// value and whether it is sensitive.
type DeployedVar struct {
	Value     string
	Sensitive bool
}

// buildVCFromCondensedMap constructs a *variables.VariableConfig from a
// condensed map of deployed variables where the value includes whether it is
// sensitive. This mirrors the reviewer-suggested table format.
func buildVCFromCondensedMap(in map[string]DeployedVar) *variables.VariableConfig {
	vc := variables.New("", nil, nil)
	for k, v := range in {
		vc.SetVariable(k, v.Value, v.Sensitive, false, v1alpha1.RawVariableType)
	}
	return vc
}

// readStringMap extracts a map[string]string from a Terraform types.Map value.
// It returns an empty map (not nil) when the input is null/unknown/empty to make
// assertions simpler in tests.
func readStringMap(ctx context.Context, m types.Map) (map[string]string, error) {
	out := map[string]string{}
	if m.IsNull() || m.IsUnknown() {
		return out, nil
	}
	diags := m.ElementsAs(ctx, &out, false)
	if diags.HasError() {
		return nil, fmt.Errorf("failed to read types.Map: %v", diags)
	}
	return out, nil
}

// NewTestComponentChartValuesModel creates a ComponentChartValuesModel with default values and applies data options
func NewTestComponentChartValuesModel(chartName string, options ...ComponentChartValuesModelDataOption) ComponentChartValuesModel {
	model := ComponentChartValuesModel{
		ChartName: types.StringValue(chartName),
	}

	for _, option := range options {
		option(&model)
	}

	return model
}

// NewComponentModelsFromNames creates ComponentModel slice from component names
func NewComponentModelsFromNames(componentNames []string) []ComponentModel {
	componentModels := make([]ComponentModel, len(componentNames))
	for i, componentName := range componentNames {
		componentModels[i] = ComponentModel{
			Name: types.StringValue(componentName),
		}
	}
	return componentModels
}

func newErrorLoadPackageResult(err error) MockLoadPackageResult {
	return MockLoadPackageResult{
		Layout: &layout.PackageLayout{
			Pkg: v1alpha1.ZarfPackage{},
		},
		Error: err,
	}
}

// Helper function to create fresh MockLoadPackageResult for each test
func newValidLoadPackageResult() MockLoadPackageResult {
	return MockLoadPackageResult{
		Layout: &layout.PackageLayout{
			Pkg: v1alpha1.ZarfPackage{
				Metadata: v1alpha1.ZarfMetadata{
					Name:        "test-package",
					Description: "Test package",
					Version:     "0.0.1",
				},
				Components: []v1alpha1.ZarfComponent{
					{
						Name:     "test-required-component-0",
						Required: helpers.BoolPtr(true),
						Default:  false,
					},
					{
						Name:     "test-required-component-1",
						Required: helpers.BoolPtr(true),
						Default:  false,
					},
					{
						Name:     "test-optional-default-component-0",
						Required: nil, // Why zarf, why?
						Default:  true,
					},
					{
						Name:     "test-optional-default-component-1",
						Required: helpers.BoolPtr(false),
						Default:  true,
					},
					{
						Name:     "test-optional-non-default-component-0",
						Required: nil,
						Default:  false,
					},
					{
						Name:     "test-optional-non-default-component-1",
						Required: helpers.BoolPtr(false),
						Default:  false,
					},
				},
			},
		},
		Error: nil,
	}
}

func TestPackageResource_Upsert_VariableModels(t *testing.T) {
	packageLayout := layout.PackageLayout{
		Pkg: v1alpha1.ZarfPackage{
			Metadata: v1alpha1.ZarfMetadata{
				Name:        "test-package",
				Description: "Test package",
				Version:     "0.0.1",
			},
			Components: []v1alpha1.ZarfComponent{
				{
					Name:     "test-required-component-0",
					Required: helpers.BoolPtr(true),
					Default:  false,
				},
			},
		},
	}

	tests := []struct {
		name                   string
		vars                   []VariableModel
		sensitiveVars          []VariableModel
		VariableModelMap       types.Map
		expectedVariableModels map[string]string
	}{
		{
			name:          "vars and sensitiveVars",
			vars:          []VariableModel{{Name: types.StringValue("listKey"), Value: types.StringValue("listsValue")}},
			sensitiveVars: []VariableModel{{Name: types.StringValue("sensitive_listKey"), Value: types.StringValue("sensitive listValue")}},
			expectedVariableModels: map[string]string{
				"listKey":           "listsValue",
				"sensitive_listKey": "sensitive listValue",
			},
		},
		{
			name:          "vars only",
			vars:          []VariableModel{{Name: types.StringValue("listKey"), Value: types.StringValue("listsValue")}},
			sensitiveVars: []VariableModel{},
			expectedVariableModels: map[string]string{
				"listKey": "listsValue",
			},
		},
		{
			name:          "sensitiveVars only",
			vars:          []VariableModel{},
			sensitiveVars: []VariableModel{{Name: types.StringValue("sensitive_listKey"), Value: types.StringValue("sensitive listValue")}},
			expectedVariableModels: map[string]string{
				"sensitive_listKey": "sensitive listValue",
			},
		},
		{
			name:                   "no vars at all",
			vars:                   []VariableModel{},
			sensitiveVars:          []VariableModel{},
			expectedVariableModels: map[string]string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockPackager := &MockPackager{}
			mockPackageComponentFilter := &MockPackageComponentFilter{}
			mockPackager.On("LoadPackage", mock.Anything, mock.Anything, mock.Anything).Return(
				&packageLayout,
				nil,
			)
			mockPackager.On("Deploy", mock.Anything, mock.Anything, mock.Anything).Return(packager.DeployResult{}, nil)
			mockPackageComponentFilter.On("ForDeploy", mock.Anything).Return(mock.Anything)

			packageResource := NewPackageResource(nil, mockPackager, mockPackageComponentFilter, nil).(*PackageResource)
			testModel := NewTestPackageResourceModel(
				WithVars(tc.vars),
				WithSensitiveVars(tc.sensitiveVars),
			)

			_, err := packageResource.upsert(context.Background(), testModel)
			assert.NoError(t, err)

			// Check that Deploy was called and the variables map was provided with the correct values
			mockPackageComponentFilter.AssertExpectations(t)
			for _, call := range mockPackager.Calls {
				if call.Method == "Deploy" {
					deployOptions := call.Arguments[2].(zarfPackager.DeployOptions)
					assert.NotNil(t, deployOptions.SetVariables)
					assert.Len(t, deployOptions.SetVariables, len(tc.expectedVariableModels))
					assert.Equal(t, deployOptions.SetVariables, tc.expectedVariableModels)
				}
			}
		})
	}
}

func TestPackageResource_Upsert_ForceHelmSSAConflicts(t *testing.T) {
	packageLayout := newValidLoadPackageResult().Layout

	tests := []struct {
		name                  string
		forceHelmSSAConflicts bool
	}{
		{
			name:                  "ForceConflicts is false when provider setting is false",
			forceHelmSSAConflicts: false,
		},
		{
			name:                  "ForceConflicts is true when provider setting is true",
			forceHelmSSAConflicts: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockPackager := &MockPackager{}
			mockPackageComponentFilter := &MockPackageComponentFilter{}
			mockPackager.On("LoadPackage", mock.Anything, mock.Anything, mock.Anything).Return(packageLayout, nil)
			mockPackager.On("Deploy", mock.Anything, mock.Anything, mock.Anything).Return(packager.DeployResult{}, nil)
			mockPackageComponentFilter.On("ForDeploy", mock.Anything).Return(mock.Anything)

			providerCfg := &udsProviderConfig{
				ForceHelmSSAConflicts: tc.forceHelmSSAConflicts,
			}
			packageResource := NewPackageResource(providerCfg, mockPackager, mockPackageComponentFilter, nil).(*PackageResource)

			_, err := packageResource.upsert(context.Background(), NewTestPackageResourceModel())
			assert.NoError(t, err)

			for _, call := range mockPackager.Calls {
				if call.Method == "Deploy" {
					deployOptions := call.Arguments[2].(zarfPackager.DeployOptions)
					assert.Equal(t, tc.forceHelmSSAConflicts, deployOptions.ForceConflicts)
				}
			}
		})
	}
}

func TestPackageResource_Upsert_SetVariables(t *testing.T) {
	cases := []struct {
		name                        string
		deployedPackageSetVariables map[string]DeployedVar
		expectedSetVariables        map[string]string
	}{
		{
			name:                        "no deployed package set variables returns empty set_variables map",
			deployedPackageSetVariables: map[string]DeployedVar{},
			expectedSetVariables:        map[string]string{},
		},
		{
			name: "single non-sensitive set variable is exported into set_variables",
			deployedPackageSetVariables: map[string]DeployedVar{
				"OUTPUT": {Value: "output-val", Sensitive: false},
			},
			expectedSetVariables: map[string]string{"output": "output-val"},
		},
		{
			name: "single sensitive set variable is exported into set_variables",
			deployedPackageSetVariables: map[string]DeployedVar{
				"API_KEY": {Value: "s3cr3t", Sensitive: true},
			},
			expectedSetVariables: map[string]string{"api_key": "s3cr3t"},
		},
		{
			name: "variable names normalized to lowercase",
			deployedPackageSetVariables: map[string]DeployedVar{
				"OUTPUT": {Value: "output-val", Sensitive: false},
			},
			expectedSetVariables: map[string]string{"output": "output-val"},
		},
		{
			name: "multiple variables are all exported into set_variables",
			deployedPackageSetVariables: map[string]DeployedVar{
				"OUTPUT":      {Value: "output-val", Sensitive: false},
				"API_KEY":     {Value: "s3cr3t", Sensitive: true},
				"DB_NAME":     {Value: "mydb", Sensitive: false},
				"DB_PASSWORD": {Value: "p@ssw0rd", Sensitive: true},
			},
			expectedSetVariables: map[string]string{
				"output":      "output-val",
				"api_key":     "s3cr3t",
				"db_name":     "mydb",
				"db_password": "p@ssw0rd",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			packageLayout := layout.PackageLayout{
				Pkg: v1alpha1.ZarfPackage{
					Metadata: v1alpha1.ZarfMetadata{
						Name:        "test-package",
						Description: "Test package",
						Version:     "0.0.1",
					},
					Components: []v1alpha1.ZarfComponent{
						{
							Name:     "test-required-component-0",
							Required: helpers.BoolPtr(true),
							Default:  false,
						},
					},
				},
			}

			mockPackager := &MockPackager{}
			mockPackageComponentFilter := &MockPackageComponentFilter{}

			mockPackager.On("LoadPackage", mock.Anything, mock.Anything, mock.Anything).Return(&packageLayout, nil)

			deployRes := packager.DeployResult{VariableConfig: buildVCFromCondensedMap(tc.deployedPackageSetVariables)}
			mockPackager.On("Deploy", mock.Anything, mock.Anything, mock.Anything).Return(deployRes, nil)
			mockPackageComponentFilter.On("ForDeploy", mock.Anything).Return(mock.Anything)

			packageResource := NewPackageResource(nil, mockPackager, mockPackageComponentFilter, nil).(*PackageResource)

			testModel := NewTestPackageResourceModel()

			plan, err := packageResource.upsert(context.Background(), testModel)
			assert.NoError(t, err)

			gotSetVars, err := readStringMap(context.Background(), plan.SetVariables)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedSetVariables, gotSetVars)

			mockPackager.AssertExpectations(t)
			mockPackageComponentFilter.AssertExpectations(t)
		})
	}
}

// Ensure provider can export values coming from deployResult.Values
// including structured values which should be YAML-encoded in the exported map.
func TestPackageResource_Upsert_DeployValues_NotPersisted(t *testing.T) {
	packageLayout := layout.PackageLayout{
		Pkg: v1alpha1.ZarfPackage{
			Metadata: v1alpha1.ZarfMetadata{
				Name:        "test-package",
				Description: "Test package",
				Version:     "0.0.1",
			},
			Components: []v1alpha1.ZarfComponent{{Name: "test-required-component-0", Required: helpers.BoolPtr(true)}},
		},
	}

	mockPackager := &MockPackager{}
	mockPackageComponentFilter := &MockPackageComponentFilter{}

	mockPackager.On("LoadPackage", mock.Anything, mock.Anything, mock.Anything).Return(&packageLayout, nil)

	// Build deploy result with Values containing a plain string and a structured value
	structured := map[string]any{"a": 1, "b": "x"}
	deployRes := packager.DeployResult{
		Values: map[string]any{
			"plain":   "strval",
			"complex": structured,
		},
	}

	mockPackager.On("Deploy", mock.Anything, mock.Anything, mock.Anything).Return(deployRes, nil)
	mockPackageComponentFilter.On("ForDeploy", mock.Anything).Return(mock.Anything)

	packageResource := NewPackageResource(nil, mockPackager, mockPackageComponentFilter, nil).(*PackageResource)

	testModel := NewTestPackageResourceModel()

	plan, err := packageResource.upsert(context.Background(), testModel)
	assert.NoError(t, err)

	// Ensure deployResult.Values are not included in set_variables (only SetVariables are persisted)
	setVars := map[string]string{}
	if !plan.SetVariables.IsNull() && !plan.SetVariables.IsUnknown() {
		diags := plan.SetVariables.ElementsAs(context.Background(), &setVars, false)
		assert.False(t, diags.HasError(), "failed to read set_variables: %v", diags)
	}

	// plain and complex should not be present in set_variables
	_, okPlain := setVars["plain"]
	_, okComplex := setVars["complex"]
	assert.False(t, okPlain)
	assert.False(t, okComplex)

	mockPackager.AssertExpectations(t)
	mockPackageComponentFilter.AssertExpectations(t)
}

// Ensure input-provided vars / sensitive_vars are NOT persisted into computed maps
func TestPackageResource_Upsert_InputVars_NotPersisted(t *testing.T) {
	packageLayout := layout.PackageLayout{
		Pkg: v1alpha1.ZarfPackage{
			Metadata:   v1alpha1.ZarfMetadata{Name: "test-package"},
			Components: []v1alpha1.ZarfComponent{{Name: "test-required-component-0", Required: helpers.BoolPtr(true)}},
		},
	}

	mockPackager := &MockPackager{}
	mockPackageComponentFilter := &MockPackageComponentFilter{}

	mockPackager.On("LoadPackage", mock.Anything, mock.Anything, mock.Anything).Return(&packageLayout, nil)
	// Deploy returns empty result (no runtime set variables)
	mockPackager.On("Deploy", mock.Anything, mock.Anything, mock.Anything).Return(packager.DeployResult{}, nil)
	mockPackageComponentFilter.On("ForDeploy", mock.Anything).Return(mock.Anything)

	packageResource := NewPackageResource(nil, mockPackager, mockPackageComponentFilter, nil).(*PackageResource)

	// Provide vars and sensitive_vars in the model inputs
	testModel := NewTestPackageResourceModel(
		WithVars([]VariableModel{{Name: types.StringValue("INP1"), Value: types.StringValue("val1")}}),
		WithSensitiveVars([]VariableModel{{Name: types.StringValue("INP_SECRET"), Value: types.StringValue("val-secret")}}),
	)

	plan, err := packageResource.upsert(context.Background(), testModel)
	assert.NoError(t, err)

	setVars := map[string]string{}
	if !plan.SetVariables.IsNull() && !plan.SetVariables.IsUnknown() {
		diags := plan.SetVariables.ElementsAs(context.Background(), &setVars, false)
		assert.False(t, diags.HasError(), "failed to read set_variables: %v", diags)
	}
	// input var names should NOT be present in the computed maps
	_, ok1 := setVars["inp1"]
	assert.False(t, ok1)

	setVars = map[string]string{}
	if !plan.SetVariables.IsNull() && !plan.SetVariables.IsUnknown() {
		diags := plan.SetVariables.ElementsAs(context.Background(), &setVars, false)
		assert.False(t, diags.HasError(), "failed to read set_variables: %v", diags)
	}
	_, ok2 := setVars["inp_secret"]
	assert.False(t, ok2)

	mockPackager.AssertExpectations(t)
	mockPackageComponentFilter.AssertExpectations(t)
}

// When no runtime SetVariables are produced, the provider should return an
// empty `set_variables` map (not null) so callers can safely index into it.
func TestPackageResource_Upsert_SetVariables_EmptyAndNull(t *testing.T) {
	packageLayout := layout.PackageLayout{
		Pkg: v1alpha1.ZarfPackage{
			Metadata:   v1alpha1.ZarfMetadata{Name: "test-package"},
			Components: []v1alpha1.ZarfComponent{{Name: "test-required-component-0", Required: helpers.BoolPtr(true)}},
		},
	}

	mockPackager := &MockPackager{}
	mockPackageComponentFilter := &MockPackageComponentFilter{}

	mockPackager.On("LoadPackage", mock.Anything, mock.Anything, mock.Anything).Return(&packageLayout, nil)
	// Deploy returns an empty result (no VariableConfig)
	mockPackager.On("Deploy", mock.Anything, mock.Anything, mock.Anything).Return(packager.DeployResult{}, nil)
	mockPackageComponentFilter.On("ForDeploy", mock.Anything).Return(mock.Anything)

	packageResource := NewPackageResource(nil, mockPackager, mockPackageComponentFilter, nil).(*PackageResource)

	// Case A: No set variables configured and no deploy results
	testModelNull := NewTestPackageResourceModel()
	planNull, err := packageResource.upsert(context.Background(), testModelNull)
	assert.NoError(t, err)
	exportedNull := map[string]string{}
	diags := planNull.SetVariables.ElementsAs(context.Background(), &exportedNull, false)
	assert.False(t, diags.HasError())
	assert.Len(t, exportedNull, 0)

	// Case B: no runtime set variables (legacy export flag removed)
	testModelEmpty := NewTestPackageResourceModel()
	planEmpty, err := packageResource.upsert(context.Background(), testModelEmpty)
	assert.NoError(t, err)
	exportedEmpty := map[string]string{}
	diags2 := planEmpty.SetVariables.ElementsAs(context.Background(), &exportedEmpty, false)
	assert.False(t, diags2.HasError())
	assert.Len(t, exportedEmpty, 0)

	mockPackager.AssertExpectations(t)
	mockPackageComponentFilter.AssertExpectations(t)
}

func TestPackageResource_Upsert_OptionalComponentInstallation(t *testing.T) {
	tests := []struct {
		name                                      string
		componentNames                            []string
		expectedCallToDeploy                      bool
		expectedOptionalComponentsForDeployFilter []string
		expectedErrorContains                     []string
	}{
		{
			name:                 "package without components deploys required components only",
			componentNames:       []string{},
			expectedCallToDeploy: true,
			expectedOptionalComponentsForDeployFilter: []string{},
			expectedErrorContains:                     []string{},
		},
		{
			name: "package with only required components deploys required components only",
			componentNames: []string{
				"test-required-component-0",
				"test-required-component-1",
			},
			expectedCallToDeploy:                      true,
			expectedOptionalComponentsForDeployFilter: []string{},
			expectedErrorContains:                     []string{},
		},
		{
			name: "package with only optional components deploys each optional component",
			componentNames: []string{
				"test-optional-default-component-0",
				"test-optional-non-default-component-0",
			},
			expectedCallToDeploy:                      true,
			expectedOptionalComponentsForDeployFilter: []string{"test-optional-default-component-0", "test-optional-non-default-component-0"},
			expectedErrorContains:                     []string{},
		},
		{
			name: "package with required and optional components deploys each optional component",
			componentNames: []string{
				"test-required-component-0",
				"test-required-component-1",
				"test-optional-default-component-0",
				"test-optional-non-default-component-0",
			},
			expectedCallToDeploy: true,
			expectedOptionalComponentsForDeployFilter: []string{
				"test-optional-default-component-0",
				"test-optional-non-default-component-0",
			},
			expectedErrorContains: []string{},
		},
		{
			name: "package with unknown components returns error for each unknown component",
			componentNames: []string{
				"test-unknown-component-0",
				"test-unknown-component-1",
			},
			expectedCallToDeploy:                      false,
			expectedOptionalComponentsForDeployFilter: []string{},
			expectedErrorContains: []string{
				"unknown package component test-unknown-component-0",
				"unknown package component test-unknown-component-1",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockPackager := &MockPackager{}
			mockPackageComponentFilter := &MockPackageComponentFilter{}

			validLoadPackageResult := newValidLoadPackageResult()
			mockPackager.On("LoadPackage", mock.Anything, mock.Anything, mock.Anything).Return(
				validLoadPackageResult.Layout,
				validLoadPackageResult.Error,
			)
			if tc.expectedCallToDeploy {
				mockPackager.On("Deploy", mock.Anything, mock.Anything, mock.Anything).Return(packager.DeployResult{}, nil)
				mockPackageComponentFilter.On("ForDeploy", mock.Anything).Return(mock.Anything)
			}

			packageResource := NewPackageResource(nil, mockPackager, mockPackageComponentFilter, nil).(*PackageResource)
			componentModels := NewComponentModelsFromNames(tc.componentNames)
			testModel := NewTestPackageResourceModel(
				WithComponents(componentModels),
			)
			expectErrors := len(tc.expectedErrorContains) > 0

			_, err := packageResource.upsert(context.Background(), testModel)

			if expectErrors {
				assert.NotNil(t, err, "Expected error, got none")
				for _, expectedErrorMsg := range tc.expectedErrorContains {
					assert.Contains(t, err.Error(), expectedErrorMsg, "Expected error to contain %q, but got: %v", expectedErrorMsg, err.Error())
				}
			} else {
				assert.Nil(t, err, "Expected no error, got %v", err)
			}
			mockPackager.AssertExpectations(t)
			mockPackageComponentFilter.AssertExpectations(t)

			// Check that ForDeploy was called with expected optional components
			var actualOptionalComponents []string
			for _, call := range mockPackageComponentFilter.Calls {
				if call.Method == "ForDeploy" && len(call.Arguments) > 0 {
					actualOptionalComponents = call.Arguments[0].([]string)
				}
			}
			assert.ElementsMatch(t, tc.expectedOptionalComponentsForDeployFilter, actualOptionalComponents,
				fmt.Sprintf("Optional components selected for deploy do not match: expected: %v, actual: %v", tc.expectedOptionalComponentsForDeployFilter, actualOptionalComponents))
		})
	}
}

func TestPackageResource_Upsert_ComponentOverrides(t *testing.T) {
	tests := []struct {
		name                    string
		components              []ComponentModel
		expectedValuesOverrides map[string]map[string]map[string]any
		expectedCallToDeploy    bool
		expectedErrorContains   string
	}{
		{
			name:                    "package without component overrides deploys with empty values overrides map",
			components:              []ComponentModel{},
			expectedValuesOverrides: map[string]map[string]map[string]any{},
			expectedCallToDeploy:    true,
			expectedErrorContains:   "",
		},
		{
			name: "package with single required component overrides deploys with single component overrides map",
			components: []ComponentModel{
				NewTestComponentModel(
					"test-required-component-0",
					WithComponentOverrides([]ComponentChartValuesModel{
						NewTestComponentChartValuesModel(
							"chart1",
							WithComponentChartValues([]HelmChartPathValueModel{
								{Path: types.StringValue("replicaCount"), Value: types.StringValue("3")},
								{Path: types.StringValue("ui.color"), Value: types.StringValue("blue")},
							}),
							WithComponentChartSensitiveValues([]HelmChartPathValueModel{
								{Path: types.StringValue("apiKey"), Value: types.StringValue("secret123")},
							}),
						),
					}),
				),
			},
			expectedValuesOverrides: map[string]map[string]map[string]any{
				"test-required-component-0": {
					"chart1": {
						"replicaCount": 3,
						"ui": map[string]any{
							"color": "blue",
						},
						"apiKey": "secret123",
					},
				},
			},
			expectedCallToDeploy:  true,
			expectedErrorContains: "",
		},
		{
			name: "package with single optional component overrides deploys with single component overrides map",
			components: []ComponentModel{
				NewTestComponentModel(
					"test-optional-default-component-0",
					WithComponentOverrides([]ComponentChartValuesModel{
						NewTestComponentChartValuesModel(
							"chart1",
							WithComponentChartValues([]HelmChartPathValueModel{
								{Path: types.StringValue("replicaCount"), Value: types.StringValue("3")},
								{Path: types.StringValue("ui.color"), Value: types.StringValue("blue")},
							}),
							WithComponentChartSensitiveValues([]HelmChartPathValueModel{
								{Path: types.StringValue("apiKey"), Value: types.StringValue("secret123")},
							}),
						),
					}),
				),
			},
			expectedValuesOverrides: map[string]map[string]map[string]any{
				"test-optional-default-component-0": {
					"chart1": {
						"replicaCount": 3,
						"ui": map[string]any{
							"color": "blue",
						},
						"apiKey": "secret123",
					},
				},
			},
			expectedCallToDeploy:  true,
			expectedErrorContains: "",
		},
		{
			name: "package with multiple components and charts passes overrides deploys with all components overrides map",
			components: []ComponentModel{
				NewTestComponentModel(
					"test-required-component-0",
					WithComponentOverrides([]ComponentChartValuesModel{
						NewTestComponentChartValuesModel(
							"chart1",
							WithComponentChartValues([]HelmChartPathValueModel{
								{Path: types.StringValue("replicaCount"), Value: types.StringValue("3")},
								{Path: types.StringValue("ui.color"), Value: types.StringValue("blue")},
							}),
							WithComponentChartSensitiveValues([]HelmChartPathValueModel{
								{Path: types.StringValue("apiKey"), Value: types.StringValue("secret123")},
							}),
						),
					}),
				),
				NewTestComponentModel(
					"test-optional-default-component-0",
					WithComponentOverrides([]ComponentChartValuesModel{
						NewTestComponentChartValuesModel(
							"chart1",
							WithComponentChartValues([]HelmChartPathValueModel{
								{Path: types.StringValue("replicaCount"), Value: types.StringValue("2")},
							}),
						),
						NewTestComponentChartValuesModel(
							"chart2",
							WithComponentChartValues([]HelmChartPathValueModel{
								{Path: types.StringValue("service.port"), Value: types.StringValue("\"8080\"")},
							}),
						),
					}),
				),
				NewTestComponentModel(
					"test-optional-non-default-component-0",
					WithComponentOverrides([]ComponentChartValuesModel{
						NewTestComponentChartValuesModel(
							"chart3",
							WithComponentChartValues([]HelmChartPathValueModel{
								{Path: types.StringValue("image.tag"), Value: types.StringValue("v2.0.0")},
							}),
						),
					}),
				),
			},
			expectedValuesOverrides: map[string]map[string]map[string]any{
				"test-required-component-0": {
					"chart1": {
						"replicaCount": 3,
						"ui": map[string]any{
							"color": "blue",
						},
						"apiKey": "secret123",
					},
				},
				"test-optional-default-component-0": {
					"chart1": {
						"replicaCount": 2,
					},
					"chart2": {
						"service": map[string]any{
							"port": "8080",
						},
					},
				},
				"test-optional-non-default-component-0": {
					"chart3": {
						"image": map[string]any{
							"tag": "v2.0.0",
						},
					},
				},
			},
			expectedCallToDeploy:  true,
			expectedErrorContains: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockPackager := &MockPackager{}
			mockPackageComponentFilter := &MockPackageComponentFilter{}

			validLoadPackageResult := newValidLoadPackageResult()
			mockPackager.On("LoadPackage", mock.Anything, mock.Anything, mock.Anything).Return(
				validLoadPackageResult.Layout,
				validLoadPackageResult.Error,
			)

			if tc.expectedCallToDeploy {
				mockPackager.On("Deploy", mock.Anything, mock.Anything, mock.Anything).Return(packager.DeployResult{}, nil)
				mockPackageComponentFilter.On("ForDeploy", mock.Anything).Return(mock.Anything)
			}

			packageResource := NewPackageResource(nil, mockPackager, mockPackageComponentFilter, nil).(*PackageResource)
			testModel := NewTestPackageResourceModel(
				WithComponents(tc.components),
			)

			_, err := packageResource.upsert(context.Background(), testModel)

			if tc.expectedErrorContains != "" {
				assert.NotNil(t, err, "Expected error, got none")
				assert.Contains(t, err.Error(), tc.expectedErrorContains, "Expected error to contain %q, but got: %v", tc.expectedErrorContains, err.Error())
			} else {
				assert.Nil(t, err, "Expected no error, got %v", err)
			}

			mockPackager.AssertExpectations(t)
			mockPackageComponentFilter.AssertExpectations(t)

			// Verify that Deploy was called with the expected ValuesOverridesMap
			if tc.expectedCallToDeploy {
				var actualValuesOverrides map[string]map[string]map[string]any
				for _, call := range mockPackager.Calls {
					if call.Method == "Deploy" && len(call.Arguments) >= 3 {
						deployOpts := call.Arguments[2].(zarfPackager.DeployOptions)
						actualValuesOverrides = deployOpts.ValuesOverridesMap
					}
				}
				assert.Equal(t, tc.expectedValuesOverrides, actualValuesOverrides,
					"ValuesOverridesMap passed to Deploy does not match expected")
			}
		})
	}
}

func TestPackageResource_Upsert_Values(t *testing.T) {
	tests := []struct {
		name             string
		values           types.Dynamic
		sensitiveValues  types.Dynamic
		expectedValues   zarfValue.Values
		expectedErrorMsg string
	}{
		{
			name:           "package without values deploys with empty values",
			expectedValues: zarfValue.Values{},
		},
		{
			name: "package with values passes deploy values",
			values: types.DynamicValue(types.ObjectValueMust(
				map[string]attr.Type{
					"pod": types.ObjectType{AttrTypes: map[string]attr.Type{
						"replicaCount": types.NumberType,
					}},
					"logLevel": types.StringType,
				},
				map[string]attr.Value{
					"pod": types.ObjectValueMust(
						map[string]attr.Type{"replicaCount": types.NumberType},
						map[string]attr.Value{"replicaCount": types.NumberValue(big.NewFloat(3))},
					),
					"logLevel": types.StringValue("info"),
				},
			)),
			expectedValues: zarfValue.Values{
				"pod":      map[string]any{"replicaCount": int64(3)},
				"logLevel": "info",
			},
		},
		{
			name: "package with sensitive values deep merges deploy values",
			values: types.DynamicValue(types.ObjectValueMust(
				map[string]attr.Type{
					"db": types.ObjectType{AttrTypes: map[string]attr.Type{
						"hostname": types.StringType,
					}},
				},
				map[string]attr.Value{
					"db": types.ObjectValueMust(
						map[string]attr.Type{"hostname": types.StringType},
						map[string]attr.Value{"hostname": types.StringValue("postgres")},
					),
				},
			)),
			sensitiveValues: types.DynamicValue(types.ObjectValueMust(
				map[string]attr.Type{
					"db": types.ObjectType{AttrTypes: map[string]attr.Type{
						"password": types.StringType,
					}},
				},
				map[string]attr.Value{
					"db": types.ObjectValueMust(
						map[string]attr.Type{"password": types.StringType},
						map[string]attr.Value{"password": types.StringValue("secret")},
					),
				},
			)),
			expectedValues: zarfValue.Values{
				"db": map[string]any{
					"hostname": "postgres",
					"password": "secret",
				},
			},
		},
		{
			name: "package with conflicting sensitive values returns error",
			values: types.DynamicValue(types.ObjectValueMust(
				map[string]attr.Type{
					"db": types.ObjectType{AttrTypes: map[string]attr.Type{
						"password": types.StringType,
					}},
				},
				map[string]attr.Value{
					"db": types.ObjectValueMust(
						map[string]attr.Type{"password": types.StringType},
						map[string]attr.Value{"password": types.StringValue("plain")},
					),
				},
			)),
			sensitiveValues: types.DynamicValue(types.ObjectValueMust(
				map[string]attr.Type{
					"db": types.ObjectType{AttrTypes: map[string]attr.Type{
						"password": types.StringType,
					}},
				},
				map[string]attr.Value{
					"db": types.ObjectValueMust(
						map[string]attr.Type{"password": types.StringType},
						map[string]attr.Value{"password": types.StringValue("secret")},
					),
				},
			)),
			expectedErrorMsg: "db.password",
		},
		{
			name: "package with unexposed value path returns error",
			values: types.DynamicValue(types.ObjectValueMust(
				map[string]attr.Type{
					"image": types.ObjectType{AttrTypes: map[string]attr.Type{
						"tag": types.StringType,
					}},
				},
				map[string]attr.Value{
					"image": types.ObjectValueMust(
						map[string]attr.Type{"tag": types.StringType},
						map[string]attr.Value{"tag": types.StringValue("latest")},
					),
				},
			)),
			expectedErrorMsg: "image.tag",
		},
		{
			name:             "package with root unknown values returns error",
			values:           types.DynamicUnknown(),
			expectedErrorMsg: "invalid package values for apply: values must be known",
		},
		{
			name:             "package with root unknown sensitive values returns error",
			sensitiveValues:  types.DynamicUnknown(),
			expectedErrorMsg: "invalid sensitive package values for apply: sensitive_values must be known",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockPackager := &MockPackager{}
			mockPackageComponentFilter := &MockPackageComponentFilter{}

			validLoadPackageResult := newValidLoadPackageResult()
			validLoadPackageResult.Layout.Pkg.Components[0].Charts = []v1alpha1.ZarfChart{
				{
					Name: "test-chart",
					Values: []v1alpha1.ZarfChartValue{
						{SourcePath: ".pod.replicaCount", TargetPath: ".pod.replicaCount"},
						{SourcePath: ".logLevel", TargetPath: ".logLevel"},
						{SourcePath: ".db", TargetPath: ".db"},
					},
				},
			}
			mockPackager.On("LoadPackage", mock.Anything, mock.Anything, mock.Anything).Return(
				validLoadPackageResult.Layout,
				validLoadPackageResult.Error,
			)

			if tc.expectedErrorMsg == "" {
				mockPackager.On("Deploy", mock.Anything, mock.Anything, mock.Anything).Return(packager.DeployResult{}, nil)
			}
			mockPackageComponentFilter.On("ForDeploy", mock.Anything).Return(mock.Anything)

			packageResource := NewPackageResource(nil, mockPackager, mockPackageComponentFilter, nil).(*PackageResource)
			testModel := NewTestPackageResourceModel(
				WithValues(tc.values),
				WithSensitiveValues(tc.sensitiveValues),
			)

			_, err := packageResource.upsert(context.Background(), testModel)

			if tc.expectedErrorMsg != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrorMsg)
				return
			}

			assert.NoError(t, err)
			mockPackager.AssertExpectations(t)
			mockPackageComponentFilter.AssertExpectations(t)

			var actualValues zarfValue.Values
			for _, call := range mockPackager.Calls {
				if call.Method == "Deploy" && len(call.Arguments) >= 3 {
					deployOpts := call.Arguments[2].(zarfPackager.DeployOptions)
					actualValues = deployOpts.Values
				}
			}
			assert.Equal(t, tc.expectedValues, actualValues)
		})
	}
}

func TestPackageResource_Upsert_SourceAttribute(t *testing.T) {
	tests := []struct {
		name                      string
		source                    string
		localFilePathExists       bool
		expectedCallToLoadPackage bool
		expectedCallToDeploy      bool
		expectedErrorContains     string
	}{
		{
			name:                      "OCI distribution source with oci:// scheme loads specified source",
			source:                    "oci://ghcr.io/defenseunicornstest/packages/test-package:v1.0.0",
			localFilePathExists:       true,
			expectedCallToLoadPackage: true,
			expectedCallToDeploy:      true,
			expectedErrorContains:     "",
		},
		{
			name:                      "existent local absolute file path loads specified source",
			source:                    "/tmp/absolute/path/to/existenttest-package.tar.zst",
			localFilePathExists:       true,
			expectedCallToLoadPackage: true,
			expectedCallToDeploy:      true,
			expectedErrorContains:     "",
		},
		{
			name:                      "existent local relative file path loads specified source",
			source:                    "./relative/path/to/existenttest-package.tar.zst",
			localFilePathExists:       true,
			expectedCallToLoadPackage: true,
			expectedCallToDeploy:      true,
			expectedErrorContains:     "",
		},
		{
			name:                      "existent file without path loads specified source",
			source:                    "existent-test-package-without-path.tar.zst",
			localFilePathExists:       true,
			expectedCallToLoadPackage: true,
			expectedCallToDeploy:      true,
			expectedErrorContains:     "",
		},
		{
			name:                      "existent uncompressed tarfile loads specified source",
			source:                    "existent-test-package-without-path.tar",
			localFilePathExists:       true,
			expectedCallToLoadPackage: true,
			expectedCallToDeploy:      true,
			expectedErrorContains:     "",
		},
		{
			name:                      "nonexistent local absolute file path returns error",
			source:                    "/tmp/absolute/path/to/nonexistenttest-package.tar.zst",
			localFilePathExists:       false,
			expectedCallToLoadPackage: false,
			expectedCallToDeploy:      false,
			expectedErrorContains:     "no such file or directory",
		},
		{
			name:                      "nonexistent local relative file path returns error",
			source:                    "./relative/path/to/nonexistenttest-package.tar.zst",
			localFilePathExists:       false,
			expectedCallToLoadPackage: false,
			expectedCallToDeploy:      false,
			expectedErrorContains:     "no such file or directory",
		},
		{
			name:                      "nonexistent file without path returns error",
			source:                    "nonexistent-test-package-without-path.tar.zst",
			localFilePathExists:       false,
			expectedCallToLoadPackage: false,
			expectedCallToDeploy:      false,
			expectedErrorContains:     "no such file or directory",
		},
		{
			name:                      "nonexistent uncompressed tarfile loads specified source",
			source:                    "nonexistent-test-package-without-path.tar",
			localFilePathExists:       false,
			expectedCallToLoadPackage: false,
			expectedCallToDeploy:      false,
			expectedErrorContains:     "no such file or directory",
		},
		{
			name:                      "missing oci:// scheme for OCI reference returns error",
			source:                    "ghcr.io/defenseunicornstest/packages/test-package:v1.0.0",
			localFilePathExists:       false,
			expectedCallToLoadPackage: false,
			expectedCallToDeploy:      false,
			expectedErrorContains:     "invalid package source",
		},
		{
			name:                      "http URL returns error",
			source:                    "http://defenseunicornstest.com/test-package.tar.zst",
			localFilePathExists:       false,
			expectedCallToLoadPackage: false,
			expectedCallToDeploy:      false,
			expectedErrorContains:     "invalid package source",
		},
		{
			name:                      "https URL returns error",
			source:                    "https://defenseunicornstest.com/test-package.tar.zst",
			localFilePathExists:       false,
			expectedCallToLoadPackage: false,
			expectedCallToDeploy:      false,
			expectedErrorContains:     "invalid package source",
		},
		{
			name:                      "empty source returns error",
			source:                    "",
			localFilePathExists:       false,
			expectedCallToLoadPackage: false,
			expectedCallToDeploy:      false,
			expectedErrorContains:     "invalid package source",
		},
		{
			name:                      "whitespace source returns error",
			source:                    "     ",
			localFilePathExists:       false,
			expectedCallToLoadPackage: false,
			expectedCallToDeploy:      false,
			expectedErrorContains:     "invalid package source",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockPackager := &MockPackager{}
			mockPackageComponentFilter := &MockPackageComponentFilter{}

			// Create temporary file if test expects local file to exist
			if tc.localFilePathExists && !strings.HasPrefix(tc.source, helpers.OCIURLPrefix) {
				dir := filepath.Dir(tc.source)
				if dir != "." {
					err := os.MkdirAll(dir, 0o755)
					if err != nil {
						t.Fatalf("Failed to create test directory %s: %v", dir, err)
					}
					defer os.RemoveAll(strings.Split(tc.source, "/")[0])
				}

				file, err := os.Create(tc.source)
				if err != nil {
					t.Fatalf("Failed to create test file %s: %v", tc.source, err)
				}
				file.Close()
				defer os.Remove(tc.source)
			}

			if tc.expectedCallToLoadPackage {
				mockPackager.On("LoadPackage", mock.Anything, mock.Anything, mock.Anything).Return(
					newValidLoadPackageResult().Layout,
					newValidLoadPackageResult().Error,
				)
			}

			if tc.expectedCallToDeploy {
				mockPackager.On("Deploy", mock.Anything, mock.Anything, mock.Anything).Return(packager.DeployResult{}, nil)
				mockPackageComponentFilter.On("ForDeploy", mock.Anything).Return(mock.Anything)
			}

			packageResource := NewPackageResource(nil, mockPackager, mockPackageComponentFilter, nil).(*PackageResource)
			testModel := NewTestPackageResourceModel(WithSource(tc.source))
			expectErrors := len(tc.expectedErrorContains) > 0

			_, err := packageResource.upsert(context.Background(), testModel)

			if expectErrors {
				assert.NotNil(t, err, "Expected error, got none")
				assert.Contains(t, err.Error(), tc.expectedErrorContains)
			} else {
				assert.Nil(t, err, "Expected no error, got %v", err)
			}
			mockPackager.AssertExpectations(t)
			mockPackageComponentFilter.AssertExpectations(t)

			// Verify that LoadPackage was called with the correct source
			if tc.expectedCallToLoadPackage {
				mockPackager.AssertCalled(t, "LoadPackage", mock.Anything, tc.source, mock.Anything)
			}
		})
	}
}

func TestPackageResource_validateUniqueVarNames(t *testing.T) {
	tests := []struct {
		name               string
		model              PackageResourceModel
		expectedErrorCount int
	}{
		{
			name:               "no vars at all",
			expectedErrorCount: 0,
			model: PackageResourceModel{
				Vars:          variableSliceToSet([]VariableModel{}),
				SensitiveVars: variableSliceToSet([]VariableModel{}),
			},
		},
		{
			name:               "only regular vars, no duplicates",
			expectedErrorCount: 0,
			model: PackageResourceModel{
				Vars: variableSliceToSet([]VariableModel{
					{
						Name:  types.StringValue("variable_1"),
						Value: types.StringValue("value 1"),
					},
				}),
				SensitiveVars: variableSliceToSet([]VariableModel{}),
			},
		},
		{
			name:               "only regular vars, with duplicates",
			expectedErrorCount: 1,
			model: PackageResourceModel{
				Vars: variableSliceToSet([]VariableModel{
					{
						Name:  types.StringValue("variable_1"),
						Value: types.StringValue("value 1"),
					},
					{
						Name:  types.StringValue("variable_1"),
						Value: types.StringValue("duplicate value"),
					},
				}),
				SensitiveVars: variableSliceToSet([]VariableModel{}),
			},
		},
		{
			name:               "only sensitive vars, no duplicates",
			expectedErrorCount: 0,
			model: PackageResourceModel{
				Vars: variableSliceToSet([]VariableModel{}),
				SensitiveVars: variableSliceToSet([]VariableModel{
					{
						Name:  types.StringValue("sensitive variable_1"),
						Value: types.StringValue("sensitive value"),
					},
					{
						Name:  types.StringValue("sensitive variable_2"),
						Value: types.StringValue("sensitive value"),
					},
				}),
			},
		},
		{
			name:               "only sensitive vars, with duplicates",
			expectedErrorCount: 1,
			model: PackageResourceModel{
				Vars: variableSliceToSet([]VariableModel{}),
				SensitiveVars: variableSliceToSet([]VariableModel{
					{
						Name:  types.StringValue("sensitive variable_1"),
						Value: types.StringValue("sensitive value"),
					},
					{
						Name:  types.StringValue("sensitive variable_1"),
						Value: types.StringValue("sensitive value"),
					},
				}),
			},
		},
		{
			name:               "both var types, no duplicates",
			expectedErrorCount: 0,
			model: PackageResourceModel{
				Vars: variableSliceToSet([]VariableModel{
					{
						Name:  types.StringValue("variable_1"),
						Value: types.StringValue("value 1"),
					},
					{
						Name:  types.StringValue("variable_2"),
						Value: types.StringValue("duplicate value"),
					},
				}),
				SensitiveVars: variableSliceToSet([]VariableModel{
					{
						Name:  types.StringValue("sensitive variable_1"),
						Value: types.StringValue("sensitive value"),
					},
					{
						Name:  types.StringValue("sensitive variable_2"),
						Value: types.StringValue("sensitive value"),
					},
				}),
			},
		},
		{
			name:               "both var types, with duplicates",
			expectedErrorCount: 2,
			model: PackageResourceModel{
				Vars: variableSliceToSet([]VariableModel{
					{
						Name:  types.StringValue("variable_1"),
						Value: types.StringValue("value 1"),
					},
					{
						Name:  types.StringValue("variable_2"),
						Value: types.StringValue("duplicate value"),
					},
				}),
				SensitiveVars: variableSliceToSet([]VariableModel{
					{
						Name:  types.StringValue("variable_1"),
						Value: types.StringValue("sensitive value"),
					},
					{
						Name:  types.StringValue("variable_2"),
						Value: types.StringValue("sensitive value"),
					},
				}),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := resource.ValidateConfigResponse{}
			validateUniqueVarNames(tc.model, &resp)
			assert.Equal(t, tc.expectedErrorCount, resp.Diagnostics.ErrorsCount())
		})
	}
}

func TestPackageResource_Upsert_NamespaceOverride(t *testing.T) {
	packageLayout := layout.PackageLayout{
		Pkg: v1alpha1.ZarfPackage{
			Metadata: v1alpha1.ZarfMetadata{
				Name:        "test-package",
				Description: "Test package",
				Version:     "0.0.1",
			},
			Components: []v1alpha1.ZarfComponent{
				{
					Name:     "test-required-component-0",
					Required: helpers.BoolPtr(true),
					Default:  false,
				},
			},
		},
	}

	tests := []struct {
		name                      string
		namespace                 string
		expectedNamespaceOverride string
	}{
		{
			name:                      "no namespace",
			namespace:                 "",
			expectedNamespaceOverride: "",
		},
		{
			name:                      "namespace provided",
			namespace:                 "aBrandNewNamespace",
			expectedNamespaceOverride: "aBrandNewNamespace",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockPackager := &MockPackager{}
			mockPackageComponentFilter := &MockPackageComponentFilter{}
			mockPackager.On("LoadPackage", mock.Anything, mock.Anything, mock.Anything).Return(
				&packageLayout,
				nil,
			)
			mockPackager.On("Deploy", mock.Anything, mock.Anything, mock.Anything).Return(packager.DeployResult{}, nil)
			mockPackageComponentFilter.On("ForDeploy", mock.Anything).Return(mock.Anything)

			packageResource := NewPackageResource(nil, mockPackager, mockPackageComponentFilter, nil).(*PackageResource)
			testModel := NewTestPackageResourceModel(WithNamespace(tc.namespace))

			_, err := packageResource.upsert(context.Background(), testModel)
			assert.NoError(t, err)

			// Check that Deploy was called and the variables map was provided with the correct values
			mockPackageComponentFilter.AssertExpectations(t)
			for _, call := range mockPackager.Calls {
				if call.Method == "Deploy" {
					deployOptions := call.Arguments[2].(zarfPackager.DeployOptions)
					assert.Equal(t, deployOptions.NamespaceOverride, tc.expectedNamespaceOverride)
				}
			}
		})
	}
}

func TestPackageResource_RunPackagePlanChecks_SignatureVerification(t *testing.T) {
	tests := []struct {
		name              string
		modelOpts         []PackageResourceModelDataOption
		loadPackageError  error
		packageSigned     bool
		expectLoadErr     bool
		expectSigErr      bool
		expectLoadPackage bool
	}{
		{
			name:              "verify=true with load success passes",
			modelOpts:         []PackageResourceModelDataOption{WithPublicKey("some-key")},
			loadPackageError:  nil,
			expectLoadErr:     false,
			expectSigErr:      false,
			expectLoadPackage: true,
		},
		{
			name:              "signed package verification failure returns sigErr",
			modelOpts:         []PackageResourceModelDataOption{WithPublicKey("invalid-key")},
			packageSigned:     true,
			expectLoadErr:     false,
			expectSigErr:      true,
			expectLoadPackage: true,
		},
		{
			name:              "verify=true with load error returns loadErr not sigErr",
			modelOpts:         []PackageResourceModelDataOption{WithPublicKey("some-key")},
			loadPackageError:  fmt.Errorf("network failure loading package"),
			expectLoadErr:     true,
			expectSigErr:      false,
			expectLoadPackage: true,
		},
		{
			name:              "verify=false skips verification entirely",
			modelOpts:         []PackageResourceModelDataOption{WithSignatureVerificationEnabled(false)},
			loadPackageError:  fmt.Errorf("should not be called"),
			expectLoadErr:     false,
			expectSigErr:      false,
			expectLoadPackage: false,
		},
		{
			name: "unknown optional_components defers validation until apply",
			modelOpts: []PackageResourceModelDataOption{
				WithSignatureVerificationEnabled(false),
				func(m *PackageResourceModel) {
					m.OptionalComponents = types.SetUnknown(types.StringType)
				},
			},
			loadPackageError:  fmt.Errorf("should not be called"),
			expectLoadErr:     false,
			expectSigErr:      false,
			expectLoadPackage: false,
		},
		{
			name: "unknown source skips verification",
			modelOpts: []PackageResourceModelDataOption{func(m *PackageResourceModel) {
				m.Source = types.StringUnknown()
			}},
			loadPackageError:  fmt.Errorf("should not be called"),
			expectLoadErr:     false,
			expectSigErr:      false,
			expectLoadPackage: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockPackager := &MockPackager{}
			if tc.expectLoadPackage {
				if tc.loadPackageError == nil {
					result := newValidLoadPackageResult()
					if tc.packageSigned {
						result.Layout.Pkg.Build.Signed = helpers.BoolPtr(true)
					}
					mockPackager.On("LoadPackage", mock.Anything, mock.Anything, mock.Anything).Return(result.Layout, result.Error)
				} else {
					result := newErrorLoadPackageResult(tc.loadPackageError)
					mockPackager.On("LoadPackage", mock.Anything, mock.Anything, mock.Anything).Return(result.Layout, result.Error)
				}
			}

			packageResource := NewPackageResource(&udsProviderConfig{ValidatePackagesOnPlan: true}, mockPackager, nil, nil).(*PackageResource)
			model := NewTestPackageResourceModel(tc.modelOpts...)
			result := packageResource.runPackagePlanChecks(context.Background(), model, nil)

			if tc.expectLoadErr {
				assert.NotNil(t, result.LoadErr)
			} else {
				assert.Nil(t, result.LoadErr)
			}
			if tc.expectSigErr {
				assert.NotNil(t, result.SigErr)
			} else {
				assert.Nil(t, result.SigErr)
			}
			if tc.expectLoadPackage {
				mockPackager.AssertCalled(t, "LoadPackage", mock.Anything, mock.Anything, mock.Anything)
			} else {
				mockPackager.AssertNotCalled(t, "LoadPackage", mock.Anything, mock.Anything, mock.Anything)
			}
		})
	}
}

func TestPackageResource_RunPackagePlanChecks_ErrorRouting(t *testing.T) {
	tests := []struct {
		name             string
		modelOpts        []PackageResourceModelDataOption
		loadPackageError error
		expectLoadErr    bool
		expectSigErr     bool
		expectOptErr     bool
	}{
		{
			name: "load failure routes to loadErr not sigErr",
			modelOpts: []PackageResourceModelDataOption{
				WithSignatureVerificationEnabled(false),
				WithOptionalComponents([]string{"test-optional-non-default-component-0"}),
			},
			loadPackageError: fmt.Errorf("connection refused"),
			expectLoadErr:    true,
			expectSigErr:     false,
			expectOptErr:     false,
		},
		{
			name: "verify=false with invalid optional_components validates optionals without sig path",
			modelOpts: []PackageResourceModelDataOption{
				WithSignatureVerificationEnabled(false),
				WithOptionalComponents([]string{"nonexistent-component"}),
			},
			loadPackageError: nil,
			expectLoadErr:    false,
			expectSigErr:     false,
			expectOptErr:     true,
		},
		{
			name: "invalid optional_components routes to optErr not sigErr",
			modelOpts: []PackageResourceModelDataOption{
				WithPublicKey("some-key"),
				WithOptionalComponents([]string{"nonexistent-component"}),
			},
			loadPackageError: nil,
			expectLoadErr:    false,
			expectSigErr:     false,
			expectOptErr:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockPackager := &MockPackager{}
			if tc.loadPackageError == nil {
				result := newValidLoadPackageResult()
				mockPackager.On("LoadPackage", mock.Anything, mock.Anything, mock.Anything).Return(result.Layout, result.Error)
			} else {
				result := newErrorLoadPackageResult(tc.loadPackageError)
				mockPackager.On("LoadPackage", mock.Anything, mock.Anything, mock.Anything).Return(result.Layout, result.Error)
			}

			packageResource := NewPackageResource(&udsProviderConfig{ValidatePackagesOnPlan: true}, mockPackager, nil, nil).(*PackageResource)
			model := NewTestPackageResourceModel(tc.modelOpts...)
			result := packageResource.runPackagePlanChecks(context.Background(), model, nil)

			if tc.expectLoadErr {
				assert.NotNil(t, result.LoadErr, "expected LoadErr")
			} else {
				assert.Nil(t, result.LoadErr, "expected no LoadErr")
			}
			if tc.expectSigErr {
				assert.NotNil(t, result.SigErr, "expected SigErr")
			} else {
				assert.Nil(t, result.SigErr, "expected no SigErr")
			}
			if tc.expectOptErr {
				assert.NotNil(t, result.OptComponentsErr, "expected OptComponentsErr")
			} else {
				assert.Nil(t, result.OptComponentsErr, "expected no OptComponentsErr")
			}

			foundLoadPackageCall := false
			var loadOpts zarfPackager.LoadOptions
			for _, call := range mockPackager.Calls {
				if call.Method == "LoadPackage" {
					loadOpts = call.Arguments[2].(zarfPackager.LoadOptions)
					foundLoadPackageCall = true
					break
				}
			}
			require.True(t, foundLoadPackageCall, "LoadPackage was not called")
			assert.Equal(t, []zarfZoci.LayerType{zarfZoci.MetadataLayers}, loadOpts.LayerTypes,
				"inspection load must request metadata layers only")
		})
	}
}

func TestNormalizeOptionalComponentsPlan(t *testing.T) {
	tests := []struct {
		name     string
		config   PackageResourceModel
		plan     PackageResourceModel // framework pre-populates plan from config for non-null optional_components
		expected types.Set
	}{
		{
			name:     "null optional_components with no component blocks normalizes to empty set",
			config:   NewTestPackageResourceModel(),
			plan:     NewTestPackageResourceModel(),
			expected: types.SetValueMust(types.StringType, []attr.Value{}),
		},
		{
			name:     "null optional_components with component blocks stays null",
			config:   NewTestPackageResourceModel(WithComponents(NewComponentModelsFromNames([]string{"comp-a"}))),
			plan:     NewTestPackageResourceModel(WithComponents(NewComponentModelsFromNames([]string{"comp-a"}))),
			expected: types.SetNull(types.StringType),
		},
		{
			name: "null optional_components with unknown component blocks stays null",
			config: func() PackageResourceModel {
				model := NewTestPackageResourceModel()
				model.Components = types.SetUnknown(model.Components.ElementType(context.Background()))
				return model
			}(),
			plan:     NewTestPackageResourceModel(),
			expected: types.SetNull(types.StringType),
		},
		{
			name:     "explicit empty optional_components unchanged",
			config:   NewTestPackageResourceModel(WithOptionalComponents([]string{})),
			plan:     NewTestPackageResourceModel(WithOptionalComponents([]string{})),
			expected: types.SetValueMust(types.StringType, []attr.Value{}),
		},
		{
			name:     "explicit optional_components unchanged",
			config:   NewTestPackageResourceModel(WithOptionalComponents([]string{"comp-a", "comp-b"})),
			plan:     NewTestPackageResourceModel(WithOptionalComponents([]string{"comp-a", "comp-b"})),
			expected: types.SetValueMust(types.StringType, []attr.Value{types.StringValue("comp-a"), types.StringValue("comp-b")}),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := normalizeOptionalComponentsPlan(tc.config, tc.plan)
			assert.Equal(t, tc.expected, result.OptionalComponents)
		})
	}
}

func TestComponentBlocksMayBePresent(t *testing.T) {
	componentSet := componentSliceToSet(NewComponentModelsFromNames([]string{"comp-a"}))
	tests := []struct {
		name       string
		components types.Set
		expected   bool
	}{
		{
			name:       "null set",
			components: types.SetNull(componentSet.ElementType(context.Background())),
			expected:   false,
		},
		{
			name:       "empty set",
			components: componentSliceToSet([]ComponentModel{}),
			expected:   false,
		},
		{
			name:       "unknown set",
			components: types.SetUnknown(componentSet.ElementType(context.Background())),
			expected:   true,
		},
		{
			name:       "non-empty set",
			components: componentSet,
			expected:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, componentBlocksMayBePresent(tc.components))
		})
	}
}

func TestPackageResource_LoadPackageLayoutFromSource_LoadsPackageMetadataOnly(t *testing.T) {
	mockPackager := &MockPackager{}
	validLoadPackageResult := newValidLoadPackageResult()
	mockPackager.On("LoadPackage", mock.Anything, mock.Anything, mock.Anything).Return(
		validLoadPackageResult.Layout,
		validLoadPackageResult.Error,
	)
	packageResource := NewPackageResource(nil, mockPackager, nil, nil).(*PackageResource)
	model := NewTestPackageResourceModel(WithPublicKey("test-public-key"))

	pkgLayout, err := packageResource.loadPackageLayoutFromSource(context.Background(), model)

	assert.NoError(t, err)
	assert.NotNil(t, pkgLayout)
	mockPackager.AssertCalled(t, "LoadPackage", mock.Anything, mock.Anything, mock.Anything)
	loadOptions := mockPackager.Calls[0].Arguments[2].(zarfPackager.LoadOptions)
	assert.Nil(t, loadOptions.VerifyBlobOptions)
	assert.Equal(t, layout.VerifyNever, loadOptions.VerificationStrategy)
}

func TestPackageResource_VerifyPackageSignature_SkipsWhenVerificationDisabled(t *testing.T) {
	packageResource := NewPackageResource(nil, nil, nil, nil).(*PackageResource)
	packageResource.verifyPackageSignatureFunc = func(context.Context, *layout.PackageLayout, zarfSigning.VerifyBlobOptions) error {
		assert.Fail(t, "signature verification should not be called when signature_verification.verify is false")
		return nil
	}
	model := NewTestPackageResourceModel(WithSignatureVerificationEnabled(false))
	pkgLayout := &layout.PackageLayout{
		Pkg: v1alpha1.ZarfPackage{Build: v1alpha1.ZarfBuildData{Signed: helpers.BoolPtr(true)}},
	}

	err := packageResource.verifyPackageSignature(context.Background(), model, pkgLayout)

	assert.NoError(t, err)
}

func TestPackageResource_VerifyPackageSignature_CallsVerifierWithPublicKey(t *testing.T) {
	packageResource := NewPackageResource(nil, nil, nil, nil).(*PackageResource)
	var called bool
	var publicKeyContent string
	packageResource.verifyPackageSignatureFunc = func(_ context.Context, _ *layout.PackageLayout, opts zarfSigning.VerifyBlobOptions) error {
		called = true
		keyContent, err := os.ReadFile(opts.Key)
		assert.NoError(t, err)
		publicKeyContent = string(keyContent)
		return nil
	}
	model := NewTestPackageResourceModel(WithPublicKey("test-public-key"))
	pkgLayout := &layout.PackageLayout{
		Pkg: v1alpha1.ZarfPackage{Build: v1alpha1.ZarfBuildData{Signed: helpers.BoolPtr(true)}},
	}

	err := packageResource.verifyPackageSignature(context.Background(), model, pkgLayout)

	assert.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, "test-public-key", publicKeyContent)
}

func TestPackageResource_VerifyPackageSignature_CallsVerifierWithKeylessOptions(t *testing.T) {
	packageResource := NewPackageResource(nil, nil, nil, nil).(*PackageResource)
	var called bool
	var capturedOptions zarfSigning.VerifyBlobOptions
	packageResource.verifyPackageSignatureFunc = func(_ context.Context, _ *layout.PackageLayout, opts zarfSigning.VerifyBlobOptions) error {
		called = true
		capturedOptions = opts
		return nil
	}
	model := NewTestPackageResourceModel(WithKeylessVerification("test@example.com", "https://token.actions.githubusercontent.com"))
	pkgLayout := &layout.PackageLayout{
		Pkg: v1alpha1.ZarfPackage{Build: v1alpha1.ZarfBuildData{Signed: helpers.BoolPtr(true)}},
	}

	err := packageResource.verifyPackageSignature(context.Background(), model, pkgLayout)

	assert.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, "test@example.com", capturedOptions.CertVerify.CertIdentity)
	assert.Equal(t, "https://token.actions.githubusercontent.com", capturedOptions.CertVerify.CertOidcIssuer)
}

func TestPackageResource_Upsert_SkipsSignatureVerificationWhenDisabled(t *testing.T) {
	mockPackager := &MockPackager{}
	mockPackageComponentFilter := &MockPackageComponentFilter{}
	validLoadPackageResult := newValidLoadPackageResult()
	validLoadPackageResult.Layout.Pkg.Build.Signed = helpers.BoolPtr(true)
	mockPackager.On("LoadPackage", mock.Anything, mock.Anything, mock.Anything).Return(
		validLoadPackageResult.Layout,
		validLoadPackageResult.Error,
	)
	mockPackager.On("Deploy", mock.Anything, mock.Anything, mock.Anything).Return(packager.DeployResult{}, nil)
	mockPackageComponentFilter.On("ForDeploy", mock.Anything).Return(mock.Anything)
	packageResource := NewPackageResource(nil, mockPackager, mockPackageComponentFilter, nil).(*PackageResource)
	model := NewTestPackageResourceModel(WithSignatureVerificationEnabled(false))

	_, err := packageResource.upsert(context.Background(), model)

	assert.NoError(t, err)
	mockPackageComponentFilter.AssertExpectations(t)
	mockPackager.AssertCalled(t, "Deploy", mock.Anything, mock.Anything, mock.Anything)
}

func TestPackageResource_Upsert_DoesNotDeployWhenSignatureVerificationFails(t *testing.T) {
	mockPackager := &MockPackager{}
	validLoadPackageResult := newValidLoadPackageResult()
	validLoadPackageResult.Layout.Pkg.Build.Signed = helpers.BoolPtr(true)
	mockPackager.On("LoadPackage", mock.Anything, mock.Anything, mock.Anything).Return(
		validLoadPackageResult.Layout,
		validLoadPackageResult.Error,
	)
	packageResource := NewPackageResource(nil, mockPackager, nil, nil).(*PackageResource)
	packageResource.verifyPackageSignatureFunc = func(context.Context, *layout.PackageLayout, zarfSigning.VerifyBlobOptions) error {
		return errors.New("signature verification failed")
	}

	_, err := packageResource.upsert(context.Background(), NewTestPackageResourceModel())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "signature verification failed")
	mockPackager.AssertNotCalled(t, "Deploy", mock.Anything, mock.Anything, mock.Anything)
}

func TestPackageResource_Upsert_DeploysWhenSignatureVerificationSucceeds(t *testing.T) {
	mockPackager := &MockPackager{}
	mockPackageComponentFilter := &MockPackageComponentFilter{}
	validLoadPackageResult := newValidLoadPackageResult()
	validLoadPackageResult.Layout.Pkg.Build.Signed = helpers.BoolPtr(true)
	mockPackager.On("LoadPackage", mock.Anything, mock.Anything, mock.Anything).Return(
		validLoadPackageResult.Layout,
		validLoadPackageResult.Error,
	)
	mockPackager.On("Deploy", mock.Anything, mock.Anything, mock.Anything).Return(packager.DeployResult{}, nil)
	mockPackageComponentFilter.On("ForDeploy", mock.Anything).Return(mock.Anything)
	packageResource := NewPackageResource(nil, mockPackager, mockPackageComponentFilter, nil).(*PackageResource)
	var called bool
	packageResource.verifyPackageSignatureFunc = func(context.Context, *layout.PackageLayout, zarfSigning.VerifyBlobOptions) error {
		called = true
		return nil
	}

	_, err := packageResource.upsert(context.Background(), NewTestPackageResourceModel())

	assert.NoError(t, err)
	assert.True(t, called)
	mockPackageComponentFilter.AssertExpectations(t)
	mockPackager.AssertCalled(t, "Deploy", mock.Anything, mock.Anything, mock.Anything)
}

// Unit tests for upsert method architecture attribute
func TestPackageResource_Upsert_Architecture(t *testing.T) {
	tests := []struct {
		name                 string
		modelArchitecture    string // Architecture set in the model (empty string means not set)
		providerArchitecture string // Architecture from provider config
		expectedArchitecture string // Expected architecture passed to LoadPackage
	}{
		{
			name:                 "architecture not specified in package model uses provider default architecture set to amd64",
			modelArchitecture:    "",
			providerArchitecture: "amd64",
			expectedArchitecture: "amd64",
		},
		{
			name:                 "architecture not specified in package model uses provider default architecture set to arm64",
			modelArchitecture:    "",
			providerArchitecture: "arm64",
			expectedArchitecture: "arm64",
		},
		{
			name:                 "architecture specified in package model uses model value",
			modelArchitecture:    "arm64",
			providerArchitecture: "amd64",
			expectedArchitecture: "arm64",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockPackager := &MockPackager{}
			mockPackageComponentFilter := &MockPackageComponentFilter{}

			validLoadPackageResult := newValidLoadPackageResult()
			mockPackager.On("LoadPackage", mock.Anything, mock.Anything, mock.Anything).Return(
				validLoadPackageResult.Layout,
				validLoadPackageResult.Error,
			)
			mockPackager.On("Deploy", mock.Anything, mock.Anything, mock.Anything).Return(packager.DeployResult{}, nil)
			mockPackageComponentFilter.On("ForDeploy", mock.Anything).Return(mock.Anything)

			providerConfig := &udsProviderConfig{
				DefaultArchitecture: tc.providerArchitecture,
			}
			packageResource := NewPackageResource(providerConfig, mockPackager, mockPackageComponentFilter, nil).(*PackageResource)

			var testModel PackageResourceModel
			if tc.modelArchitecture != "" {
				testModel = NewTestPackageResourceModel(WithArchitecture(tc.modelArchitecture))
			} else {
				// Create model without architecture set (null value)
				testModel = NewTestPackageResourceModel()
				testModel.Architecture = types.StringNull()
			}

			_, err := packageResource.upsert(context.Background(), testModel)
			assert.NoError(t, err)

			// Find the LoadOptions from the LoadPackage call
			loadOptionsArgFound := false
			var loadOptions zarfPackager.LoadOptions
			for _, call := range mockPackager.Calls {
				if call.Method == "LoadPackage" {
					loadOptions = call.Arguments[2].(zarfPackager.LoadOptions)
					loadOptionsArgFound = true
					break
				}
			}

			assert.True(t, loadOptionsArgFound, "Could not find LoadOptions argument in LoadPackage call")
			assert.Equal(t, tc.expectedArchitecture, loadOptions.Architecture,
				"Expected architecture %s but got %s", tc.expectedArchitecture, loadOptions.Architecture)

			mockPackageComponentFilter.AssertExpectations(t)
		})
	}
}

func TestPackageResource_Upsert_ConnectStrings(t *testing.T) {
	packageLayout := layout.PackageLayout{
		Pkg: v1alpha1.ZarfPackage{
			Metadata: v1alpha1.ZarfMetadata{
				Name:        "test-package",
				Description: "Test package",
				Version:     "0.0.1",
			},
			Components: []v1alpha1.ZarfComponent{
				{
					Name:     "test-component",
					Required: helpers.BoolPtr(true),
					Default:  false,
				},
			},
		},
	}

	tests := []struct {
		name                       string
		deployResultConnectStrings map[string]zarfState.ConnectString
		expectedConnectStrings     []struct {
			name        string
			description string
		}
	}{
		{
			name:                       "no connect strings results in empty set",
			deployResultConnectStrings: map[string]zarfState.ConnectString{},
			expectedConnectStrings:     []struct{ name, description string }{},
		},
		{
			name: "one connect string results in single entity set",
			deployResultConnectStrings: map[string]zarfState.ConnectString{
				"test-app-1": {
					Description: "Test application 1",
				},
			},
			expectedConnectStrings: []struct{ name, description string }{
				{
					name:        "test-app-1",
					description: "Test application 1",
				},
			},
		},
		{
			name: "multiple connect strings result in multiple entities in set",
			deployResultConnectStrings: map[string]zarfState.ConnectString{
				"test-app-1": {
					Description: "Test application 1",
				},
				"test-app-2": {
					Description: "Test application 2",
				},
				"test-app-3": {
					Description: "Test application 3",
				},
			},
			expectedConnectStrings: []struct{ name, description string }{
				{
					name:        "test-app-1",
					description: "Test application 1",
				},
				{
					name:        "test-app-2",
					description: "Test application 2",
				},
				{
					name:        "test-app-3",
					description: "Test application 3",
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockPackager := &MockPackager{}
			mockPackageComponentFilter := &MockPackageComponentFilter{}

			mockPackager.On("LoadPackage", mock.Anything, mock.Anything, mock.Anything).Return(
				&packageLayout,
				nil,
			)

			deployResult := packager.DeployResult{
				DeployedComponents: []zarfState.DeployedComponent{
					{
						Name: "test-component",
						InstalledCharts: []zarfState.InstalledChart{
							{
								Namespace:      "default",
								ChartName:      "test-chart",
								ConnectStrings: tc.deployResultConnectStrings,
							},
						},
					},
				},
			}

			mockPackager.On("Deploy", mock.Anything, mock.Anything, mock.Anything).Return(deployResult, nil)
			mockPackageComponentFilter.On("ForDeploy", mock.Anything).Return(mock.Anything)

			packageResource := NewPackageResource(nil, mockPackager, mockPackageComponentFilter, nil).(*PackageResource)
			testModel := NewTestPackageResourceModel()

			result, err := packageResource.upsert(context.Background(), testModel)
			assert.NoError(t, err)
			assert.False(t, result.ConnectStrings.IsNull(), "ConnectStrings should not be null")

			// Verify connect strings were populated
			expectedConnectStringsCount := len(tc.expectedConnectStrings)
			if expectedConnectStringsCount == 0 {
				assert.True(t, result.ConnectStrings.IsUnknown() || len(result.ConnectStrings.Elements()) == 0, "ConnectStrings should be empty")
			} else {
				assert.Len(t, result.ConnectStrings.Elements(), expectedConnectStringsCount)

				connectStringsMap := make(map[string]string)
				for _, elem := range result.ConnectStrings.Elements() {
					obj := elem.(types.Object)
					attrs := obj.Attributes()
					name := attrs["name"].(types.String).ValueString()
					description := attrs["description"].(types.String).ValueString()
					connectStringsMap[name] = description
				}

				for _, expected := range tc.expectedConnectStrings {
					description, found := connectStringsMap[expected.name]
					assert.True(t, found, "Expected connect string %s not found", expected.name)
					assert.Equal(t, expected.description, description, "Connect string %s has incorrect description", expected.name)
				}
			}
		})
	}
}

func TestComputePackageID(t *testing.T) {
	tests := []struct {
		name       string
		namespace  string
		pkgName    string
		expectedID string
	}{
		{
			name:       "No namespace provided",
			namespace:  "",
			pkgName:    "my-package-name",
			expectedID: "my-package-name",
		},
		{
			name:       "namespace provided",
			namespace:  "my-namespace",
			pkgName:    "my-package-name",
			expectedID: "my-namespace:my-package-name",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expectedID, computePackageID(tc.namespace, tc.pkgName))
		})
	}
}

func TestGetOptionalComponentsToRemove(t *testing.T) {
	tests := []struct {
		name                  string
		newPlanComponentNames []string
		oldPlanComponentNames []string
		expectedComponents    []string
	}{
		{
			name: "identifical components returns empty list",
			newPlanComponentNames: []string{
				"component-1",
			},
			oldPlanComponentNames: []string{
				"component-1",
			},
			expectedComponents: []string{},
		},
		{
			name: "single component removed from new plan returns removed components",
			newPlanComponentNames: []string{
				"component-1",
			},
			oldPlanComponentNames: []string{
				"component-1",
				"component-2",
			},
			expectedComponents: []string{"component-2"},
		},
		{
			name: "multiple components removed from new plan returns removed components",
			newPlanComponentNames: []string{
				"component-1",
				"component-2",
			},
			oldPlanComponentNames: []string{
				"component-1",
				"component-2",
				"component-3",
				"component-4",
			},
			expectedComponents: []string{"component-3", "component-4"},
		},
		{
			name: "new plan with additional components without removing any returns an empty list",
			newPlanComponentNames: []string{
				"component-1",
				"component-2",
				"component-3",
				"component-4",
			},
			oldPlanComponentNames: []string{
				"component-1",
				"component-2",
			},
			expectedComponents: []string{},
		},
		{
			name: "new plan with additional components and removed components returns only removed components",
			newPlanComponentNames: []string{
				"component-1",
				"component-3",
				"component-4",
			},
			oldPlanComponentNames: []string{
				"component-1",
				"component-2",
			},
			expectedComponents: []string{"component-2"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			newPlan := NewTestPackageResourceModel(
				WithComponents(NewComponentModelsFromNames(tc.newPlanComponentNames)),
			)
			oldPlan := NewTestPackageResourceModel(
				WithComponents(NewComponentModelsFromNames(tc.oldPlanComponentNames)),
			)

			componentsToRemove := getMissingComponents(newPlan, oldPlan)

			assert.Equal(t, len(tc.expectedComponents), len(componentsToRemove))
			for i := range componentsToRemove {
				assert.Equal(t, tc.expectedComponents[i], componentsToRemove[i])
			}
		})
	}
}

func TestGetMissingOptionalComponents(t *testing.T) {
	tests := []struct {
		name     string
		oldPlan  PackageResourceModel
		newPlan  PackageResourceModel
		expected []string
	}{
		{
			name:     "returns old optional for removal when old has one optional and new is empty",
			oldPlan:  NewTestPackageResourceModel(WithOptionalComponents([]string{"metrics"})),
			newPlan:  NewTestPackageResourceModel(WithOptionalComponents([]string{})),
			expected: []string{"metrics"},
		},
		{
			name:     "returns removed optional when old has two optionals and new keeps one",
			oldPlan:  NewTestPackageResourceModel(WithOptionalComponents([]string{"metrics", "logging"})),
			newPlan:  NewTestPackageResourceModel(WithOptionalComponents([]string{"logging"})),
			expected: []string{"metrics"},
		},
		{
			name:     "returns nothing when old and new are identical",
			oldPlan:  NewTestPackageResourceModel(WithOptionalComponents([]string{"metrics"})),
			newPlan:  NewTestPackageResourceModel(WithOptionalComponents([]string{"metrics"})),
			expected: nil,
		},
		{
			name:     "returns nothing when old is null on the legacy path",
			oldPlan:  NewTestPackageResourceModel(),
			newPlan:  NewTestPackageResourceModel(WithOptionalComponents([]string{})),
			expected: nil,
		},
		{
			name:     "returns nothing when new is null on the legacy path",
			oldPlan:  NewTestPackageResourceModel(WithOptionalComponents([]string{"metrics"})),
			newPlan:  NewTestPackageResourceModel(),
			expected: nil,
		},
		{
			name: "returns nothing when old optional_components is unknown",
			oldPlan: func() PackageResourceModel {
				model := NewTestPackageResourceModel()
				model.OptionalComponents = types.SetUnknown(types.StringType)
				return model
			}(),
			newPlan:  NewTestPackageResourceModel(WithOptionalComponents([]string{})),
			expected: nil,
		},
		{
			name:    "returns nothing when new optional_components is unknown",
			oldPlan: NewTestPackageResourceModel(WithOptionalComponents([]string{"metrics"})),
			newPlan: func() PackageResourceModel {
				model := NewTestPackageResourceModel()
				model.OptionalComponents = types.SetUnknown(types.StringType)
				return model
			}(),
			expected: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := getMissingOptionalComponents(tc.newPlan, tc.oldPlan)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestUpdate_RemoveComponents(t *testing.T) {
	tests := []struct {
		name               string
		componentsToRemove []string
		removeCalled       bool
	}{
		{
			name:               "remove single component",
			componentsToRemove: []string{"this-is-my-component"},
			removeCalled:       true,
		},
		{
			name:               "remove no components",
			componentsToRemove: []string{},
			removeCalled:       false,
		},
		{
			name:               "remove multiple components",
			componentsToRemove: []string{"this-is-my-component", "this-is-also-a-component"},
			removeCalled:       true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockPackageComponentFilter := &MockPackageComponentFilter{}
			mockPackageComponentFilter.On("ForRemove", mock.Anything).Return(mock.Anything)

			mockCluster := MockCluster{}
			zarfCluster := zarfCluster.Cluster{}
			mockCluster.On("NewWithWait", mock.Anything).Return(&zarfCluster, nil)

			mockPackager := &MockPackager{}
			zarfPackage := v1alpha1.ZarfPackage{}
			mockPackager.On("GetPackageFromSourceOrCluster", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(zarfPackage, nil)
			mockPackager.On("Remove", mock.Anything, mock.Anything, mock.Anything).Return(nil)

			resp := resource.UpdateResponse{}
			packageResource := NewPackageResource(nil, mockPackager, mockPackageComponentFilter, &mockCluster).(*PackageResource)

			// This is the meat of the test!
			plan := NewTestPackageResourceModel()
			packageResource.removeComponents(context.TODO(), plan, tc.componentsToRemove, &resp)

			// Assertions
			assert.False(t, resp.Diagnostics.HasError(), resp.Diagnostics.Errors())

			// Check that Deploy was called and the variables map was provided with the correct values
			for _, call := range mockPackager.Calls {
				if call.Method == "Deploy" {
					deployOptions := call.Arguments[2].(zarfPackager.DeployOptions)
					assert.NotNil(t, deployOptions.SetVariables)
				}
			}

			if tc.removeCalled {
				// Validate that a remove filter was created with the correct inputs
				mockPackageComponentFilter.AssertExpectations(t)
				for _, call := range mockPackageComponentFilter.Calls {
					if call.Method == "ForRemove" {
						forRemoveOptions := call.Arguments[0].([]string)
						assert.Equal(t, len(tc.componentsToRemove), len(forRemoveOptions))
						for i := range forRemoveOptions {
							assert.Equal(t, tc.componentsToRemove[i], forRemoveOptions[i])
						}
					}
				}

				// Validate that the remove function was called
				mockPackager.AssertExpectations(t)
			} else {
				// Verify that functions were not called if we did not expect them to be
				mockPackageComponentFilter.AssertNotCalled(t, "ForRemove")
				mockCluster.AssertNotCalled(t, "NewWithWait")
				mockPackager.AssertNotCalled(t, "GetPackageFromSourceOrCluster")
				mockPackager.AssertNotCalled(t, "Remove")
			}
		})
	}
}

func TestValidateOptionalComponentsAgainstPackage(t *testing.T) {
	boolTrue := true
	pkgComponents := []v1alpha1.ZarfComponent{
		{Name: "required-comp", Required: &boolTrue},
		{Name: "optional-a"},
		{Name: "optional-b"},
	}

	tests := []struct {
		name          string
		requested     []string
		pkgComponents []v1alpha1.ZarfComponent
		expectError   bool
		errorContains []string
	}{
		{
			name:          "valid optional component passes",
			requested:     []string{"optional-a"},
			pkgComponents: pkgComponents,
			expectError:   false,
		},
		{
			name:          "unknown name errors and lists valid optionals",
			requested:     []string{"does-not-exist"},
			pkgComponents: pkgComponents,
			expectError:   true,
			errorContains: []string{`"does-not-exist"`, `"optional-a"`, `"optional-b"`},
		},
		{
			name:          "required component name errors and lists valid optionals",
			requested:     []string{"required-comp"},
			pkgComponents: pkgComponents,
			expectError:   true,
			errorContains: []string{`"required-comp"`, `"optional-a"`, `"optional-b"`},
		},
		{
			name:          "multiple invalid names all shown",
			requested:     []string{"bad-one", "bad-two", "optional-a"},
			pkgComponents: pkgComponents,
			expectError:   true,
			errorContains: []string{`"bad-one"`, `"bad-two"`},
		},
		{
			name:          "package with no optionals reports specific message",
			requested:     []string{"anything"},
			pkgComponents: []v1alpha1.ZarfComponent{{Name: "only-required", Required: &boolTrue}},
			expectError:   true,
			errorContains: []string{"no optional components"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateOptionalComponentsAgainstPackage(tc.requested, tc.pkgComponents)
			if tc.expectError {
				assert.NotNil(t, err)
				for _, s := range tc.errorContains {
					assert.Contains(t, err.Error(), s)
				}
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

func TestDeployAsNewOrUpdate_OptionalComponentRemoval(t *testing.T) {
	boolFalse := false
	zarfPkg := v1alpha1.ZarfPackage{
		Components: []v1alpha1.ZarfComponent{
			{Name: "metrics", Required: &boolFalse},
		},
	}

	mockCluster := MockCluster{}
	zarfClusterInst := zarfCluster.Cluster{}
	mockCluster.On("NewWithWait", mock.Anything).Return(&zarfClusterInst, nil)

	mockPackager := &MockPackager{}
	mockPackager.On("GetPackageFromSourceOrCluster", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(zarfPkg, nil)
	mockPackager.On("Remove", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	validResult := newValidLoadPackageResult()
	mockPackager.On("LoadPackage", mock.Anything, mock.Anything, mock.Anything).Return(validResult.Layout, nil)
	mockPackager.On("Deploy", mock.Anything, mock.Anything, mock.Anything).Return(packager.DeployResult{}, nil)

	mockPackageComponentFilter := &MockPackageComponentFilter{}
	mockPackageComponentFilter.On("ForRemove", mock.Anything).Return(mock.Anything)
	mockPackageComponentFilter.On("ForDeploy", mock.Anything).Return(mock.Anything)

	oldPlan := NewTestPackageResourceModel(WithOptionalComponents([]string{"metrics"}))
	newPlan := NewTestPackageResourceModel(WithOptionalComponents([]string{}))

	packageResource := NewPackageResource(nil, mockPackager, mockPackageComponentFilter, &mockCluster).(*PackageResource)
	resp := resource.UpdateResponse{}
	_, err := packageResource.deployAsNewOrUpdate(context.Background(), newPlan, oldPlan, &resp)

	assert.NoError(t, err)
	assert.False(t, resp.Diagnostics.HasError(), resp.Diagnostics.Errors())

	var forRemoveArgs []string
	for _, call := range mockPackageComponentFilter.Calls {
		if call.Method == "ForRemove" {
			forRemoveArgs = call.Arguments[0].([]string)
		}
	}
	assert.Equal(t, []string{"metrics"}, forRemoveArgs, "ForRemove should be called with the removed optional component")
	mockPackager.AssertCalled(t, "Remove", mock.Anything, mock.Anything, mock.Anything)
}

func TestDeployAsNewOrUpdate_RemovalFailureSkipsUpsert(t *testing.T) {
	boolFalse := false
	zarfPkg := v1alpha1.ZarfPackage{
		Components: []v1alpha1.ZarfComponent{
			{Name: "metrics", Required: &boolFalse},
		},
	}

	mockCluster := MockCluster{}
	zarfClusterInst := zarfCluster.Cluster{}
	mockCluster.On("NewWithWait", mock.Anything).Return(&zarfClusterInst, nil)

	mockPackager := &MockPackager{}
	mockPackager.On("GetPackageFromSourceOrCluster", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(zarfPkg, nil)
	mockPackager.On("Remove", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("remove failed"))

	mockPackageComponentFilter := &MockPackageComponentFilter{}
	mockPackageComponentFilter.On("ForRemove", mock.Anything).Return(mock.Anything)

	oldPlan := NewTestPackageResourceModel(WithOptionalComponents([]string{"metrics"}))
	newPlan := NewTestPackageResourceModel(WithOptionalComponents([]string{}))

	packageResource := NewPackageResource(nil, mockPackager, mockPackageComponentFilter, &mockCluster).(*PackageResource)
	resp := resource.UpdateResponse{}
	_, err := packageResource.deployAsNewOrUpdate(context.Background(), newPlan, oldPlan, &resp)

	assert.NoError(t, err)
	assert.True(t, resp.Diagnostics.HasError(), "diagnostics should have error from failed removal")
	mockPackager.AssertNotCalled(t, "Deploy", mock.Anything, mock.Anything, mock.Anything)
	mockPackager.AssertNotCalled(t, "LoadPackage", mock.Anything, mock.Anything, mock.Anything)
}

// Helper function to convert a slice of HelmChartPathValueModel to types.Set
func helmChartPathValueSliceToSet(values []HelmChartPathValueModel) types.Set {
	if len(values) == 0 {
		return types.SetNull(types.ObjectType{
			AttrTypes: map[string]attr.Type{
				"path":  types.StringType,
				"value": types.StringType,
			},
		})
	}

	elements := make([]attr.Value, len(values))
	for i, v := range values {
		elements[i] = types.ObjectValueMust(
			map[string]attr.Type{
				"path":  types.StringType,
				"value": types.StringType,
			},
			map[string]attr.Value{
				"path":  v.Path,
				"value": v.Value,
			},
		)
	}

	setValue, _ := types.SetValue(
		types.ObjectType{
			AttrTypes: map[string]attr.Type{
				"path":  types.StringType,
				"value": types.StringType,
			},
		},
		elements,
	)
	return setValue
}

// Helper function to convert a slice of ComponentChartValuesModel to types.Set
func componentChartValuesSliceToSet(overrides []ComponentChartValuesModel) types.Set {
	if len(overrides) == 0 {
		return types.SetNull(types.ObjectType{
			AttrTypes: map[string]attr.Type{
				"chart_name":       types.StringType,
				"values":           types.SetType{ElemType: types.ObjectType{AttrTypes: map[string]attr.Type{"path": types.StringType, "value": types.StringType}}},
				"sensitive_values": types.SetType{ElemType: types.ObjectType{AttrTypes: map[string]attr.Type{"path": types.StringType, "value": types.StringType}}},
			},
		})
	}

	elements := make([]attr.Value, len(overrides))
	pathValueSetType := types.SetType{ElemType: types.ObjectType{AttrTypes: map[string]attr.Type{"path": types.StringType, "value": types.StringType}}}

	for i, override := range overrides {
		// Ensure Values and SensitiveValues are properly initialized
		values := override.Values
		if values.IsNull() || values.IsUnknown() {
			values = types.SetNull(pathValueSetType.ElemType.(types.ObjectType))
		}
		sensitiveValues := override.SensitiveValues
		if sensitiveValues.IsNull() || sensitiveValues.IsUnknown() {
			sensitiveValues = types.SetNull(pathValueSetType.ElemType.(types.ObjectType))
		}

		elements[i] = types.ObjectValueMust(
			map[string]attr.Type{
				"chart_name":       types.StringType,
				"values":           pathValueSetType,
				"sensitive_values": pathValueSetType,
			},
			map[string]attr.Value{
				"chart_name":       override.ChartName,
				"values":           values,
				"sensitive_values": sensitiveValues,
			},
		)
	}

	setValue, _ := types.SetValue(
		types.ObjectType{
			AttrTypes: map[string]attr.Type{
				"chart_name":       types.StringType,
				"values":           types.SetType{ElemType: types.ObjectType{AttrTypes: map[string]attr.Type{"path": types.StringType, "value": types.StringType}}},
				"sensitive_values": types.SetType{ElemType: types.ObjectType{AttrTypes: map[string]attr.Type{"path": types.StringType, "value": types.StringType}}},
			},
		},
		elements,
	)
	return setValue
}

// Helper function to convert a slice of ComponentModel to types.Set
func componentSliceToSet(components []ComponentModel) types.Set {
	if len(components) == 0 {
		return types.SetNull(types.ObjectType{
			AttrTypes: map[string]attr.Type{
				"name": types.StringType,
				"override": types.SetType{ElemType: types.ObjectType{AttrTypes: map[string]attr.Type{
					"chart_name":       types.StringType,
					"values":           types.SetType{ElemType: types.ObjectType{AttrTypes: map[string]attr.Type{"path": types.StringType, "value": types.StringType}}},
					"sensitive_values": types.SetType{ElemType: types.ObjectType{AttrTypes: map[string]attr.Type{"path": types.StringType, "value": types.StringType}}},
				}}},
			},
		})
	}

	overrideSetType := types.SetType{ElemType: types.ObjectType{AttrTypes: map[string]attr.Type{
		"chart_name":       types.StringType,
		"values":           types.SetType{ElemType: types.ObjectType{AttrTypes: map[string]attr.Type{"path": types.StringType, "value": types.StringType}}},
		"sensitive_values": types.SetType{ElemType: types.ObjectType{AttrTypes: map[string]attr.Type{"path": types.StringType, "value": types.StringType}}},
	}}}

	elements := make([]attr.Value, len(components))
	for i, component := range components {
		overrides := component.Overrides
		if overrides.IsNull() || overrides.IsUnknown() {
			overrides = types.SetNull(overrideSetType.ElemType.(types.ObjectType))
		}

		elements[i] = types.ObjectValueMust(
			map[string]attr.Type{
				"name":     types.StringType,
				"override": overrideSetType,
			},
			map[string]attr.Value{
				"name":     component.Name,
				"override": overrides,
			},
		)
	}

	setValue, _ := types.SetValue(
		types.ObjectType{
			AttrTypes: map[string]attr.Type{
				"name":     types.StringType,
				"override": overrideSetType,
			},
		},
		elements,
	)
	return setValue
}

// Helper function to convert a slice of VariableModel to types.Set
func variableSliceToSet(vars []VariableModel) types.Set {
	if len(vars) == 0 {
		return types.SetNull(types.ObjectType{
			AttrTypes: map[string]attr.Type{
				"name":  types.StringType,
				"value": types.StringType,
			},
		})
	}

	elements := make([]attr.Value, len(vars))
	for i, v := range vars {
		elements[i] = types.ObjectValueMust(
			map[string]attr.Type{
				"name":  types.StringType,
				"value": types.StringType,
			},
			map[string]attr.Value{
				"name":  v.Name,
				"value": v.Value,
			},
		)
	}

	setValue, _ := types.SetValue(
		types.ObjectType{
			AttrTypes: map[string]attr.Type{
				"name":  types.StringType,
				"value": types.StringType,
			},
		},
		elements,
	)
	return setValue
}

// Unit test for flattenComponentOverrides function
func TestFlattenComponentOverrides(t *testing.T) {
	tests := []struct {
		name                  string
		input                 []ComponentModel
		expectedMap           map[string]map[string]map[string]any
		expectedErrorContains string
	}{
		{
			name: "single component without any chart overrides returns empty map",
			input: []ComponentModel{
				{
					Name:      types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{}),
				},
			},
			expectedMap:           map[string]map[string]map[string]any{},
			expectedErrorContains: "",
		},
		{
			name: "single component with single chart overrides block with no values or sensitive values returns empty map",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
						},
					}),
				},
			},
			expectedMap:           map[string]map[string]map[string]any{},
			expectedErrorContains: "",
		},
		{
			name: "single component with single chart overrides block with single simple path value",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1"), Value: types.StringValue("component1chart1path1Value")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"path1": "component1chart1path1Value",
					},
				},
			},
		},
		{
			name: "single component with single chart overrides block with single nested path value",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1.nestedPath"), Value: types.StringValue("component1chart1path1nestedPathValue")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"path1": map[string]any{
							"nestedPath": "component1chart1path1nestedPathValue",
						},
					},
				},
			},
		},
		{
			name: "single component with multiple chart overrides blocks each with single simple path value",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1"), Value: types.StringValue("component1chart1path1Value")},
							}),
						},
						{
							ChartName: types.StringValue("chart2"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1"), Value: types.StringValue("component1chart2path1Value")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"path1": "component1chart1path1Value",
					},
					"chart2": {
						"path1": "component1chart2path1Value",
					},
				},
			},
		},
		{
			name: "single component with multiple chart overrides blocks each with single nested path value",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1.nestedPath"), Value: types.StringValue("component1chart1path1nestedPathValue")},
							}),
						},
						{
							ChartName: types.StringValue("chart2"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1.nestedPath"), Value: types.StringValue("component1chart2path1nestedPathValue")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"path1": map[string]any{
							"nestedPath": "component1chart1path1nestedPathValue",
						},
					},
					"chart2": {
						"path1": map[string]any{
							"nestedPath": "component1chart2path1nestedPathValue",
						},
					},
				},
			},
		},
		{
			name: "single component with single chart overrides block with single simple path sensitive value",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1"), Value: types.StringValue("component1chart1sensitivePath1Value")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"sensitivePath1": "component1chart1sensitivePath1Value",
					},
				},
			},
		},
		{
			name: "single component with single chart overrides block with single nested path sensitive value",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1.nestedPath"), Value: types.StringValue("component1chart1sensitivePath1nestedPathValue")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"sensitivePath1": map[string]any{
							"nestedPath": "component1chart1sensitivePath1nestedPathValue",
						},
					},
				},
			},
		},
		{
			name: "single component with multiple chart overrides blocks each with single simple path sensitive value",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1"), Value: types.StringValue("component1chart1sensitivePath1Value")},
							}),
						},
						{
							ChartName: types.StringValue("chart2"),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1"), Value: types.StringValue("component1chart2sensitivePath1Value")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"sensitivePath1": "component1chart1sensitivePath1Value",
					},
					"chart2": {
						"sensitivePath1": "component1chart2sensitivePath1Value",
					},
				},
			},
		},
		{
			name: "single component with multiple chart overrides blocks each with single nested path sensitive value",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1.nestedPath"), Value: types.StringValue("component1chart1sensitivePath1nestedPathValue")},
							}),
						},
						{
							ChartName: types.StringValue("chart2"),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1.nestedPath"), Value: types.StringValue("component1chart2sensitivePath1nestedPathValue")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"sensitivePath1": map[string]any{
							"nestedPath": "component1chart1sensitivePath1nestedPathValue",
						},
					},
					"chart2": {
						"sensitivePath1": map[string]any{
							"nestedPath": "component1chart2sensitivePath1nestedPathValue",
						},
					},
				},
			},
		},
		{
			name: "single component with single chart overrides block with single simple path value and single simple path sensitive value",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1"), Value: types.StringValue("component1chart1path1Value")},
							}),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1"), Value: types.StringValue("component1chart1sensitivePath1Value")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"path1":          "component1chart1path1Value",
						"sensitivePath1": "component1chart1sensitivePath1Value",
					},
				},
			},
		},
		{
			name: "single component with single chart overrides block with single nested path value and single nested path sensitive value",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1.nestedPath"), Value: types.StringValue("component1chart1path1nestedPathValue")},
							}),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1.nestedPath"), Value: types.StringValue("component1chart1sensitivePath1nestedPathValue")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"path1": map[string]any{
							"nestedPath": "component1chart1path1nestedPathValue",
						},
						"sensitivePath1": map[string]any{
							"nestedPath": "component1chart1sensitivePath1nestedPathValue",
						},
					},
				},
			},
		},
		{
			name: "single component with multiple chart overrides blocks with both single simple path value and single simple path sensitive value",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1"), Value: types.StringValue("component1chart1path1Value")},
							}),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1"), Value: types.StringValue("component1chart1sensitivePath1Value")},
							}),
						},
						{
							ChartName: types.StringValue("chart2"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1"), Value: types.StringValue("component1chart2path1Value")},
							}),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1"), Value: types.StringValue("component1chart2sensitivePath1Value")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"path1":          "component1chart1path1Value",
						"sensitivePath1": "component1chart1sensitivePath1Value",
					},
					"chart2": {
						"path1":          "component1chart2path1Value",
						"sensitivePath1": "component1chart2sensitivePath1Value",
					},
				},
			},
		},
		{
			name: "single component with multiple chart overrides blocks with both single nested path value and single nested path sensitive value",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1.nestedPath"), Value: types.StringValue("component1chart1path1nestedPathValue")},
							}),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1.nestedPath"), Value: types.StringValue("component1chart1sensitivePath1nestedPathValue")},
							}),
						},
						{
							ChartName: types.StringValue("chart2"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1.nestedPath"), Value: types.StringValue("component1chart2path1nestedPathValue")},
							}),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1.nestedPath"), Value: types.StringValue("component1chart2sensitivePath1nestedPathValue")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"path1": map[string]any{
							"nestedPath": "component1chart1path1nestedPathValue",
						},
						"sensitivePath1": map[string]any{
							"nestedPath": "component1chart1sensitivePath1nestedPathValue",
						},
					},
					"chart2": {
						"path1": map[string]any{
							"nestedPath": "component1chart2path1nestedPathValue",
						},
						"sensitivePath1": map[string]any{
							"nestedPath": "component1chart2sensitivePath1nestedPathValue",
						},
					},
				},
			},
		},

		// MULTIPLE COMPONENTS
		{
			name: "multiple components without any chart overrides returns empty map",
			input: []ComponentModel{
				{
					Name:      types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{}),
				},
				{
					Name:      types.StringValue("component2"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{}),
				},
			},
			expectedMap:           map[string]map[string]map[string]any{},
			expectedErrorContains: "",
		},
		{
			name: "multiple components with single chart overrides block but no values or sensitive values returns empty map",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
						},
					}),
				},
				{
					Name: types.StringValue("component2"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
						},
					}),
				},
			},
			expectedMap:           map[string]map[string]map[string]any{},
			expectedErrorContains: "",
		},
		{
			name: "multiple components with single chart overrides block with single simple path value",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1"), Value: types.StringValue("component1chart1path1Value")},
							}),
						},
					}),
				},
				{
					Name: types.StringValue("component2"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1"), Value: types.StringValue("component2chart1path1Value")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"path1": "component1chart1path1Value",
					},
				},
				"component2": {
					"chart1": {
						"path1": "component2chart1path1Value",
					},
				},
			},
		},
		{
			name: "multiple components with single chart overrides block each with single nested path value",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1.nestedPath"), Value: types.StringValue("component1chart1path1nestedPathValue")},
							}),
						},
					}),
				},
				{
					Name: types.StringValue("component2"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1.nestedPath"), Value: types.StringValue("component2chart1path1nestedPathValue")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"path1": map[string]any{
							"nestedPath": "component1chart1path1nestedPathValue",
						},
					},
				},
				"component2": {
					"chart1": {
						"path1": map[string]any{
							"nestedPath": "component2chart1path1nestedPathValue",
						},
					},
				},
			},
		},
		//nolint:dupl // Test structure is similar to other multi-component tests, but tests specific scenario
		{
			name: "multiple components with multiple chart overrides blocks each with single simple path value",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1"), Value: types.StringValue("component1chart1path1Value")},
							}),
						},
						{
							ChartName: types.StringValue("chart2"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1"), Value: types.StringValue("component1chart2path1Value")},
							}),
						},
					}),
				},
				{
					Name: types.StringValue("component2"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1"), Value: types.StringValue("component2chart1path1Value")},
							}),
						},
						{
							ChartName: types.StringValue("chart2"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1"), Value: types.StringValue("component2chart2path1Value")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"path1": "component1chart1path1Value",
					},
					"chart2": {
						"path1": "component1chart2path1Value",
					},
				},
				"component2": {
					"chart1": {
						"path1": "component2chart1path1Value",
					},
					"chart2": {
						"path1": "component2chart2path1Value",
					},
				},
			},
		},
		//nolint:dupl // Test structure is similar to other multi-component tests, but tests nested paths
		{
			name: "multiple components with multiple chart overrides blocks each with single nested path value",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1.nestedPath"), Value: types.StringValue("component1chart1path1nestedPathValue")},
							}),
						},
						{
							ChartName: types.StringValue("chart2"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1.nestedPath"), Value: types.StringValue("component1chart2path1nestedPathValue")},
							}),
						},
					}),
				},
				{
					Name: types.StringValue("component2"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1.nestedPath"), Value: types.StringValue("component2chart1path1nestedPathValue")},
							}),
						},
						{
							ChartName: types.StringValue("chart2"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1.nestedPath"), Value: types.StringValue("component2chart2path1nestedPathValue")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"path1": map[string]any{
							"nestedPath": "component1chart1path1nestedPathValue",
						},
					},
					"chart2": {
						"path1": map[string]any{
							"nestedPath": "component1chart2path1nestedPathValue",
						},
					},
				},
				"component2": {
					"chart1": {
						"path1": map[string]any{
							"nestedPath": "component2chart1path1nestedPathValue",
						},
					},
					"chart2": {
						"path1": map[string]any{
							"nestedPath": "component2chart2path1nestedPathValue",
						},
					},
				},
			},
		},
		{
			name: "multiple components with single chart overrides block with single simple path sensitive value",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1"), Value: types.StringValue("component1chart1sensitivePath1Value")},
							}),
						},
					}),
				},
				{
					Name: types.StringValue("component2"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1"), Value: types.StringValue("component2chart1sensitivePath1Value")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"sensitivePath1": "component1chart1sensitivePath1Value",
					},
				},
				"component2": {
					"chart1": {
						"sensitivePath1": "component2chart1sensitivePath1Value",
					},
				},
			},
		},
		{
			name: "multiple components with single chart overrides block with single nested path sensitive value",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1.nestedPath"), Value: types.StringValue("component1chart1sensitivePath1nestedPathValue")},
							}),
						},
					}),
				},
				{
					Name: types.StringValue("component2"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1.nestedPath"), Value: types.StringValue("component2chart1sensitivePath1nestedPathValue")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"sensitivePath1": map[string]any{
							"nestedPath": "component1chart1sensitivePath1nestedPathValue",
						},
					},
				},
				"component2": {
					"chart1": {
						"sensitivePath1": map[string]any{
							"nestedPath": "component2chart1sensitivePath1nestedPathValue",
						},
					},
				},
			},
		},
		//nolint:dupl // Similar structure to regular values test, but tests sensitive values specifically
		{
			name: "multiple components with multiple chart overrides blocks each with single simple path sensitive value",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1"), Value: types.StringValue("component1chart1sensitivePath1Value")},
							}),
						},
						{
							ChartName: types.StringValue("chart2"),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1"), Value: types.StringValue("component1chart2sensitivePath1Value")},
							}),
						},
					}),
				},
				{
					Name: types.StringValue("component2"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1"), Value: types.StringValue("component2chart1sensitivePath1Value")},
							}),
						},
						{
							ChartName: types.StringValue("chart2"),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1"), Value: types.StringValue("component2chart2sensitivePath1Value")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"sensitivePath1": "component1chart1sensitivePath1Value",
					},
					"chart2": {
						"sensitivePath1": "component1chart2sensitivePath1Value",
					},
				},
				"component2": {
					"chart1": {
						"sensitivePath1": "component2chart1sensitivePath1Value",
					},
					"chart2": {
						"sensitivePath1": "component2chart2sensitivePath1Value",
					},
				},
			},
		},
		{
			name: "multiple components with single chart overrides block with single simple path value and single simple path sensitive value",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1"), Value: types.StringValue("component1chart1path1Value")},
							}),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1"), Value: types.StringValue("component1chart1sensitivePath1Value")},
							}),
						},
					}),
				},
				{
					Name: types.StringValue("component2"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1"), Value: types.StringValue("component2chart1path1Value")},
							}),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1"), Value: types.StringValue("component2chart1sensitivePath1Value")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"path1":          "component1chart1path1Value",
						"sensitivePath1": "component1chart1sensitivePath1Value",
					},
				},
				"component2": {
					"chart1": {
						"path1":          "component2chart1path1Value",
						"sensitivePath1": "component2chart1sensitivePath1Value",
					},
				},
			},
		},
		{
			name: "multiple components with single chart overrides block with single nested path value and single nested path sensitive value",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1.nestedPath"), Value: types.StringValue("component1chart1path1nestedPathValue")},
							}),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1.nestedPath"), Value: types.StringValue("component1chart1sensitivePath1nestedPathValue")},
							}),
						},
					}),
				},
				{
					Name: types.StringValue("component2"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1.nestedPath"), Value: types.StringValue("component2chart1path1nestedPathValue")},
							}),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1.nestedPath"), Value: types.StringValue("component2chart1sensitivePath1nestedPathValue")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"path1": map[string]any{
							"nestedPath": "component1chart1path1nestedPathValue",
						},
						"sensitivePath1": map[string]any{
							"nestedPath": "component1chart1sensitivePath1nestedPathValue",
						},
					},
				},
				"component2": {
					"chart1": {
						"path1": map[string]any{
							"nestedPath": "component2chart1path1nestedPathValue",
						},
						"sensitivePath1": map[string]any{
							"nestedPath": "component2chart1sensitivePath1nestedPathValue",
						},
					},
				},
			},
		},
		//nolint:dupl // Test structure is similar to other multi-component tests, but tests both values and sensitive values with simple paths
		{
			name: "multiple components with multiple chart overrides blocks with both single simple path value and single simple path sensitive value",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1"), Value: types.StringValue("component1chart1path1Value")},
							}),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1"), Value: types.StringValue("component1chart1sensitivePath1Value")},
							}),
						},
						{
							ChartName: types.StringValue("chart2"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1"), Value: types.StringValue("component1chart2path1Value")},
							}),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1"), Value: types.StringValue("component1chart2sensitivePath1Value")},
							}),
						},
					}),
				},
				{
					Name: types.StringValue("component2"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1"), Value: types.StringValue("component2chart1path1Value")},
							}),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1"), Value: types.StringValue("component2chart1sensitivePath1Value")},
							}),
						},
						{
							ChartName: types.StringValue("chart2"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1"), Value: types.StringValue("component2chart2path1Value")},
							}),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1"), Value: types.StringValue("component2chart2sensitivePath1Value")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"path1":          "component1chart1path1Value",
						"sensitivePath1": "component1chart1sensitivePath1Value",
					},
					"chart2": {
						"path1":          "component1chart2path1Value",
						"sensitivePath1": "component1chart2sensitivePath1Value",
					},
				},
				"component2": {
					"chart1": {
						"path1":          "component2chart1path1Value",
						"sensitivePath1": "component2chart1sensitivePath1Value",
					},
					"chart2": {
						"path1":          "component2chart2path1Value",
						"sensitivePath1": "component2chart2sensitivePath1Value",
					},
				},
			},
		},
		//nolint:dupl // Test structure is similar to other multi-component tests, but tests both values and sensitive values with nested paths
		{
			name: "multiple components with multiple chart overrides blocks with both single nested path value and single nested path sensitive value",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1.nestedPath"), Value: types.StringValue("component1chart1path1nestedPathValue")},
							}),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1.nestedPath"), Value: types.StringValue("component1chart1sensitivePath1nestedPathValue")},
							}),
						},
						{
							ChartName: types.StringValue("chart2"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1.nestedPath"), Value: types.StringValue("component1chart2path1nestedPathValue")},
							}),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1.nestedPath"), Value: types.StringValue("component1chart2sensitivePath1nestedPathValue")},
							}),
						},
					}),
				},
				{
					Name: types.StringValue("component2"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1.nestedPath"), Value: types.StringValue("component2chart1path1nestedPathValue")},
							}),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1.nestedPath"), Value: types.StringValue("component2chart1sensitivePath1nestedPathValue")},
							}),
						},
						{
							ChartName: types.StringValue("chart2"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1.nestedPath"), Value: types.StringValue("component2chart2path1nestedPathValue")},
							}),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1.nestedPath"), Value: types.StringValue("component2chart2sensitivePath1nestedPathValue")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"path1": map[string]any{
							"nestedPath": "component1chart1path1nestedPathValue",
						},
						"sensitivePath1": map[string]any{
							"nestedPath": "component1chart1sensitivePath1nestedPathValue",
						},
					},
					"chart2": {
						"path1": map[string]any{
							"nestedPath": "component1chart2path1nestedPathValue",
						},
						"sensitivePath1": map[string]any{
							"nestedPath": "component1chart2sensitivePath1nestedPathValue",
						},
					},
				},
				"component2": {
					"chart1": {
						"path1": map[string]any{
							"nestedPath": "component2chart1path1nestedPathValue",
						},
						"sensitivePath1": map[string]any{
							"nestedPath": "component2chart1sensitivePath1nestedPathValue",
						},
					},
					"chart2": {
						"path1": map[string]any{
							"nestedPath": "component2chart2path1nestedPathValue",
						},
						"sensitivePath1": map[string]any{
							"nestedPath": "component2chart2sensitivePath1nestedPathValue",
						},
					},
				},
			},
		},
		{
			name: "unescaped integer value is converted to int type",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("replicaCount"), Value: types.StringValue("3")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"replicaCount": 3,
					},
				},
			},
		},
		{
			name: "unescaped float value is converted to float type",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("cpuLimit"), Value: types.StringValue("1.5")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"cpuLimit": 1.5,
					},
				},
			},
		},
		{
			name: "unescaped boolean true value is converted to bool type",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("enabled"), Value: types.StringValue("true")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"enabled": true,
					},
				},
			},
		},
		{
			name: "unescaped boolean false value is converted to bool type",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("debug"), Value: types.StringValue("false")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"debug": false,
					},
				},
			},
		},
		{
			name: "quoted string value remains as string type",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("port"), Value: types.StringValue("\"8080\"")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"port": "8080",
					},
				},
			},
		},
		{
			name: "mixed types with nested paths are properly converted",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("config.replicas"), Value: types.StringValue("5")},
								{Path: types.StringValue("config.enabled"), Value: types.StringValue("true")},
								{Path: types.StringValue("config.name"), Value: types.StringValue("\"my-app\"")},
								{Path: types.StringValue("config.timeout"), Value: types.StringValue("30.5")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"config": map[string]any{
							"replicas": 5,
							"enabled":  true,
							"name":     "my-app",
							"timeout":  30.5,
						},
					},
				},
			},
		},
		{
			name: "sensitive values with unescaped integer are converted to int type",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("secretPort"), Value: types.StringValue("9000")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"secretPort": 9000,
					},
				},
			},
		},
		{
			name: "sensitive values with unescaped boolean are converted to bool type",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("tlsEnabled"), Value: types.StringValue("true")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"tlsEnabled": true,
					},
				},
			},
		},
		{
			name: "sensitive values with quoted strings remain as string type",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("apiKey"), Value: types.StringValue("\"secret123\"")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"apiKey": "secret123",
					},
				},
			},
		},
		{
			name: "mixed regular and sensitive values both get type conversion",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("replicas"), Value: types.StringValue("3")},
								{Path: types.StringValue("enabled"), Value: types.StringValue("true")},
							}),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("adminPort"), Value: types.StringValue("8443")},
								{Path: types.StringValue("debugMode"), Value: types.StringValue("false")},
							}),
						},
					}),
				},
			},
			expectedMap: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"replicas":  3,
						"enabled":   true,
						"adminPort": 8443,
						"debugMode": false,
					},
				},
			},
		},
		{
			name: "error when component has overrides with duplicate chart names",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
						},
						{
							ChartName: types.StringValue("chart1"),
						},
					}),
				},
			},
			expectedMap:           map[string]map[string]map[string]any{},
			expectedErrorContains: "chart 'chart1' is defined multiple times in component 'component1",
		},
		{
			name: "error when component overrides values has duplicate simple paths",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1"), Value: types.StringValue("value1")},
								{Path: types.StringValue("path1"), Value: types.StringValue("value2")},
							}),
						},
					}),
				},
			},
			expectedMap:           map[string]map[string]map[string]any{},
			expectedErrorContains: "path 'path1' is defined multiple times in overrides for chart 'chart1' of component 'component1'",
		},
		{
			name: "error when component overrides sensitive values has duplicate simple paths",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1"), Value: types.StringValue("value1")},
								{Path: types.StringValue("sensitivePath1"), Value: types.StringValue("value2")},
							}),
						},
					}),
				},
			},
			expectedMap:           map[string]map[string]map[string]any{},
			expectedErrorContains: "path 'sensitivePath1' is defined multiple times in overrides for chart 'chart1' of component 'component1'",
		},
		{
			name: "error when same simple path exists in both component overrides values and sensitive values",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1"), Value: types.StringValue("value1")},
							}),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1"), Value: types.StringValue("sensitiveValue1")},
							}),
						},
					}),
				},
			},
			expectedMap:           map[string]map[string]map[string]any{},
			expectedErrorContains: "path 'path1' is defined multiple times in overrides for chart 'chart1' of component 'component1'",
		},
		{
			name: "error when component overrides values has duplicate nested paths",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1.nestedPath"), Value: types.StringValue("value1")},
								{Path: types.StringValue("path1.nestedPath"), Value: types.StringValue("value2")},
							}),
						},
					}),
				},
			},
			expectedMap:           map[string]map[string]map[string]any{},
			expectedErrorContains: "path 'path1.nestedPath' is defined multiple times in overrides for chart 'chart1' of component 'component1'",
		},
		{
			name: "error when component overrides sensitive values has duplicate nested paths",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("sensitivePath1.nestedPath"), Value: types.StringValue("value1")},
								{Path: types.StringValue("sensitivePath1.nestedPath"), Value: types.StringValue("value2")},
							}),
						},
					}),
				},
			},
			expectedMap:           map[string]map[string]map[string]any{},
			expectedErrorContains: "path 'sensitivePath1.nestedPath' is defined multiple times in overrides for chart 'chart1' of component 'component1'",
		},
		{
			name: "error when nested path exists in both values and sensitive values arrays",
			input: []ComponentModel{
				{
					Name: types.StringValue("component1"),
					Overrides: componentChartValuesSliceToSet([]ComponentChartValuesModel{
						{
							ChartName: types.StringValue("chart1"),
							Values: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1.nestedPath"), Value: types.StringValue("value1")},
							}),
							SensitiveValues: helmChartPathValueSliceToSet([]HelmChartPathValueModel{
								{Path: types.StringValue("path1.nestedPath"), Value: types.StringValue("sensitiveValue1")},
							}),
						},
					}),
				},
			},
			expectedMap:           map[string]map[string]map[string]any{},
			expectedErrorContains: "path 'path1.nestedPath' is defined multiple times in overrides for chart 'chart1' of component 'component1'",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := flattenComponentOverrides(context.Background(), tc.input)

			if tc.expectedErrorContains != "" {
				assert.ErrorContains(t, err, tc.expectedErrorContains)
			} else {
				assert.Equal(t, tc.expectedMap, actual)
			}
		})
	}
}

func TestGetPackageOverrideName(t *testing.T) {
	t.Run("oci source uses full reference", func(t *testing.T) {
		source := "oci://ghcr.io/defenseunicorns/packages/test:latest"
		model := NewTestPackageResourceModel(WithSource(source))

		expectedHash := sha1.Sum([]byte(source))
		expectedName := fmt.Sprintf("zarf-package-%x.tar.zst", expectedHash)

		assert.Equal(t, expectedName, getPackageOverrideName(model))
	})

	t.Run("local path uses base filename", func(t *testing.T) {
		source := filepath.Join("some", "nested", "dir", "custom-package.tar.zst")
		model := NewTestPackageResourceModel(WithSource(source))

		expectedHash := sha1.Sum([]byte(filepath.Base(source)))
		expectedName := fmt.Sprintf("zarf-package-%x.tar.zst", expectedHash)

		assert.Equal(t, expectedName, getPackageOverrideName(model))
	})
}

func TestGetPackageSource_LocalPathOverride(t *testing.T) {
	t.Run("returns override path for oci source when file exists", func(t *testing.T) {
		tempDir := t.TempDir()

		model := NewTestPackageResourceModel()
		overrideFilename := getPackageOverrideName(model)
		overridePath := filepath.Join(tempDir, overrideFilename)

		err := os.WriteFile(overridePath, []byte("test"), 0o600)
		assert.NoError(t, err)
		defer os.Remove(overridePath)

		source, err := getPackageSource(model, udsProviderConfig{LocalPathOverride: tempDir})
		assert.NoError(t, err)
		assert.Equal(t, overridePath, source)
	})

	t.Run("returns error when override file missing", func(t *testing.T) {
		tempDir := t.TempDir()

		model := NewTestPackageResourceModel()
		source, err := getPackageSource(model, udsProviderConfig{LocalPathOverride: tempDir})
		assert.Error(t, err)
		assert.Equal(t, "", source)
	})
}

func TestPackageResource_ValidateConfig_SignatureVerification(t *testing.T) {
	tests := []struct {
		name        string
		configFunc  func() PackageResourceModel
		expectError bool
		errorMsg    string
	}{
		{
			name:        "no error when signature_verification absent",
			configFunc:  func() PackageResourceModel { return NewTestPackageResourceModel() },
			expectError: false,
		},
		{
			name: "no error with enabled=false and no key",
			configFunc: func() PackageResourceModel {
				return NewTestPackageResourceModel(WithSignatureVerificationEnabled(false))
			},
			expectError: false,
		},
		{
			name: "no error with public_key only",
			configFunc: func() PackageResourceModel {
				return NewTestPackageResourceModel(WithPublicKey("some-key"))
			},
			expectError: false,
		},
		{
			name: "no error with valid keyless config",
			configFunc: func() PackageResourceModel {
				return NewTestPackageResourceModel(WithKeylessVerification("user@example.com", "https://accounts.google.com"))
			},
			expectError: false,
		},
		{
			name: "error when public_key and keyless both set",
			configFunc: func() PackageResourceModel {
				return NewTestPackageResourceModel(
					withSigVerification(newTestSigVerification(true, "some-key", &KeylessVerificationModel{
						CertificateIdentity:         types.StringValue("user@example.com"),
						CertificateIdentityRegexp:   types.StringNull(),
						CertificateOIDCIssuer:       types.StringValue("https://accounts.google.com"),
						CertificateOIDCIssuerRegexp: types.StringNull(),
						TrustedRoot:                 types.StringNull(),
						InsecureIgnoreTlog:          types.BoolValue(false),
						UseSignedTimestamps:         types.BoolValue(false),
					})),
				)
			},
			expectError: true,
			errorMsg:    "public_key",
		},
		{
			name: "error when keyless missing identity",
			configFunc: func() PackageResourceModel {
				return NewTestPackageResourceModel(
					withSigVerification(newTestSigVerification(true, "", &KeylessVerificationModel{
						CertificateIdentity:         types.StringNull(),
						CertificateIdentityRegexp:   types.StringNull(),
						CertificateOIDCIssuer:       types.StringValue("https://accounts.google.com"),
						CertificateOIDCIssuerRegexp: types.StringNull(),
						TrustedRoot:                 types.StringNull(),
						InsecureIgnoreTlog:          types.BoolValue(false),
						UseSignedTimestamps:         types.BoolValue(false),
					})),
				)
			},
			expectError: true,
			errorMsg:    "certificate_identity",
		},
		{
			name: "error when keyless missing issuer",
			configFunc: func() PackageResourceModel {
				return NewTestPackageResourceModel(
					withSigVerification(newTestSigVerification(true, "", &KeylessVerificationModel{
						CertificateIdentity:         types.StringValue("user@example.com"),
						CertificateIdentityRegexp:   types.StringNull(),
						CertificateOIDCIssuer:       types.StringNull(),
						CertificateOIDCIssuerRegexp: types.StringNull(),
						TrustedRoot:                 types.StringNull(),
						InsecureIgnoreTlog:          types.BoolValue(false),
						UseSignedTimestamps:         types.BoolValue(false),
					})),
				)
			},
			expectError: true,
			errorMsg:    "certificate_oidc_issuer",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			model := tc.configFunc()
			resp := &resource.ValidateConfigResponse{Diagnostics: diag.Diagnostics{}}

			validateSignatureVerificationAttributes(context.Background(), model, resp)

			if tc.expectError {
				assert.True(t, resp.Diagnostics.HasError(), "expected error but got none")
				if tc.errorMsg != "" {
					found := false
					for _, d := range resp.Diagnostics.Errors() {
						if strings.Contains(d.Summary()+d.Detail(), tc.errorMsg) {
							found = true
							break
						}
					}
					assert.True(t, found, "expected error containing %q", tc.errorMsg)
				}
			} else {
				assert.False(t, resp.Diagnostics.HasError(), "unexpected error: %v", resp.Diagnostics.Errors())
			}
		})
	}
}

func TestPackageResource_Schema_SignatureVerificationDefault(t *testing.T) {
	packageResource := NewPackageResource(nil, nil, nil, nil).(*PackageResource)
	resp := &resource.SchemaResponse{}

	packageResource.Schema(context.Background(), resource.SchemaRequest{}, resp)
	assert.False(t, resp.Diagnostics.HasError(), "unexpected schema diagnostics: %v", resp.Diagnostics.Errors())

	signatureVerificationAttr, ok := resp.Schema.Attributes["signature_verification"].(schema.SingleNestedAttribute)
	assert.True(t, ok, "signature_verification should be a SingleNestedAttribute")
	assert.True(t, signatureVerificationAttr.Optional)
	assert.True(t, signatureVerificationAttr.Computed)
	assert.NotNil(t, signatureVerificationAttr.Default)

	defaultResp := &defaults.ObjectResponse{}
	signatureVerificationAttr.Default.DefaultObject(context.Background(), defaults.ObjectRequest{}, defaultResp)
	assert.False(t, defaultResp.Diagnostics.HasError(), "unexpected default diagnostics: %v", defaultResp.Diagnostics.Errors())
	assert.True(t, defaultSignatureVerification.Equal(defaultResp.PlanValue))

	var defaultModel SignatureVerificationModel
	diags := defaultResp.PlanValue.As(context.Background(), &defaultModel, basetypes.ObjectAsOptions{})
	assert.False(t, diags.HasError(), "unexpected default conversion diagnostics: %v", diags.Errors())
	assert.True(t, defaultModel.Verify.ValueBool())
	assert.True(t, defaultModel.PublicKey.IsNull())
	assert.True(t, defaultModel.Keyless.IsNull())
}

// WithKeylessVerification sets signature_verification.keyless with exact identity and issuer.
func WithKeylessVerification(identity, issuer string) PackageResourceModelDataOption {
	keyless := &KeylessVerificationModel{
		CertificateIdentity:         types.StringValue(identity),
		CertificateIdentityRegexp:   types.StringNull(),
		CertificateOIDCIssuer:       types.StringValue(issuer),
		CertificateOIDCIssuerRegexp: types.StringNull(),
		TrustedRoot:                 types.StringNull(),
		InsecureIgnoreTlog:          types.BoolValue(false),
		UseSignedTimestamps:         types.BoolValue(false),
	}
	return withSigVerification(newTestSigVerification(true, "", keyless))
}

func TestPackageResource_ValidateConfig_ValuesComponentMutualExclusivity(t *testing.T) {
	emptyDynamicObject := func() types.Dynamic {
		return types.DynamicValue(types.ObjectValueMust(map[string]attr.Type{}, map[string]attr.Value{}))
	}

	tests := []struct {
		name                  string
		model                 PackageResourceModel
		expectedErrorContains []string
	}{
		{
			name: "values without components is allowed",
			model: NewTestPackageResourceModel(
				WithValues(emptyDynamicObject()),
			),
		},
		{
			name: "sensitive_values without components is allowed",
			model: NewTestPackageResourceModel(
				WithSensitiveValues(emptyDynamicObject()),
			),
		},
		{
			name: "null values with components is allowed",
			model: NewTestPackageResourceModel(
				WithComponents(NewComponentModelsFromNames([]string{"test-component"})),
				WithValues(types.DynamicNull()),
				WithSensitiveValues(types.DynamicNull()),
			),
		},
		{
			name: "empty values object conflicts with components",
			model: NewTestPackageResourceModel(
				WithComponents(NewComponentModelsFromNames([]string{"test-component"})),
				WithValues(emptyDynamicObject()),
			),
			expectedErrorContains: []string{"values cannot be specified together with component blocks"},
		},
		{
			name: "empty sensitive_values object conflicts with components",
			model: NewTestPackageResourceModel(
				WithComponents(NewComponentModelsFromNames([]string{"test-component"})),
				WithSensitiveValues(emptyDynamicObject()),
			),
			expectedErrorContains: []string{"sensitive_values cannot be specified together with component blocks"},
		},
		{
			name: "values and sensitive_values both conflict with components",
			model: NewTestPackageResourceModel(
				WithComponents(NewComponentModelsFromNames([]string{"test-component"})),
				WithValues(emptyDynamicObject()),
				WithSensitiveValues(emptyDynamicObject()),
			),
			expectedErrorContains: []string{
				"values cannot be specified together with component blocks",
				"sensitive_values cannot be specified together with component blocks",
			},
		},
		{
			name: "unknown values conflicts with components",
			model: NewTestPackageResourceModel(
				WithComponents(NewComponentModelsFromNames([]string{"test-component"})),
				WithValues(types.DynamicUnknown()),
			),
			expectedErrorContains: []string{"values cannot be specified together with component blocks"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := &resource.ValidateConfigResponse{Diagnostics: diag.Diagnostics{}}

			validateValuesComponentMutualExclusivity(tc.model, resp)

			if len(tc.expectedErrorContains) == 0 {
				assert.False(t, resp.Diagnostics.HasError(), "expected no error but got: %v", resp.Diagnostics.Errors())
				return
			}

			assert.True(t, resp.Diagnostics.HasError(), "expected error but got none")
			for _, expected := range tc.expectedErrorContains {
				found := false
				for _, d := range resp.Diagnostics.Errors() {
					if strings.Contains(d.Detail(), expected) {
						found = true
						break
					}
				}
				assert.True(t, found, "expected error containing %q, got: %v", expected, resp.Diagnostics.Errors())
			}
		})
	}
}

func TestDynamicToValues(t *testing.T) {
	objectValue := types.ObjectValueMust(
		map[string]attr.Type{
			"string": types.StringType,
			"bool":   types.BoolType,
			"int":    types.NumberType,
			"float":  types.NumberType,
			"nested": types.ObjectType{AttrTypes: map[string]attr.Type{
				"list": types.ListType{ElemType: types.StringType},
			}},
		},
		map[string]attr.Value{
			"string": types.StringValue("value"),
			"bool":   types.BoolValue(true),
			"int":    types.NumberValue(big.NewFloat(3)),
			"float":  types.NumberValue(big.NewFloat(1.5)),
			"nested": types.ObjectValueMust(
				map[string]attr.Type{"list": types.ListType{ElemType: types.StringType}},
				map[string]attr.Value{
					"list": types.ListValueMust(types.StringType, []attr.Value{
						types.StringValue("one"),
						types.StringValue("two"),
					}),
				},
			),
		},
	)

	tests := []struct {
		name       string
		input      types.Dynamic
		expected   zarfValue.Values
		errorText  string
	}{
		{
			name:     "null returns empty values",
			input:    types.DynamicNull(),
			expected: zarfValue.Values{},
		},
		{
			name:      "unknown returns error",
			input:     types.DynamicUnknown(),
			errorText: "values must be known",
		},
		{
			name:  "object converts recursively",
			input: types.DynamicValue(objectValue),
			expected: zarfValue.Values{
				"string": "value",
				"bool":   true,
				"int":    int64(3),
				"float":  1.5,
				"nested": map[string]any{
					"list": []any{"one", "two"},
				},
			},
		},
		{
			name:      "root scalar returns error",
			input:     types.DynamicValue(types.StringValue("nope")),
			errorText: "values must be a map or object",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := dynamicToValues("values", tc.input)

			if tc.errorText != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorText)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func TestDynamicContainsUnknown(t *testing.T) {
	nestedKnownObject := types.DynamicValue(types.ObjectValueMust(
		map[string]attr.Type{
			"pod": types.ObjectType{AttrTypes: map[string]attr.Type{
				"annotations": types.ObjectType{AttrTypes: map[string]attr.Type{
					"pet-name": types.StringType,
				}},
			}},
		},
		map[string]attr.Value{
			"pod": types.ObjectValueMust(
				map[string]attr.Type{
					"annotations": types.ObjectType{AttrTypes: map[string]attr.Type{
						"pet-name": types.StringType,
					}},
				},
				map[string]attr.Value{
					"annotations": types.ObjectValueMust(
						map[string]attr.Type{"pet-name": types.StringType},
						map[string]attr.Value{"pet-name": types.StringValue("fluffy")},
					),
				},
			),
		},
	))
	nestedUnknownObject := types.DynamicValue(types.ObjectValueMust(
		map[string]attr.Type{
			"pod": types.ObjectType{AttrTypes: map[string]attr.Type{
				"annotations": types.ObjectType{AttrTypes: map[string]attr.Type{
					"pet-name": types.StringType,
				}},
			}},
		},
		map[string]attr.Value{
			"pod": types.ObjectValueMust(
				map[string]attr.Type{
					"annotations": types.ObjectType{AttrTypes: map[string]attr.Type{
						"pet-name": types.StringType,
					}},
				},
				map[string]attr.Value{
					"annotations": types.ObjectValueMust(
						map[string]attr.Type{"pet-name": types.StringType},
						map[string]attr.Value{"pet-name": types.StringUnknown()},
					),
				},
			),
		},
	))
	listWithUnknown := types.DynamicValue(types.ObjectValueMust(
		map[string]attr.Type{
			"items": types.ListType{ElemType: types.StringType},
		},
		map[string]attr.Value{
			"items": types.ListValueMust(types.StringType, []attr.Value{
				types.StringValue("known"),
				types.StringUnknown(),
			}),
		},
	))

	tests := []struct {
		name     string
		input    types.Dynamic
		expected bool
	}{
		{name: "null does not contain unknown", input: types.DynamicNull(), expected: false},
		{name: "top-level unknown contains unknown", input: types.DynamicUnknown(), expected: true},
		{name: "known nested object does not contain unknown", input: nestedKnownObject, expected: false},
		{name: "nested object contains unknown", input: nestedUnknownObject, expected: true},
		{name: "list contains unknown", input: listWithUnknown, expected: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, dynamicContainsUnknown(tc.input))
		})
	}
}

func TestCollectDynamicValuePaths(t *testing.T) {
	nestedKnownObject := types.DynamicValue(types.ObjectValueMust(
		map[string]attr.Type{
			"pod": types.ObjectType{AttrTypes: map[string]attr.Type{
				"annotations": types.ObjectType{AttrTypes: map[string]attr.Type{
					"pet-name": types.StringType,
				}},
				"empty": types.ObjectType{AttrTypes: map[string]attr.Type{}},
			}},
			"items": types.ListType{ElemType: types.StringType},
		},
		map[string]attr.Value{
			"pod": types.ObjectValueMust(
				map[string]attr.Type{
					"annotations": types.ObjectType{AttrTypes: map[string]attr.Type{
						"pet-name": types.StringType,
					}},
					"empty": types.ObjectType{AttrTypes: map[string]attr.Type{}},
				},
				map[string]attr.Value{
					"annotations": types.ObjectValueMust(
						map[string]attr.Type{"pet-name": types.StringType},
						map[string]attr.Value{"pet-name": types.StringValue("fluffy")},
					),
					"empty": types.ObjectValueMust(map[string]attr.Type{}, map[string]attr.Value{}),
				},
			),
			"items": types.ListValueMust(types.StringType, []attr.Value{
				types.StringValue("one"),
				types.StringValue("two"),
			}),
		},
	))
	nestedUnknownObject := types.DynamicValue(types.ObjectValueMust(
		map[string]attr.Type{
			"pod": types.ObjectType{AttrTypes: map[string]attr.Type{
				"annotations": types.ObjectType{AttrTypes: map[string]attr.Type{
					"pet-name": types.StringType,
				}},
			}},
		},
		map[string]attr.Value{
			"pod": types.ObjectValueMust(
				map[string]attr.Type{
					"annotations": types.ObjectType{AttrTypes: map[string]attr.Type{
						"pet-name": types.StringType,
					}},
				},
				map[string]attr.Value{
					"annotations": types.ObjectValueMust(
						map[string]attr.Type{"pet-name": types.StringType},
						map[string]attr.Value{"pet-name": types.StringUnknown()},
					),
				},
			),
		},
	))
	listWithUnknown := types.DynamicValue(types.ObjectValueMust(
		map[string]attr.Type{
			"items": types.ListType{ElemType: types.StringType},
		},
		map[string]attr.Value{
			"items": types.ListValueMust(types.StringType, []attr.Value{
				types.StringValue("known"),
				types.StringUnknown(),
			}),
		},
	))
	unknownIntermediateObject := types.DynamicValue(types.ObjectValueMust(
		map[string]attr.Type{
			"pod": types.DynamicType,
		},
		map[string]attr.Value{
			"pod": types.DynamicUnknown(),
		},
	))

	tests := []struct {
		name               string
		input              types.Dynamic
		expectedPaths      []plannedPackageValuePath
		expectedHasUnknown bool
		errorText          string
	}{
		{name: "null has no paths", input: types.DynamicNull(), expectedPaths: []plannedPackageValuePath{}},
		{name: "root unknown has no paths but records unknown", input: types.DynamicUnknown(), expectedPaths: []plannedPackageValuePath{}, expectedHasUnknown: true},
		{name: "known object returns leaf paths", input: nestedKnownObject, expectedPaths: []plannedPackageValuePath{{path: "pod.annotations.pet-name"}, {path: "pod.empty"}, {path: "items"}}},
		{name: "nested unknown scalar returns path and records unknown", input: nestedUnknownObject, expectedPaths: []plannedPackageValuePath{{path: "pod.annotations.pet-name"}}, expectedHasUnknown: true},
		{name: "unknown intermediate object returns unknown subtree path", input: unknownIntermediateObject, expectedPaths: []plannedPackageValuePath{{path: "pod", unknownSubtree: true}}, expectedHasUnknown: true},
		{name: "list with unknown returns list path and records unknown", input: listWithUnknown, expectedPaths: []plannedPackageValuePath{{path: "items"}}, expectedHasUnknown: true},
		{name: "root scalar returns error", input: types.DynamicValue(types.StringValue("nope")), errorText: "values must be a map or object"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			paths, hasUnknown, err := collectDynamicValuePaths(tc.input, "values")

			if tc.errorText != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorText)
				return
			}

			assert.NoError(t, err)
			assert.ElementsMatch(t, tc.expectedPaths, paths)
			assert.Equal(t, tc.expectedHasUnknown, hasUnknown)
		})
	}
}

func TestPackageResource_RunPackagePlanChecks_NestedUnknownValuesValidateKnownPaths(t *testing.T) {
	mockPackager := &MockPackager{}
	model := NewTestPackageResourceModel(
		WithOptionalComponents([]string{}),
		WithValues(types.DynamicValue(types.ObjectValueMust(
			map[string]attr.Type{
				"pod": types.ObjectType{AttrTypes: map[string]attr.Type{
					"annotations": types.ObjectType{AttrTypes: map[string]attr.Type{
						"pet-name": types.StringType,
					}},
				}},
			},
			map[string]attr.Value{
				"pod": types.ObjectValueMust(
					map[string]attr.Type{
						"annotations": types.ObjectType{AttrTypes: map[string]attr.Type{
							"pet-name": types.StringType,
						}},
					},
					map[string]attr.Value{
						"annotations": types.ObjectValueMust(
							map[string]attr.Type{"pet-name": types.StringType},
							map[string]attr.Value{"pet-name": types.StringUnknown()},
						),
					},
				),
			},
		))),
	)
	pkgLayout := &layout.PackageLayout{
		Pkg: v1alpha1.ZarfPackage{
			Components: []v1alpha1.ZarfComponent{
				{
					Name:     "required-component",
					Required: helpers.BoolPtr(true),
					Charts: []v1alpha1.ZarfChart{
						{
							Name: "chart",
							Values: []v1alpha1.ZarfChartValue{
								{SourcePath: ".pod.annotations", TargetPath: ".pod.annotations"},
							},
						},
					},
				},
			},
		},
	}
	mockPackager.On("LoadPackage", mock.Anything, mock.Anything, mock.Anything).Return(pkgLayout, nil)
	packageResource := NewPackageResource(&udsProviderConfig{ValidatePackagesOnPlan: true}, mockPackager, nil, nil).(*PackageResource)
	resp := &resource.ModifyPlanResponse{Diagnostics: diag.Diagnostics{}}

	valuePaths := collectAndValidatePackageValuePaths(model, resp)
	result := packageResource.runPackagePlanChecks(context.Background(), model, valuePaths)

	assert.False(t, resp.Diagnostics.HasError(), "expected nested unknown values with exposed paths to pass, got: %v", resp.Diagnostics.Errors())
	assert.Nil(t, result.LoadErr)
	assert.Nil(t, result.ValuePathsErr)
	mockPackager.AssertCalled(t, "LoadPackage", mock.Anything, mock.Anything, mock.Anything)
}

func TestPackageResource_RunPackagePlanChecks_UnknownIntermediateObjectSourcePathValidation(t *testing.T) {
	tests := []struct {
		name        string
		sourcePaths []string
		expectErr   bool
		errorText   string
	}{
		{
			name:        "defers when descendant source path is exposed",
			sourcePaths: []string{".pod.replicaCount"},
		},
		{
			name:        "fails when no source path can match unknown object",
			sourcePaths: []string{".service.enabled"},
			expectErr:   true,
			errorText:   "pod",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockPackager := &MockPackager{}
			model := NewTestPackageResourceModel(
				WithOptionalComponents([]string{}),
				WithValues(types.DynamicValue(types.ObjectValueMust(
					map[string]attr.Type{
						"pod": types.DynamicType,
					},
					map[string]attr.Value{
						"pod": types.DynamicUnknown(),
					},
				))),
			)
			chartValues := make([]v1alpha1.ZarfChartValue, 0, len(tc.sourcePaths))
			for _, sourcePath := range tc.sourcePaths {
				chartValues = append(chartValues, v1alpha1.ZarfChartValue{SourcePath: sourcePath, TargetPath: sourcePath})
			}
			pkgLayout := &layout.PackageLayout{
				Pkg: v1alpha1.ZarfPackage{
					Components: []v1alpha1.ZarfComponent{
						{
							Name:     "required-component",
							Required: helpers.BoolPtr(true),
							Charts: []v1alpha1.ZarfChart{
								{
									Name:   "chart",
									Values: chartValues,
								},
							},
						},
					},
				},
			}
			mockPackager.On("LoadPackage", mock.Anything, mock.Anything, mock.Anything).Return(pkgLayout, nil)
			packageResource := NewPackageResource(&udsProviderConfig{ValidatePackagesOnPlan: true}, mockPackager, nil, nil).(*PackageResource)
			resp := &resource.ModifyPlanResponse{Diagnostics: diag.Diagnostics{}}

			valuePaths := collectAndValidatePackageValuePaths(model, resp)
			result := packageResource.runPackagePlanChecks(context.Background(), model, valuePaths)

			assert.False(t, resp.Diagnostics.HasError(), "expected local validation to pass, got: %v", resp.Diagnostics.Errors())
			assert.NoError(t, result.LoadErr)
			if tc.expectErr {
				assert.Error(t, result.ValuePathsErr)
				assert.Contains(t, result.ValuePathsErr.Error(), tc.errorText)
			} else {
				assert.NoError(t, result.ValuePathsErr)
			}
			mockPackager.AssertCalled(t, "LoadPackage", mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestPackageResource_RunPackagePlanChecks_NestedUnknownValuesFailKnownUnexposedPaths(t *testing.T) {
	mockPackager := &MockPackager{}
	model := NewTestPackageResourceModel(
		WithOptionalComponents([]string{}),
		WithValues(types.DynamicValue(types.ObjectValueMust(
			map[string]attr.Type{
				"pod": types.ObjectType{AttrTypes: map[string]attr.Type{
					"annotations": types.ObjectType{AttrTypes: map[string]attr.Type{
						"pet-name": types.StringType,
					}},
				}},
				"image": types.ObjectType{AttrTypes: map[string]attr.Type{
					"tag": types.StringType,
				}},
			},
			map[string]attr.Value{
				"pod": types.ObjectValueMust(
					map[string]attr.Type{
						"annotations": types.ObjectType{AttrTypes: map[string]attr.Type{
							"pet-name": types.StringType,
						}},
					},
					map[string]attr.Value{
						"annotations": types.ObjectValueMust(
							map[string]attr.Type{"pet-name": types.StringType},
							map[string]attr.Value{"pet-name": types.StringUnknown()},
						),
					},
				),
				"image": types.ObjectValueMust(
					map[string]attr.Type{"tag": types.StringType},
					map[string]attr.Value{"tag": types.StringValue("1.2.3")},
				),
			},
		))),
	)
	pkgLayout := &layout.PackageLayout{
		Pkg: v1alpha1.ZarfPackage{
			Components: []v1alpha1.ZarfComponent{
				{
					Name:     "required-component",
					Required: helpers.BoolPtr(true),
					Charts: []v1alpha1.ZarfChart{
						{
							Name: "chart",
							Values: []v1alpha1.ZarfChartValue{
								{SourcePath: ".pod.annotations", TargetPath: ".pod.annotations"},
							},
						},
					},
				},
			},
		},
	}
	mockPackager.On("LoadPackage", mock.Anything, mock.Anything, mock.Anything).Return(pkgLayout, nil)
	packageResource := NewPackageResource(&udsProviderConfig{ValidatePackagesOnPlan: true}, mockPackager, nil, nil).(*PackageResource)
	resp := &resource.ModifyPlanResponse{Diagnostics: diag.Diagnostics{}}

	valuePaths := collectAndValidatePackageValuePaths(model, resp)
	result := packageResource.runPackagePlanChecks(context.Background(), model, valuePaths)

	assert.False(t, resp.Diagnostics.HasError(), "expected local validation to pass, got: %v", resp.Diagnostics.Errors())
	assert.NotNil(t, result.ValuePathsErr, "expected unexposed known path to fail")
	assert.Contains(t, result.ValuePathsErr.Error(), "image.tag")
	mockPackager.AssertCalled(t, "LoadPackage", mock.Anything, mock.Anything, mock.Anything)
}

func TestCollectAndValidatePackageValuePaths_RootUnknownValuesReturnNoPaths(t *testing.T) {
	model := NewTestPackageResourceModel(WithValues(types.DynamicUnknown()))
	resp := &resource.ModifyPlanResponse{Diagnostics: diag.Diagnostics{}}

	valuePaths := collectAndValidatePackageValuePaths(model, resp)

	assert.False(t, resp.Diagnostics.HasError(), "expected root unknown values to defer validation, got: %v", resp.Diagnostics.Errors())
	assert.Empty(t, valuePaths)
}

func TestPackageResource_RunPackagePlanChecks_ReturnsLoadErrWhenValuesRequirePackageLoad(t *testing.T) {
	mockPackager := &MockPackager{}
	model := NewTestPackageResourceModel(
		WithOptionalComponents([]string{}),
		WithValues(types.DynamicValue(types.ObjectValueMust(
			map[string]attr.Type{
				"pod": types.ObjectType{AttrTypes: map[string]attr.Type{
					"annotations": types.ObjectType{AttrTypes: map[string]attr.Type{
						"test": types.StringType,
					}},
				}},
			},
			map[string]attr.Value{
				"pod": types.ObjectValueMust(
					map[string]attr.Type{
						"annotations": types.ObjectType{AttrTypes: map[string]attr.Type{
							"test": types.StringType,
						}},
					},
					map[string]attr.Value{
						"annotations": types.ObjectValueMust(
							map[string]attr.Type{"test": types.StringType},
							map[string]attr.Value{"test": types.StringValue("value")},
						),
					},
				),
			},
		))),
	)
	mockPackager.On("LoadPackage", mock.Anything, mock.Anything, mock.Anything).Return(
		&layout.PackageLayout{Pkg: v1alpha1.ZarfPackage{}},
		fmt.Errorf("package metadata unavailable"),
	)
	packageResource := NewPackageResource(&udsProviderConfig{ValidatePackagesOnPlan: true}, mockPackager, nil, nil).(*PackageResource)
	resp := &resource.ModifyPlanResponse{Diagnostics: diag.Diagnostics{}}

	valuePaths := collectAndValidatePackageValuePaths(model, resp)
	result := packageResource.runPackagePlanChecks(context.Background(), model, valuePaths)

	assert.False(t, resp.Diagnostics.HasError(), "expected local validation to pass, got: %v", resp.Diagnostics.Errors())
	assert.NotNil(t, result.LoadErr)
	assert.Contains(t, result.LoadErr.Error(), "package metadata unavailable")
	mockPackager.AssertCalled(t, "LoadPackage", mock.Anything, mock.Anything, mock.Anything)
}

func TestValidateNoValueConflicts(t *testing.T) {
	tests := []struct {
		name            string
		values          zarfValue.Values
		sensitiveValues zarfValue.Values
		errorText       string
	}{
		{
			name:   "disjoint top-level keys are allowed",
			values: zarfValue.Values{"replicaCount": 2},
			sensitiveValues: zarfValue.Values{
				"token": "secret",
			},
		},
		{
			name: "sibling nested keys are allowed",
			values: zarfValue.Values{
				"db": map[string]any{"hostname": "postgres"},
			},
			sensitiveValues: zarfValue.Values{
				"db": map[string]any{"password": "secret"},
			},
		},
		{
			name: "duplicate leaf path conflicts",
			values: zarfValue.Values{
				"db": map[string]any{"password": "plain"},
			},
			sensitiveValues: zarfValue.Values{
				"db": map[string]any{"password": "secret"},
			},
			errorText: "db.password",
		},
		{
			name: "scalar and object conflict",
			values: zarfValue.Values{
				"db": "postgres",
			},
			sensitiveValues: zarfValue.Values{
				"db": map[string]any{"password": "secret"},
			},
			errorText: "db",
		},
		{
			name: "list and list conflict",
			values: zarfValue.Values{
				"tolerations": []any{"a"},
			},
			sensitiveValues: zarfValue.Values{
				"tolerations": []any{"b"},
			},
			errorText: "tolerations",
		},
		{
			name: "null and object conflict",
			values: zarfValue.Values{
				"db": nil,
			},
			sensitiveValues: zarfValue.Values{
				"db": map[string]any{"password": "secret"},
			},
			errorText: "db",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateNoValueConflicts(tc.values, tc.sensitiveValues)

			if tc.errorText != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorText)
				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestMergePackageValues(t *testing.T) {
	merged, err := mergePackageValues(
		zarfValue.Values{
			"db": map[string]any{
				"hostname": "postgres",
			},
			"replicaCount": 2,
		},
		zarfValue.Values{
			"db": map[string]any{
				"password": "secret",
			},
		},
	)

	assert.NoError(t, err)
	assert.Equal(t, zarfValue.Values{
		"db": map[string]any{
			"hostname": "postgres",
			"password": "secret",
		},
		"replicaCount": 2,
	}, merged)
}

func TestCollectValuePaths(t *testing.T) {
	paths := collectValuePaths(map[string]any{
		"pod": map[string]any{
			"replicaCount": int64(3),
			"annotations": map[string]any{
				"example.com/source": "terraform-provider-uds",
			},
			"empty": map[string]any{},
		},
		"logLevel": "info",
	})

	assert.ElementsMatch(t, []string{
		"pod.replicaCount",
		"pod.annotations.example.com/source",
		"pod.empty",
		"logLevel",
	}, paths)
}

func TestIsValuePathExposed(t *testing.T) {
	exposedPaths := []string{".pod.annotations", ".pod.replicaCount", ".logLevel"}

	tests := []struct {
		name        string
		valuePath   string
		exposedPath []string
		expected    bool
	}{
		{
			name:      "exact path is exposed",
			valuePath: "pod.replicaCount",
			expected:  true,
		},
		{
			name:      "descendant path is exposed by map source path",
			valuePath: "pod.annotations.example.com/source",
			expected:  true,
		},
		{
			name:      "unmapped sibling is not exposed",
			valuePath: "pod.unmapped",
			expected:  false,
		},
		{
			name:        "root source path exposes all values",
			valuePath:   "anything.nested",
			exposedPath: []string{"."},
			expected:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			paths := exposedPaths
			if tc.exposedPath != nil {
				paths = tc.exposedPath
			}
			assert.Equal(t, tc.expected, isValuePathExposed(tc.valuePath, paths))
		})
	}
}

func TestIsUnknownValuePathPotentiallyExposed(t *testing.T) {
	tests := []struct {
		name         string
		valuePath    string
		exposedPaths []string
		expected     bool
	}{
		{
			name:         "root source path potentially exposes unknown object",
			valuePath:    "pod",
			exposedPaths: []string{"."},
			expected:     true,
		},
		{
			name:         "exact source path potentially exposes unknown object",
			valuePath:    "pod",
			exposedPaths: []string{".pod"},
			expected:     true,
		},
		{
			name:         "descendant source path potentially exposes unknown object",
			valuePath:    "pod",
			exposedPaths: []string{".pod.replicaCount"},
			expected:     true,
		},
		{
			name:         "unrelated source path does not potentially expose unknown object",
			valuePath:    "pod",
			exposedPaths: []string{".service.enabled"},
			expected:     false,
		},
		{
			name:         "similar prefix source path does not potentially expose unknown object",
			valuePath:    "pod",
			exposedPaths: []string{".podinfo.replicaCount"},
			expected:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, isUnknownValuePathPotentiallyExposed(tc.valuePath, tc.exposedPaths))
		})
	}
}

func TestPackageResource_GetPlannedComponentValueSourcePaths(t *testing.T) {
	pkgLayout := &layout.PackageLayout{
		Pkg: v1alpha1.ZarfPackage{
			Components: []v1alpha1.ZarfComponent{
				{
					Name:     "required-component",
					Required: helpers.BoolPtr(true),
					Charts: []v1alpha1.ZarfChart{
						{
							Name: "required-chart",
							Values: []v1alpha1.ZarfChartValue{
								{SourcePath: ".required.value", TargetPath: ".required.value"},
							},
						},
					},
				},
				{
					Name:     "optional-component",
					Required: helpers.BoolPtr(false),
					Charts: []v1alpha1.ZarfChart{
						{
							Name: "optional-chart",
							Values: []v1alpha1.ZarfChartValue{
								{SourcePath: ".optional.value", TargetPath: ".optional.value"},
							},
						},
					},
				},
			},
		},
	}

	packageResource := NewPackageResource(nil, nil, nil, nil).(*PackageResource)
	tests := []struct {
		name          string
		model         PackageResourceModel
		expectedPaths []string
	}{
		{
			name:          "empty optional_components uses required components only",
			model:         NewTestPackageResourceModel(WithOptionalComponents([]string{})),
			expectedPaths: []string{".required.value"},
		},
		{
			name:          "selected optional component includes optional value paths",
			model:         NewTestPackageResourceModel(WithOptionalComponents([]string{"optional-component"})),
			expectedPaths: []string{".required.value", ".optional.value"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			paths, err := packageResource.getPlannedComponentValueSourcePaths(context.Background(), tc.model, pkgLayout)

			assert.NoError(t, err)
			assert.ElementsMatch(t, tc.expectedPaths, paths)
		})
	}
}

func TestBuildVerifyBlobOptions(t *testing.T) {
	tests := []struct {
		name                string
		setupModel          func() PackageResourceModel
		wantNil             bool
		wantKey             bool
		wantCertIdentity    string
		wantOIDCIssuer      string
		checkIgnoreTlog     bool
		wantIgnoreTlog      bool
		wantTrustedRootFile bool
	}{
		{
			name: "no verification config returns nil",
			setupModel: func() PackageResourceModel {
				return NewTestPackageResourceModel()
			},
			wantNil: true,
		},
		{
			name: "public key sets Key on options and writes key file",
			setupModel: func() PackageResourceModel {
				return NewTestPackageResourceModel(WithPublicKey("test-public-key-content"))
			},
			wantKey: true,
		},
		{
			name: "keyless sets cert identity and issuer, IgnoreTlog defaults false",
			setupModel: func() PackageResourceModel {
				return NewTestPackageResourceModel(
					WithKeylessVerification("test@example.com", "https://token.actions.githubusercontent.com"),
				)
			},
			wantCertIdentity: "test@example.com",
			wantOIDCIssuer:   "https://token.actions.githubusercontent.com",
			checkIgnoreTlog:  true,
			wantIgnoreTlog:   false,
		},
		{
			name: "keyless insecure_ignore_tlog=true sets IgnoreTlog=true",
			setupModel: func() PackageResourceModel {
				return NewTestPackageResourceModel(withSigVerification(newTestSigVerification(true, "", &KeylessVerificationModel{
					CertificateIdentity:         types.StringValue("test@example.com"),
					CertificateIdentityRegexp:   types.StringNull(),
					CertificateOIDCIssuer:       types.StringValue("https://token.actions.githubusercontent.com"),
					CertificateOIDCIssuerRegexp: types.StringNull(),
					TrustedRoot:                 types.StringNull(),
					InsecureIgnoreTlog:          types.BoolValue(true),
					UseSignedTimestamps:         types.BoolValue(false),
				})))
			},
			wantCertIdentity: "test@example.com",
			wantOIDCIssuer:   "https://token.actions.githubusercontent.com",
			checkIgnoreTlog:  true,
			wantIgnoreTlog:   true,
		},
		{
			name: "keyless with trusted_root writes file and sets TrustedRootPath",
			setupModel: func() PackageResourceModel {
				return NewTestPackageResourceModel(withSigVerification(newTestSigVerification(true, "", &KeylessVerificationModel{
					CertificateIdentity:         types.StringValue("test@example.com"),
					CertificateIdentityRegexp:   types.StringNull(),
					CertificateOIDCIssuer:       types.StringValue("https://token.actions.githubusercontent.com"),
					CertificateOIDCIssuerRegexp: types.StringNull(),
					TrustedRoot:                 types.StringValue(`{"mediaType":"application/vnd.dev.sigstore.trustedroot+json;version=0.1"}`),
					InsecureIgnoreTlog:          types.BoolValue(false),
					UseSignedTimestamps:         types.BoolValue(false),
				})))
			},
			wantCertIdentity:    "test@example.com",
			wantTrustedRootFile: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			model := tc.setupModel()

			opts, err := buildVerifyBlobOptions(context.Background(), model, tmpDir)
			assert.NoError(t, err)

			if tc.wantNil {
				assert.Nil(t, opts)
				return
			}

			assert.NotNil(t, opts)
			if tc.wantKey {
				assert.NotEmpty(t, opts.Key, "expected Key to be set")
				assert.FileExists(t, opts.Key)
			}
			if tc.wantCertIdentity != "" {
				assert.Equal(t, tc.wantCertIdentity, opts.CertVerify.CertIdentity)
			}
			if tc.wantOIDCIssuer != "" {
				assert.Equal(t, tc.wantOIDCIssuer, opts.CertVerify.CertOidcIssuer)
			}
			if tc.checkIgnoreTlog {
				assert.Equal(t, tc.wantIgnoreTlog, opts.CommonVerifyOptions.IgnoreTlog)
			}
			if tc.wantTrustedRootFile {
				assert.NotEmpty(t, opts.CommonVerifyOptions.TrustedRootPath)
				assert.FileExists(t, opts.CommonVerifyOptions.TrustedRootPath)
			}
		})
	}
}

func TestPackageResource_GetEffectiveSignatureVerification(t *testing.T) {
	tests := []struct {
		name       string
		setupModel func() PackageResourceModel
		expected   bool
	}{
		{
			name:       "absent signature_verification block returns true",
			setupModel: func() PackageResourceModel { return NewTestPackageResourceModel() },
			expected:   true,
		},
		{
			name: "signature_verification with enabled=true returns true",
			setupModel: func() PackageResourceModel {
				return NewTestPackageResourceModel(WithSignatureVerificationEnabled(true))
			},
			expected: true,
		},
		{
			name: "signature_verification with enabled=false returns false",
			setupModel: func() PackageResourceModel {
				return NewTestPackageResourceModel(WithSignatureVerificationEnabled(false))
			},
			expected: false,
		},
		{
			name: "signature_verification block present without explicit enabled defaults to true",
			setupModel: func() PackageResourceModel {
				sig := SignatureVerificationModel{
					Verify:    types.BoolNull(),
					PublicKey: types.StringNull(),
					Keyless:   types.ObjectNull(keylessVerificationAttrTypes),
				}
				return NewTestPackageResourceModel(withSigVerification(sig))
			},
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			model := tc.setupModel()
			result := getEffectiveSignatureVerification(context.Background(), model)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestHandleVerifyResult(t *testing.T) {
	someErr := errors.New("signature check failed")

	tests := []struct {
		name     string
		err      error
		isSigned bool
		enforce  bool
		wantErr  bool
	}{
		{
			name:     "no error returns nil",
			err:      nil,
			isSigned: true,
			enforce:  true,
			wantErr:  false,
		},
		{
			name:     "unsigned package warns and returns nil",
			err:      someErr,
			isSigned: false,
			enforce:  true,
			wantErr:  false,
		},
		{
			name:     "signed package with enforce=true returns error",
			err:      someErr,
			isSigned: true,
			enforce:  true,
			wantErr:  true,
		},
		{
			name:     "signed package with enforce=false warns and returns nil",
			err:      someErr,
			isSigned: true,
			enforce:  false,
			wantErr:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := handleVerifyResult(context.Background(), tc.err, tc.isSigned, tc.enforce)
			if tc.wantErr {
				assert.Error(t, result)
				assert.Equal(t, tc.err, result)
			} else {
				assert.NoError(t, result)
			}
		})
	}
}

func TestValidateComponentBlockOptionalComponentsMutualExclusivity(t *testing.T) {
	componentSet := componentSliceToSet([]ComponentModel{
		{Name: types.StringValue("some-component")},
	})
	emptyComponents := componentSliceToSet([]ComponentModel{})

	optionalWithNames := types.SetValueMust(types.StringType, []attr.Value{types.StringValue("metrics")})
	emptyOptional := types.SetValueMust(types.StringType, []attr.Value{})

	tests := []struct {
		name               string
		optionalComponents types.Set
		components         types.Set
		expectError        bool
	}{
		{
			name:               "allows null optional_components with component blocks",
			optionalComponents: types.SetNull(types.StringType),
			components:         componentSet,
			expectError:        false,
		},
		{
			name:               "allows null optional_components without component blocks",
			optionalComponents: types.SetNull(types.StringType),
			components:         emptyComponents,
			expectError:        false,
		},
		{
			name:               "allows non-empty optional_components without component blocks",
			optionalComponents: optionalWithNames,
			components:         emptyComponents,
			expectError:        false,
		},
		{
			name:               "allows empty optional_components without component blocks",
			optionalComponents: emptyOptional,
			components:         emptyComponents,
			expectError:        false,
		},
		{
			name:               "errors on empty optional_components with component blocks",
			optionalComponents: emptyOptional,
			components:         componentSet,
			expectError:        true,
		},
		{
			name:               "errors on non-empty optional_components with component blocks",
			optionalComponents: optionalWithNames,
			components:         componentSet,
			expectError:        true,
		},
		{
			name:               "errors on unknown optional_components with component blocks",
			optionalComponents: types.SetUnknown(types.StringType),
			components:         componentSet,
			expectError:        true,
		},
		{
			name:               "errors on optional_components with unknown component blocks",
			optionalComponents: optionalWithNames,
			components:         types.SetUnknown(componentSet.ElementType(context.Background())),
			expectError:        true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			model := PackageResourceModel{
				OptionalComponents: tc.optionalComponents,
				Components:         tc.components,
			}
			resp := &resource.ValidateConfigResponse{}
			validateComponentBlockOptionalComponentsMutualExclusivity(model, resp)
			if tc.expectError {
				assert.True(t, resp.Diagnostics.HasError(), "expected error but got none")
			} else {
				assert.False(t, resp.Diagnostics.HasError(), "expected no error but got: %v", resp.Diagnostics)
			}
		})
	}
}

func TestPackageResource_Upsert_OptionalComponents(t *testing.T) {
	tests := []struct {
		name                  string
		optionalComponents    *[]string // nil = null (component block path)
		components            []ComponentModel
		expectedFilteredNames []string
	}{
		{
			name:                  "uses component block path with no selected optionals when optional_components is null",
			optionalComponents:    nil,
			components:            []ComponentModel{},
			expectedFilteredNames: []string{},
		},
		{
			name:                  "calls filter with no optional components when optional_components is empty",
			optionalComponents:    &[]string{},
			components:            []ComponentModel{},
			expectedFilteredNames: []string{},
		},
		{
			name:                  "calls filter with the selected single optional component",
			optionalComponents:    &[]string{"test-optional-non-default-component-0"},
			components:            []ComponentModel{},
			expectedFilteredNames: []string{"test-optional-non-default-component-0"},
		},
		{
			name:                  "calls filter with all selected optional components",
			optionalComponents:    &[]string{"test-optional-non-default-component-0", "test-optional-non-default-component-1"},
			components:            []ComponentModel{},
			expectedFilteredNames: []string{"test-optional-non-default-component-0", "test-optional-non-default-component-1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			validLoadPackageResult := newValidLoadPackageResult()
			mockPackager := &MockPackager{}
			mockPackageComponentFilter := &MockPackageComponentFilter{}
			mockPackager.On("LoadPackage", mock.Anything, mock.Anything, mock.Anything).Return(validLoadPackageResult.Layout, nil)
			mockPackager.On("Deploy", mock.Anything, mock.Anything, mock.Anything).Return(packager.DeployResult{}, nil)
			mockPackageComponentFilter.On("ForDeploy", mock.Anything).Return(mock.Anything)

			packageResource := NewPackageResource(nil, mockPackager, mockPackageComponentFilter, nil).(*PackageResource)

			opts := []PackageResourceModelDataOption{WithComponents(tc.components)}
			if tc.optionalComponents != nil {
				opts = append(opts, WithOptionalComponents(*tc.optionalComponents))
			}

			_, err := packageResource.upsert(context.Background(), NewTestPackageResourceModel(opts...))
			assert.NoError(t, err)

			mockPackageComponentFilter.AssertCalled(t, "ForDeploy", mock.Anything)
			for _, call := range mockPackageComponentFilter.Calls {
				if call.Method == "ForDeploy" {
					got, _ := call.Arguments[0].([]string)
					assert.ElementsMatch(t, tc.expectedFilteredNames, got)
				}
			}
		})
	}
}

func TestPackageResource_Upsert_UnknownOptionalComponents(t *testing.T) {
	mockPackager := &MockPackager{}
	mockPackageComponentFilter := &MockPackageComponentFilter{}

	packageResource := NewPackageResource(nil, mockPackager, mockPackageComponentFilter, nil).(*PackageResource)
	model := NewTestPackageResourceModel(func(model *PackageResourceModel) {
		model.OptionalComponents = types.SetUnknown(types.StringType)
	})

	_, err := packageResource.upsert(context.Background(), model)
	require.EqualError(t, err, "optional_components must be known before apply")
	mockPackager.AssertNotCalled(t, "LoadPackage", mock.Anything, mock.Anything, mock.Anything)
	mockPackageComponentFilter.AssertNotCalled(t, "ForDeploy", mock.Anything)
	mockPackager.AssertNotCalled(t, "Deploy", mock.Anything, mock.Anything, mock.Anything)
}

func TestDeployedOptionalComponents(t *testing.T) {
	boolTrue := true
	boolFalse := false

	pkgComponents := []v1alpha1.ZarfComponent{
		{Name: "required-a", Required: &boolTrue},
		{Name: "required-b", Required: &boolTrue},
		{Name: "optional-nil-required", Required: nil},
		{Name: "optional-false-required", Required: &boolFalse},
	}

	tests := []struct {
		name               string
		deployedComponents []zarfState.DeployedComponent
		expected           []string
	}{
		{
			name: "returns empty when only required components are deployed",
			deployedComponents: []zarfState.DeployedComponent{
				{Name: "required-a"},
				{Name: "required-b"},
			},
			expected: []string{},
		},
		{
			name: "returns only optional components when required and optional components are deployed",
			deployedComponents: []zarfState.DeployedComponent{
				{Name: "required-a"},
				{Name: "required-b"},
				{Name: "optional-nil-required"},
			},
			expected: []string{"optional-nil-required"},
		},
		{
			name: "multiple optional components deployed",
			deployedComponents: []zarfState.DeployedComponent{
				{Name: "required-a"},
				{Name: "optional-nil-required"},
				{Name: "optional-false-required"},
			},
			expected: []string{"optional-nil-required", "optional-false-required"},
		},
		{
			name: "excludes deployed components that are not in the package definition",
			deployedComponents: []zarfState.DeployedComponent{
				{Name: "required-a"},
				{Name: "unknown-component"},
			},
			expected: []string{},
		},
		{
			name:               "returns empty when no components are deployed",
			deployedComponents: []zarfState.DeployedComponent{},
			expected:           []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := deployedOptionalComponents(tc.deployedComponents, pkgComponents)
			assert.ElementsMatch(t, tc.expected, result)
		})
	}
}

func TestRefreshOptionalComponentsFromDeployedPackage(t *testing.T) {
	boolTrue := true
	boolFalse := false

	deployedPkg := zarfState.DeployedPackage{
		DeployedComponents: []zarfState.DeployedComponent{
			{Name: "required-a"},
			{Name: "optional-nil-required"},
		},
		Data: v1alpha1.ZarfPackage{
			Components: []v1alpha1.ZarfComponent{
				{Name: "required-a", Required: &boolTrue},
				{Name: "optional-nil-required", Required: nil},
				{Name: "optional-false-required", Required: &boolFalse},
			},
		},
	}

	t.Run("null current returns null unchanged", func(t *testing.T) {
		current := types.SetNull(types.StringType)
		result, diags := refreshOptionalComponentsFromDeployedPackage(deployedPkg, current)
		assert.False(t, diags.HasError())
		assert.True(t, result.IsNull())
	})

	t.Run("unknown current resolves deployed optional components", func(t *testing.T) {
		current := types.SetUnknown(types.StringType)
		result, diags := refreshOptionalComponentsFromDeployedPackage(deployedPkg, current)
		assert.False(t, diags.HasError())
		var names []string
		result.ElementsAs(context.Background(), &names, false)
		assert.ElementsMatch(t, []string{"optional-nil-required"}, names)
	})

	t.Run("returns deployed optional components from package metadata", func(t *testing.T) {
		current := types.SetValueMust(types.StringType, []attr.Value{types.StringValue("optional-nil-required")})
		result, diags := refreshOptionalComponentsFromDeployedPackage(deployedPkg, current)
		assert.False(t, diags.HasError())
		var names []string
		result.ElementsAs(context.Background(), &names, false)
		assert.ElementsMatch(t, []string{"optional-nil-required"}, names)
	})

	t.Run("no deployed optionals returns empty set", func(t *testing.T) {
		pkgNoOptionals := zarfState.DeployedPackage{
			DeployedComponents: []zarfState.DeployedComponent{{Name: "required-a"}},
			Data:               v1alpha1.ZarfPackage{Components: []v1alpha1.ZarfComponent{{Name: "required-a", Required: &boolTrue}}},
		}
		current := types.SetValueMust(types.StringType, []attr.Value{})
		result, diags := refreshOptionalComponentsFromDeployedPackage(pkgNoOptionals, current)
		assert.False(t, diags.HasError())
		assert.Equal(t, 0, len(result.Elements()))
	})
}
