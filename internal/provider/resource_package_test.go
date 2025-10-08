// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package provider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	zarfCluster "github.com/zarf-dev/zarf/src/pkg/cluster"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	zarfPackager "github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"

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

func WithPublicKey(publicKey string) PackageResourceModelDataOption {
	return func(model *PackageResourceModel) {
		model.PublicKey = types.StringValue(publicKey)
	}
}

func WithSkipSignatureValidation(skip bool) PackageResourceModelDataOption {
	return func(model *PackageResourceModel) {
		model.SkipSignatureValidation = types.BoolValue(skip)
	}
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

// NewTestPackageResourceModel creates a PackageResourceModel with default values and applies data options
func NewTestPackageResourceModel(options ...PackageResourceModelDataOption) PackageResourceModel {
	model := PackageResourceModel{
		Source:                  types.StringValue("oci://ghcr.io/defenseunicorns/packages/test:latest"),
		Architecture:            types.StringValue(runtime.GOARCH),
		PublicKey:               types.StringValue(""),
		SkipSignatureValidation: types.BoolValue(false),
		Timeout:                 types.StringValue("10m"),
		Namespace:               types.StringValue(""),
		Components:              componentSliceToSet([]ComponentModel{}),
		Vars:                    variableSliceToSet([]VariableModel{}),
		SensitiveVars:           variableSliceToSet([]VariableModel{}),
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
				NewTestComponentModel("test-required-component-0",
					WithComponentOverrides([]ComponentChartValuesModel{
						NewTestComponentChartValuesModel("chart1",
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
				NewTestComponentModel("test-optional-default-component-0",
					WithComponentOverrides([]ComponentChartValuesModel{
						NewTestComponentChartValuesModel("chart1",
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
				NewTestComponentModel("test-required-component-0",
					WithComponentOverrides([]ComponentChartValuesModel{
						NewTestComponentChartValuesModel("chart1",
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
				NewTestComponentModel("test-optional-default-component-0",
					WithComponentOverrides([]ComponentChartValuesModel{
						NewTestComponentChartValuesModel("chart1",
							WithComponentChartValues([]HelmChartPathValueModel{
								{Path: types.StringValue("replicaCount"), Value: types.StringValue("2")},
							}),
						),
						NewTestComponentChartValuesModel("chart2",
							WithComponentChartValues([]HelmChartPathValueModel{
								{Path: types.StringValue("service.port"), Value: types.StringValue("\"8080\"")},
							}),
						),
					}),
				),
				NewTestComponentModel("test-optional-non-default-component-0",
					WithComponentOverrides([]ComponentChartValuesModel{
						NewTestComponentChartValuesModel("chart3",
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
					err := os.MkdirAll(dir, 0755)
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
						Name:  types.StringValue("sensitive varjable_2"),
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

// Unit tests for upsert method public key and skip signature validation
func TestPackageResource_Upsert_PublicKeyAndSkipSignatureValidation(t *testing.T) {
	tests := []struct {
		name                                         string
		publicKey                                    string
		skipSignatureValidation                      bool
		zarfPackagerLoadPackageError                 error
		expectedLoadPackageWithPublicKeyPathProvided bool
		expectedCallToDeploy                         bool
	}{
		{
			name:                         "public key not provided with signature validation enabled for unsigned package loads package without public key file",
			publicKey:                    "",
			skipSignatureValidation:      false,
			zarfPackagerLoadPackageError: nil,
			expectedLoadPackageWithPublicKeyPathProvided: false,
			expectedCallToDeploy:                         true,
		},
		{
			name:                         "public key not provided with signature validation disabled for unsigned package loads package without public key file",
			publicKey:                    "",
			skipSignatureValidation:      true,
			zarfPackagerLoadPackageError: nil,
			expectedLoadPackageWithPublicKeyPathProvided: false,
			expectedCallToDeploy:                         true,
		},
		{
			name:                         "public key provided with signature validation enabled for signed package loads package with public key file",
			publicKey:                    "test-public-key",
			skipSignatureValidation:      false,
			zarfPackagerLoadPackageError: nil,
			expectedLoadPackageWithPublicKeyPathProvided: true,
			expectedCallToDeploy:                         true,
		},
		{
			name:                         "package key provided with signature validation disabled for signed package loads package without public key file",
			publicKey:                    "test-public-key",
			skipSignatureValidation:      true,
			zarfPackagerLoadPackageError: nil,
			expectedLoadPackageWithPublicKeyPathProvided: false,
			expectedCallToDeploy:                         true,
		},
		{
			name:                         "signed package load error when public key path not provided and signature validation enabled returns error without deploying",
			publicKey:                    "",
			skipSignatureValidation:      false,
			zarfPackagerLoadPackageError: fmt.Errorf("package is signed but no key was provided"),
			expectedLoadPackageWithPublicKeyPathProvided: false,
			expectedCallToDeploy:                         false,
		},
		{
			name:                         "unsigned package load error when public key path provided and signature validation enabled returns error without deploying",
			publicKey:                    "test-public-key",
			skipSignatureValidation:      false,
			zarfPackagerLoadPackageError: fmt.Errorf("a key was provided but the package is not signed"),
			expectedLoadPackageWithPublicKeyPathProvided: true,
			expectedCallToDeploy:                         false,
		},
		{
			name:                         "signed package load when public key path provided and signature validation enabled with mismatched or malformed key returns error without deploying",
			publicKey:                    "mismatched-or-malformed-public-key",
			skipSignatureValidation:      false,
			zarfPackagerLoadPackageError: fmt.Errorf("any error regarding mistmatched or malfored key"),
			expectedLoadPackageWithPublicKeyPathProvided: true,
			expectedCallToDeploy:                         false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockPackager := &MockPackager{}
			mockPackageComponentFilter := &MockPackageComponentFilter{}
			if tc.zarfPackagerLoadPackageError == nil {
				validLoadPackageResult := newValidLoadPackageResult()
				mockPackager.On("LoadPackage", mock.Anything, mock.Anything, mock.Anything).Return(
					validLoadPackageResult.Layout,
					validLoadPackageResult.Error,
				)
				mockPackager.On("Deploy", mock.Anything, mock.Anything, mock.Anything).Return(packager.DeployResult{}, nil)
				mockPackageComponentFilter.On("ForDeploy", mock.Anything).Return(mock.Anything)
			} else {
				errorLoadPackageResult := newErrorLoadPackageResult(tc.zarfPackagerLoadPackageError)
				mockPackager.On("LoadPackage", mock.Anything, mock.Anything, mock.Anything).Return(
					errorLoadPackageResult.Layout,
					errorLoadPackageResult.Error,
				)
			}

			packageResource := NewPackageResource(nil, mockPackager, mockPackageComponentFilter, nil).(*PackageResource)
			testModel := NewTestPackageResourceModel(
				WithPublicKey(tc.publicKey),
				WithSkipSignatureValidation(tc.skipSignatureValidation),
			)
			_, err := packageResource.upsert(context.Background(), testModel)

			mockPackageComponentFilter.AssertExpectations(t)
			if tc.zarfPackagerLoadPackageError != nil {
				assert.NotNil(t, err)
				assert.Equal(t, tc.zarfPackagerLoadPackageError, err)
			}

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
			publicKeyPathProvided := loadOptions.PublicKeyPath != ""
			if tc.expectedLoadPackageWithPublicKeyPathProvided {
				assert.True(t, publicKeyPathProvided,
					"Expected public key path to be provided but it was not. LoadOptions.PublicKeyPath: %s", loadOptions.PublicKeyPath)
			} else {
				assert.False(t, publicKeyPathProvided,
					"Expected public key path to not be provided but it was. LoadOptions.PublicKeyPath: %s", loadOptions.PublicKeyPath)
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
