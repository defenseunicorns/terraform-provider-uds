// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package validator

import (
	"context"
	"testing"

	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	tfvalidator "github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func newObjectListWithAttributeValues(attributeName string, values []string) types.List {
	attrValues := make([]attr.Value, len(values))
	for i, value := range values {
		attrValues[i] = types.ObjectValueMust(
			map[string]attr.Type{attributeName: types.StringType},
			map[string]attr.Value{attributeName: types.StringValue(value)},
		)
	}

	return types.ListValueMust(
		types.ObjectType{AttrTypes: map[string]attr.Type{attributeName: types.StringType}},
		attrValues,
	)
}

func TestBlockStringAttributeUniquenessValidator_ValidateList(t *testing.T) {
	block_name := "test_block"
	attributeName := "test_attr"
	duplicateAttributeErrorSummary := "Duplicate block attribute."
	incorrectAttributeTypeErrorSummary := "Incorrect block attribute type."
	missingAttributeErrorSummary := "Missing block attribute."
	tests := []struct {
		name                 string
		configValue          types.List
		expectedErrorCount   int
		expectedErrorSummary string
	}{
		{
			name:               "null list is valid",
			configValue:        types.ListNull(types.ObjectType{AttrTypes: map[string]attr.Type{attributeName: types.StringType}}),
			expectedErrorCount: 0,
		},
		{
			name:               "unknown list is valid",
			configValue:        types.ListUnknown(types.ObjectType{AttrTypes: map[string]attr.Type{attributeName: types.StringType}}),
			expectedErrorCount: 0,
		},
		{
			name: "empty list is valid",
			configValue: types.ListValueMust(types.ObjectType{AttrTypes: map[string]attr.Type{attributeName: types.StringType}},
				[]attr.Value{},
			),
			expectedErrorCount: 0,
		},
		{
			name:               "single item is valid",
			configValue:        newObjectListWithAttributeValues(attributeName, []string{"value1"}),
			expectedErrorCount: 0,
		},
		{
			name:               "multiple unique items is valid",
			configValue:        newObjectListWithAttributeValues(attributeName, []string{"value1", "value2", "value3"}),
			expectedErrorCount: 0,
		},
		{
			name:                 "duplicate items is not valid",
			configValue:          newObjectListWithAttributeValues(attributeName, []string{"value1", "value2", "value1"}),
			expectedErrorCount:   1,
			expectedErrorSummary: duplicateAttributeErrorSummary,
		},
		{
			name:                 "multiple duplicates is not valid with multiple errors",
			configValue:          newObjectListWithAttributeValues(attributeName, []string{"value1", "value1", "value2", "value2"}),
			expectedErrorCount:   2,
			expectedErrorSummary: duplicateAttributeErrorSummary,
		},
		{
			name:                 "missing attribute is not valid",
			configValue:          newObjectListWithAttributeValues("other_attr", []string{"value1", "value2"}),
			expectedErrorCount:   1,
			expectedErrorSummary: missingAttributeErrorSummary,
		},
		{
			name:               "unknown attribute value should be skipped",
			configValue:        newObjectListWithAttributeValues(attributeName, []string{"value1", " ", "value2"}),
			expectedErrorCount: 0,
		},
		{
			name: "wrong attribute type is not valid",
			configValue: types.ListValueMust(
				types.ObjectType{AttrTypes: map[string]attr.Type{attributeName: types.Int64Type}},
				[]attr.Value{
					types.ObjectValueMust(
						map[string]attr.Type{attributeName: types.Int64Type},
						map[string]attr.Value{attributeName: types.Int64Value(123)},
					),
				},
			),
			expectedErrorCount:   1,
			expectedErrorSummary: incorrectAttributeTypeErrorSummary,
		},
	}

	validator, _ := NewBlockStringAttributeUniquenessValidator(block_name, attributeName)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := tfvalidator.ListRequest{
				ConfigValue: tc.configValue,
			}
			resp := &tfvalidator.ListResponse{
				Diagnostics: diag.Diagnostics{},
			}

			validator.ValidateList(context.Background(), req, resp)

			if len(resp.Diagnostics) != tc.expectedErrorCount {
				t.Errorf("Expected %d errors, got %d. Diagnostics: %v", tc.expectedErrorCount, len(resp.Diagnostics), resp.Diagnostics)
			}

			if tc.expectedErrorSummary != "" && len(resp.Diagnostics) > 0 {
				found := false
				for _, diag := range resp.Diagnostics {
					if diag.Summary() == tc.expectedErrorSummary {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected error with summary %q, but it was not found in diagnostics: %v", tc.expectedErrorSummary, resp.Diagnostics)
				}
			}
		})
	}
}

