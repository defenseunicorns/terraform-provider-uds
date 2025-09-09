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

// Unit test for flattenOverrides function
func TestFlattenOverrides(t *testing.T) {
	tests := []struct {
		name     string
		input    []OverrideModel
		expected map[string]map[string]map[string]any
	}{
		{
			name: "Single Override with Simple Value",
			input: []OverrideModel{
				{
					ComponentName: types.StringValue("component1"),
					ChartName:     types.StringValue("chart1"),
					Values: []OverrideValue{
						{Path: types.StringValue("replicaCount"), Value: types.StringValue("2")},
					},
				},
			},
			expected: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"replicaCount": "2",
					},
				},
			},
		},
		{
			name: "Override with Nested Variables",
			input: []OverrideModel{
				{
					ComponentName: types.StringValue("component1"),
					ChartName:     types.StringValue("chart1"),
					Variables: []OverrideVariable{
						{
							Path:    types.StringValue("ui.color"),
							Default: types.StringValue("purple"),
						},
					},
				},
			},
			expected: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"ui": map[string]any{
							"color": "purple",
						},
					},
				},
			},
		},
		{
			name: "Multiple Overrides",
			input: []OverrideModel{
				{
					ComponentName: types.StringValue("component1"),
					ChartName:     types.StringValue("chart1"),
					Values: []OverrideValue{
						{Path: types.StringValue("replicaCount"), Value: types.StringValue("3")},
					},
				},
				{
					ComponentName: types.StringValue("component2"),
					ChartName:     types.StringValue("chart2"),
					Variables: []OverrideVariable{
						{
							Path:    types.StringValue("database.url"),
							Default: types.StringValue("localhost"),
						},
					},
				},
			},
			expected: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"replicaCount": "3",
					},
				},
				"component2": {
					"chart2": {
						"database": map[string]any{
							"url": "localhost",
						},
					},
				},
			},
		},
		{
			name: "Variable with Empty Default Value (Should Be Ignored)",
			input: []OverrideModel{
				{
					ComponentName: types.StringValue("component1"),
					ChartName:     types.StringValue("chart1"),
					Variables: []OverrideVariable{
						{
							Path: types.StringValue("ui.theme"),
							// Default: basetypes.StringValue{},
						},
					},
				},
			},
			expected: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {},
				},
			},
		},
		{
			name: "Nested Variable Paths",
			input: []OverrideModel{
				{
					ComponentName: types.StringValue("component1"),
					ChartName:     types.StringValue("chart1"),
					Variables: []OverrideVariable{
						{
							Path:    types.StringValue("app.settings.features.enable"),
							Default: types.StringValue("true"),
						},
					},
				},
			},
			expected: map[string]map[string]map[string]any{
				"component1": {
					"chart1": {
						"app": map[string]any{
							"settings": map[string]any{
								"features": map[string]any{
									"enable": "true",
								},
							},
						},
					},
				},
			},
		},
	}

	// Run the test cases
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := flattenOverrides(tc.input)
			assert.Equal(t, tc.expected, actual)
		})
	}
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

