// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package provider

import (
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Unit test for flattenOverrides function
func TestFlattenOverrides(t *testing.T) {
	tests := []struct {
		name     string
		input    []OverrideModel
		expected map[string]map[string]map[string]interface{}
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
			expected: map[string]map[string]map[string]interface{}{
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
			expected: map[string]map[string]map[string]interface{}{
				"component1": {
					"chart1": {
						"ui": map[string]interface{}{
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
			expected: map[string]map[string]map[string]interface{}{
				"component1": {
					"chart1": {
						"replicaCount": "3",
					},
				},
				"component2": {
					"chart2": {
						"database": map[string]interface{}{
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
			expected: map[string]map[string]map[string]interface{}{
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
			expected: map[string]map[string]map[string]interface{}{
				"component1": {
					"chart1": {
						"app": map[string]interface{}{
							"settings": map[string]interface{}{
								"features": map[string]interface{}{
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