func TestNewBlockStringAttributeUniquenessValidator(t *testing.T) {
	validBlockName := "test_block"
	validAttributeName := "test_attr"

	tests := []struct {
		name          string
		blockName     string
		attributeName string
		expectError   bool
		errorContains string
	}{
		{
			name:          "block name with mixed case letters is valid",
			blockName:     "testBlock",
			attributeName: validAttributeName,
			expectError:   false,
		},
		{
			name:          "block name with letters and underscores is valid",
			blockName:     "a_test_block",
			attributeName: validAttributeName,
			expectError:   false,
		},
		{
			name:          "block name with leading underscore is valid",
			blockName:     "_testBlock",
			attributeName: validAttributeName,
			expectError:   false,
		},
		{
			name:          "block name with trailing underscore is valid",
			blockName:     "testBlock_",
			attributeName: validAttributeName,
			expectError:   false,
		},
		{
			name:          "block name with letters and numbers is valid",
			blockName:     "testBlock123",
			attributeName: validAttributeName,
			expectError:   false,
		},
		{
			name:          "empty block name returns error",
			blockName:     "",
			attributeName: validAttributeName,
			expectError:   true,
			errorContains: "block name cannot be empty",
		},
		{
			name:          "all whitespace block name returns error",
			blockName:     " ",
			attributeName: validAttributeName,
			expectError:   true,
			errorContains: "invalid block name",
		},
		{
			name:          "block name with spaces returns error",
			blockName:     "test block",
			attributeName: validAttributeName,
			expectError:   true,
			errorContains: "invalid block name",
		},
		{
			name:          "block name starting with number returns error",
			blockName:     "123testBlock",
			attributeName: validAttributeName,
			expectError:   true,
			errorContains: "invalid block name",
		},
		{
			name:          "block name with special characters returns error",
			blockName:     "test-block",
			attributeName: validAttributeName,
			expectError:   true,
			errorContains: "invalid block name",
		},
		{
			name:          "block name with period returns error",
			blockName:     "test.block",
			attributeName: validAttributeName,
			expectError:   true,
			errorContains: "invalid block name",
		},

		{
			name:          "attribute name with mixed case letters is valid",
			blockName:     validBlockName,
			attributeName: "testAttribute",
			expectError:   false,
		},
		{
			name:          "attribute name with letters and underscores is valid",
			blockName:     validBlockName,
			attributeName: "a_test_attribute",
			expectError:   false,
		},
		{
			name:          "attribute name with leading underscore is valid",
			blockName:     validBlockName,
			attributeName: "_testAttribute",
			expectError:   false,
		},
		{
			name:          "attribute name with trailing underscore is valid",
			blockName:     validBlockName,
			attributeName: "testAttribute_",
			expectError:   false,
		},
		{
			name:          "attribute name with letters and numbers is valid",
			blockName:     validBlockName,
			attributeName: "testAttribute123",
			expectError:   false,
		},
		{
			name:          "empty attribute name returns error",
			blockName:     validBlockName,
			attributeName: "",
			expectError:   true,
			errorContains: "attribute name cannot be empty",
		},
		{
			name:          "all whitespace attribute name returns error",
			blockName:     validBlockName,
			attributeName: " ",
			expectError:   true,
			errorContains: "invalid attribute name",
		},
		{
			name:          "attribute name with spaces returns error",
			blockName:     validBlockName,
			attributeName: "test attribute",
			expectError:   true,
			errorContains: "invalid attribute name",
		},
		{
			name:          "attribute name starting with number returns error",
			blockName:     validBlockName,
			attributeName: "123testAttribute",
			expectError:   true,
			errorContains: "invalid attribute name",
		},
		{
			name:          "attribute name with special characters returns error",
			blockName:     validBlockName,
			attributeName: "test-attribute",
			expectError:   true,
			errorContains: "invalid attribute name",
		},
		{
			name:          "attribute name with period returns error",
			blockName:     validBlockName,
			attributeName: "test.attribute",
			expectError:   true,
			errorContains: "invalid attribute name",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			validator, err := NewBlockStringAttributeUniquenessValidator(tc.blockName, tc.attributeName)

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error for block name %q and attribute name %q, but got none", tc.blockName, tc.attributeName)
					return
				}
				if tc.errorContains != "" && !strings.Contains(err.Error(), tc.errorContains) {
					t.Errorf("Expected error to contain %q, but got: %s", tc.errorContains, err.Error())
				}
				if validator != nil {
					t.Errorf("Expected validator to be nil when error occurs, but got %T", validator)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for block name %q and attribute name %q, but got: %s", tc.blockName, tc.attributeName, err.Error())
					return
				}
				if validator == nil {
					t.Errorf("Expected validator to be non-nil for valid block name %q and attribute name %q", tc.blockName, tc.attributeName)
					return
				}
			}
		})
	}
}
