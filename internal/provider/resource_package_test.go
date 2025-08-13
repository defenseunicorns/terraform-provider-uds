// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package provider

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"testing"

	udsPackager "github.com/defenseunicorns/terraform-provider-uds/internal/packager"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/zarf-dev/zarf/src/api/v1alpha1"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/packager/filters"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
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
			got := flattenOverrides(tc.input)
			if !reflect.DeepEqual(got, tc.expected) {
				t.Errorf("flattenOverrides() = %v, expected %v", got, tc.expected)
			}
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

type MockPackageComponentFilter struct {
	mock.Mock

	packageComponentFilter udsPackager.PackageComponentFilter
}

func (m *MockPackageComponentFilter) getPackageComponentFilter() udsPackager.PackageComponentFilter {
	if m.packageComponentFilter == nil {
		m.packageComponentFilter = udsPackager.NewPackageComponentFilter()
	}
	return m.packageComponentFilter
}

func (m *MockPackageComponentFilter) ForRemove() filters.ComponentFilterStrategy {
	m.Called()
	return m.getPackageComponentFilter().ForRemove()
}

func (m *MockPackageComponentFilter) ForDeploy(optionalComponents []string) filters.ComponentFilterStrategy {
	m.Called(optionalComponents)
	return m.getPackageComponentFilter().ForDeploy(optionalComponents)
}

type MockLoadPackageResult struct {
	Layout *layout.PackageLayout
	Error  error
}

type PackageModelTestData struct {
	Name         string
	Repository   string
	Ref          string
	Timeout      string
	Architecture string
}

type ComponentModelTestData struct {
	Name string
	// TODO(erickson): Add chart overides in future
}

func NewPackageResourceModelFromTestData(packageModelData PackageModelTestData, componentModelData []ComponentModelTestData) PackageResourceModel {
	return PackageResourceModel{
		Name:         types.StringValue(packageModelData.Name),
		Repository:   types.StringValue(packageModelData.Repository),
		Ref:          types.StringValue(packageModelData.Ref),
		Timeout:      types.StringValue(packageModelData.Timeout),
		Architecture: types.StringValue(packageModelData.Architecture),
		Component:    NewComponentModelsFromTestData(componentModelData),
	}
}

func NewComponentModelsFromTestData(componentModelData []ComponentModelTestData) []ComponentModel {
	var componentModels []ComponentModel
	for _, data := range componentModelData {
		componentModels = append(componentModels, ComponentModel{
			Name: types.StringValue(data.Name),
		})
	}
	return componentModels
}

func TestPackageResource_Upsert_OptionalComponentInstallation(t *testing.T) {

	validPackageModelData := PackageModelTestData{
		Name:         "test-package",
		Repository:   "ghcr.io/defenseunicornstest/packages/test-package",
		Ref:          "v0.0.1",
		Timeout:      "10m",
		Architecture: runtime.GOARCH,
	}

	// Helper function to create fresh MockLoadPackageResult for each test
	newValidLoadPackageResult := func() MockLoadPackageResult {
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
							Required: boolPtr(true),
							Default:  false,
						},
						{
							Name:     "test-required-component-1",
							Required: boolPtr(true),
							Default:  false,
						},
						{
							Name:     "test-optional-default-component-0",
							Required: nil, // Why zarf, why?
							Default:  true,
						},
						{
							Name:     "test-optional-default-component-1",
							Required: boolPtr(false),
							Default:  true,
						},
						{
							Name:     "test-optional-non-default-component-0",
							Required: nil,
							Default:  false,
						},
						{
							Name:     "test-optional-non-default-component-1",
							Required: boolPtr(false),
							Default:  false,
						},
					},
				},
			},
			Error: nil,
		}
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

			_, err := packageResource.upsert(context.Background(), testModel)

			if err == nil && len(tc.expectedErrorContains) > 0 {
				t.Errorf("Expected error, got none")
				return
			}
			if err != nil && len(tc.expectedErrorContains) == 0 {
				t.Errorf("Expected no error, got %v", err)
				return
			}
			for _, expectedErrorMsg := range tc.expectedErrorContains {
				if !strings.Contains(err.Error(), expectedErrorMsg) {
					t.Errorf("Expected error to contain %q, but got: %v", expectedErrorMsg, err)
					return
				}
			}

			// Check that ForDeploy was called with expected optional components
			mockPackageComponentFilter.AssertExpectations(t)
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

// Helper function to create bool pointers
func boolPtr(b bool) *bool {
	return &b
}