func (m *MockPackageComponentFilter) ForRemove() filters.ComponentFilterStrategy {
	m.Called()
	return m.getPackageComponentFilter().ForRemove()
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

type PackageModelTestData struct {
	Name         string
	Source       string
	Architecture string
	Timeout      string
}

type ComponentModelTestData struct {
	Name string
	// TODO(erickson): Add chart overides in future
}

func NewPackageResourceModelFromTestData(packageModelData PackageModelTestData, componentModelData []ComponentModelTestData) PackageResourceModel {
	return PackageResourceModel{
		Name:         types.StringValue(packageModelData.Name),
		Source:       types.StringValue(packageModelData.Source),
		Architecture: types.StringValue(packageModelData.Architecture),
		Timeout:      types.StringValue(packageModelData.Timeout),
		Component:    NewComponentModelsFromTestData(componentModelData),
	}
}

func NewComponentModelsFromTestData(componentModelData []ComponentModelTestData) []ComponentModel {
	componentModels := make([]ComponentModel, 0, len(componentModelData))
	for _, data := range componentModelData {
		componentModels = append(componentModels, ComponentModel{
			Name: types.StringValue(data.Name),
		})
	}
	return componentModels
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
	validPackageModelData := PackageModelTestData{
		Name:         "test-package",
		Source:       "oci://ghcr.io/defenseunicornstest/packages/test-package:v0.0.1",
		Timeout:      "10m",
		Architecture: runtime.GOARCH,
	}

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

			packageResource := NewPackageResource(nil, mockPackager, mockPackageComponentFilter).(*PackageResource)
			testModel := NewPackageResourceModelFromTestData(validPackageModelData, []ComponentModelTestData{})
			testModel.Vars = tc.vars
			testModel.SensitiveVars = tc.sensitiveVars

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

	validPackageModelData := PackageModelTestData{
		Name:         "test-package",
		Source:       "oci://ghcr.io/defenseunicornstest/packages/test-package",
		Architecture: runtime.GOARCH,
		Timeout:      "10m",
	}

	tests := []struct {
		name                                      string
		packageModelData                          PackageModelTestData
		componentModelData                        []ComponentModelTestData
		zarfPackagerLoadPackageResult             MockLoadPackageResult
		expectedCallToDeploy                      bool
		expectedOptionalComponentsForDeployFilter []string
		expectedErrorContains                     []string
	}{
		{
			name:                          "package without components deploys required components only",
			packageModelData:              validPackageModelData,
			componentModelData:            []ComponentModelTestData{},
			zarfPackagerLoadPackageResult: newValidLoadPackageResult(),
			expectedCallToDeploy:          true,
			expectedOptionalComponentsForDeployFilter: []string{},
			expectedErrorContains:                     []string{},
		},
		{
			name:             "package with only required components deploys required components only",
			packageModelData: validPackageModelData,
			componentModelData: []ComponentModelTestData{
				{
					Name: "test-required-component-0",
				},
				{
					Name: "test-required-component-1",
				},
			},
			zarfPackagerLoadPackageResult:             newValidLoadPackageResult(),
			expectedCallToDeploy:                      true,
			expectedOptionalComponentsForDeployFilter: []string{},
			expectedErrorContains:                     []string{},
		},
		{
			name:             "package with only optional components deploys each optional component",
			packageModelData: validPackageModelData,
			componentModelData: []ComponentModelTestData{
				{
					Name: "test-optional-default-component-0",
				},
				{
					Name: "test-optional-non-default-component-0",
				},
			},
			zarfPackagerLoadPackageResult: newValidLoadPackageResult(),
			expectedCallToDeploy:          true,
			expectedOptionalComponentsForDeployFilter: []string{
				"test-optional-default-component-0",
				"test-optional-non-default-component-0",
			},
			expectedErrorContains: []string{},
		},
		{
			name:             "package with required and optional components deploys each optional component",
			packageModelData: validPackageModelData,
			componentModelData: []ComponentModelTestData{
				{
					Name: "test-required-component-0",
				},
				{
					Name: "test-required-component-1",
				},
				{
					Name: "test-optional-default-component-0",
				},
				{
					Name: "test-optional-non-default-component-0",
				},
			},
			zarfPackagerLoadPackageResult: newValidLoadPackageResult(),
			expectedCallToDeploy:          true,
			expectedOptionalComponentsForDeployFilter: []string{
				"test-optional-default-component-0",
				"test-optional-non-default-component-0",
			},
			expectedErrorContains: []string{},
		},
		{
			name:             "package with unknown components returns error for each unknown component",
			packageModelData: validPackageModelData,
			componentModelData: []ComponentModelTestData{
				{
					Name: "test-unknown-component-0",
				},
				{
					Name: "test-unknown-component-1",
				},
			},
			zarfPackagerLoadPackageResult:             newValidLoadPackageResult(),
			expectedCallToDeploy:                      false,
			expectedOptionalComponentsForDeployFilter: []string{},
			expectedErrorContains: []string{
				"test-unknown-component-0 not found in package",
				"test-unknown-component-1 not found in package",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockPackager := &MockPackager{}
			mockPackageComponentFilter := &MockPackageComponentFilter{}

			mockPackager.On("LoadPackage", mock.Anything, mock.Anything, mock.Anything).Return(
				tc.zarfPackagerLoadPackageResult.Layout,
				tc.zarfPackagerLoadPackageResult.Error,
			)
			if tc.expectedCallToDeploy {
				mockPackager.On("Deploy", mock.Anything, mock.Anything, mock.Anything).Return(packager.DeployResult{}, nil)
				mockPackageComponentFilter.On("ForDeploy", mock.Anything).Return(mock.Anything)
			}

			packageResource := NewPackageResource(nil, mockPackager, mockPackageComponentFilter).(*PackageResource)
			testModel := NewPackageResourceModelFromTestData(tc.packageModelData, tc.componentModelData)
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

			packageModelData := PackageModelTestData{
				Name:         "test-package",
				Source:       tc.source,
				Architecture: runtime.GOARCH,
				Timeout:      "10m",
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

			packageResource := NewPackageResource(nil, mockPackager, mockPackageComponentFilter).(*PackageResource)
			testModel := NewPackageResourceModelFromTestData(packageModelData, []ComponentModelTestData{})
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
				Vars:          []VariableModel{},
				SensitiveVars: []VariableModel{},
			},
		},
		{
			name:               "only regular vars, no duplicates",
			expectedErrorCount: 0,
			model: PackageResourceModel{
				Vars: []VariableModel{
					{
						Name:  types.StringValue("variable_1"),
						Value: types.StringValue("value 1"),
					},
				},
				SensitiveVars: []VariableModel{},
			},
		},
		{
			name:               "only regular vars, with duplicates",
			expectedErrorCount: 1,
			model: PackageResourceModel{
				Vars: []VariableModel{
					{
						Name:  types.StringValue("variable_1"),
						Value: types.StringValue("value 1"),
					},
					{
						Name:  types.StringValue("variable_1"),
						Value: types.StringValue("duplicate value"),
					},
				},
				SensitiveVars: []VariableModel{},
			},
		},
		{
			name:               "only sensitive vars, no duplicates",
			expectedErrorCount: 0,
			model: PackageResourceModel{
				Vars: []VariableModel{},
				SensitiveVars: []VariableModel{
					{
						Name:  types.StringValue("sensitive variable_1"),
						Value: types.StringValue("sensitive value"),
					},
					{
						Name:  types.StringValue("sensitive variable_2"),
						Value: types.StringValue("sensitive value"),
					},
				},
			},
		},
		{
			name:               "only sensitive vars, with duplicates",
			expectedErrorCount: 1,
			model: PackageResourceModel{
				Vars: []VariableModel{},
				SensitiveVars: []VariableModel{
					{
						Name:  types.StringValue("sensitive variable_1"),
						Value: types.StringValue("sensitive value"),
					},
					{
						Name:  types.StringValue("sensitive variable_1"),
						Value: types.StringValue("sensitive value"),
					},
				},
			},
		},
		{
			name:               "both var types, no duplicates",
			expectedErrorCount: 0,
			model: PackageResourceModel{
				Vars: []VariableModel{
					{
						Name:  types.StringValue("variable_1"),
						Value: types.StringValue("value 1"),
					},
					{
						Name:  types.StringValue("variable_2"),
						Value: types.StringValue("duplicate value"),
					},
				},
				SensitiveVars: []VariableModel{
					{
						Name:  types.StringValue("sensitive variable_1"),
						Value: types.StringValue("sensitive value"),
					},
					{
						Name:  types.StringValue("sensitive varjable_2"),
						Value: types.StringValue("sensitive value"),
					},
				},
			},
		},
		{
			name:               "both var types, with duplicates",
			expectedErrorCount: 2,
			model: PackageResourceModel{
				Vars: []VariableModel{
					{
						Name:  types.StringValue("variable_1"),
						Value: types.StringValue("value 1"),
					},
					{
						Name:  types.StringValue("variable_2"),
						Value: types.StringValue("duplicate value"),
					},
				},
				SensitiveVars: []VariableModel{
					{
						Name:  types.StringValue("variable_1"),
						Value: types.StringValue("sensitive value"),
					},
					{
						Name:  types.StringValue("variable_2"),
						Value: types.StringValue("sensitive value"),
					},
				},
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
	validPackageModelData := PackageModelTestData{
		Name:         "test-package",
		Source:       "oci://ghcr.io/defenseunicornstest/packages/test-package:v0.0.1",
		Timeout:      "10m",
		Architecture: runtime.GOARCH,
	}

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

			packageResource := NewPackageResource(nil, mockPackager, mockPackageComponentFilter).(*PackageResource)
			testModel := NewPackageResourceModelFromTestData(validPackageModelData, []ComponentModelTestData{})
			testModel.Namespace = types.StringValue(tc.namespace)

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
