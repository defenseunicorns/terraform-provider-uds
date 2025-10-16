// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package validator

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	tfvalidator "github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
)

const (
	// Package source local file path test values
	localTarFilePathWithoutPath                 = "test-package.tar"
	localTarZstFilePathWithoutPath              = "test-package.tar.zst"
	localTarFilePathWithExplicitRelativePath    = "./path/to/test-package.tar"
	localTarZstFilePathWithExplicitRelativePath = "./path/to/test-package.tar.zst"
	localTarFilePathWithImplicitRelativePath    = "path/to/test-package.tar"
	localTarZstFilePathWithImplicitRelativePath = "path/to/test-package.tar.zst"
	localTarFilePathWithAbsoluteUnixPath        = "/absolute/path/to/test-package.tar"
	localTarZstFilePathWithAbsoluteUnixPath     = "/absolute/path/to/test-package.tar.zst"
	localTarFilePathWithWindowsDriveLetter      = "C:/path/to/test-package.tar"
	localTarZstFilePathWithWindowsDriveLetter   = "C:/path/to/test-package.tar.zst"
	localTarFilePathWithUnixHomePath            = "~/path/to/test-package.tar"
	localTarZstFilePathWithUnixHomePath         = "~/path/to/test-package.tar.zst"
	localFilePathWithUnderscoresAndDashes       = "some-dir/sub_dir/test-package-1.0.tar.zst"
	localFilePathWithPathSpecialCharacters      = "my_test-package-v1.0.tar"
	localFilePathWithoutTarOrTarZstExtension    = "test-package.zip"
	localFilePathWithoutExtension               = "test-package"
	localFilePathJustTarExtension               = ".tar"
	localFilePathJustTarZstExtension            = ".tar.zst"
	localFilePathWithInvalidCharacters          = "pack@ge.tar"
	localFilePathWithSpaces                     = "path with spaces/test-package.tar"
	localFilePathWithDoubleSlash                = "path//test-package.tar"
	localFilePathEndsWithSlash                  = "path/test-package.tar/"

	// Package source OCI reference test values
	ociReferenceWithVersionTag                = "oci://ghcr.io/defenseunicornstest/packages/test-package:v1.0.0"
	ociReferenceWithLatestTag                 = "oci://ghcr.io/defenseunicornstest/packages/test-package:latest"
	ociReferenceWithDigest                    = "oci://ghcr.io/defenseunicornstest/packages/test-package@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	ociReferenceWithoutTagOrDigest            = "oci://ghcr.io/defenseunicornstest/packages/test-package"
	ociReferenceWithPort                      = "oci://registry.example.com:5000/namespace/test-package:v1.0.0"
	ociReferenceWithLocalhost                 = "oci://localhost:5000/test-package:latest"
	ociReferenceWithNestedNamespace           = "oci://ghcr.io/defenseunicornstest/team/project/test-package:v1.0.0"
	ociReferenceWithoutScheme                 = "ghcr.io/defenseunicornstest/packages/test-package:v1.0.0"
	ociReferenceWithRepositoryOnly            = "ghcr.io/defenseunicornstest/packages/test-package:v1.0.0"
	ociReferenceWithInvalidRepoNameCharacters = "oci://ghcr.io/INVALID/test-package:v1.0.0"
	ociReferenceWithInvalidTagFormat          = "oci://ghcr.io/defenseunicornstest/packages/test-package:invalid tag with spaces"
	ociReferenceWithOnlyScheme                = "oci://"
	ociReferenceWithOnlyHostname              = "oci://ghcr.io"
	ociReferenceWithInvalidDigest             = "oci://ghcr.io/defenseunicornstest/packages/test-package@invalid-digest"
	ociReferenceWithDoubleSlash               = "oci://ghcr.io//defenseunicornstest/packages/test-package:v1.0.0"
	ociReferenceWithTrailingSlash             = "oci://ghcr.io/defenseunicornstest/packages/test-package:v1.0.0/"

	// Package source misc test values
	httpPackageReference  = "http://defenseunicornstest.com/test-package"
	httpsPackageReference = "https://defenseunicornstest.com/test-package"
)

func newObjectValues(attributeName string, values []string) []attr.Value {
	attrValues := make([]attr.Value, len(values))
	for i, value := range values {
		attrValues[i] = types.ObjectValueMust(
			map[string]attr.Type{attributeName: types.StringType},
			map[string]attr.Value{attributeName: types.StringValue(value)},
		)
	}
	return attrValues
}

func newObjectListWithAttributeValues(attributeName string, values []string) types.List {
	attrValues := newObjectValues(attributeName, values)
	return types.ListValueMust(
		types.ObjectType{AttrTypes: map[string]attr.Type{attributeName: types.StringType}},
		attrValues,
	)
}

func newObjectSetWithAttributeValues(attributeName string, values []string) types.Set {
	attrValues := newObjectValues(attributeName, values)
	return types.SetValueMust(
		types.ObjectType{AttrTypes: map[string]attr.Type{attributeName: types.StringType}},
		attrValues,
	)
}

func assertValidatorResponseDiagnosticsContainsErrorSummary(t *testing.T, diagnostics diag.Diagnostics, expectedErrorSummary string) {
	// Check that at least one error has the expected summary
	found := false
	for _, d := range diagnostics {
		if d.Summary() == expectedErrorSummary {
			found = true
			break
		}
	}
	assert.True(t, found, "Expected error with summary %q, but it was not found in diagnostics: %v", expectedErrorSummary, diagnostics)
}

//nolint:dupl // Test structure is similar to ValidateSet test, but tests List type specifically
func TestBlockStringAttributeUniquenessValidator_ValidateList(t *testing.T) {
	t.Parallel()

	blockName := "test_block"
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

	validator, _ := NewBlockStringAttributeUniquenessValidator(blockName, attributeName)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := tfvalidator.ListRequest{
				ConfigValue: tc.configValue,
			}
			resp := &tfvalidator.ListResponse{
				Diagnostics: diag.Diagnostics{},
			}

			validator.ValidateList(context.Background(), req, resp)
			assert.Equal(t, tc.expectedErrorCount, len(resp.Diagnostics), "Expected %d errors, got %d. Diagnostics: %v", tc.expectedErrorCount, len(resp.Diagnostics), resp.Diagnostics)

			if tc.expectedErrorSummary != "" && len(resp.Diagnostics) > 0 {
				assertValidatorResponseDiagnosticsContainsErrorSummary(t, resp.Diagnostics, tc.expectedErrorSummary)
			}
		})
	}
}

//nolint:dupl // Test structure is similar to ValidateList test, but tests Set type specifically
func TestBlockStringAttributeUniquenessValidator_ValidateSet(t *testing.T) {
	t.Parallel()

	blockName := "test_block"
	attributeName := "test_attr"
	duplicateAttributeErrorSummary := "Duplicate block attribute."
	incorrectAttributeTypeErrorSummary := "Incorrect block attribute type."
	missingAttributeErrorSummary := "Missing block attribute."
	tests := []struct {
		name                 string
		configValue          types.Set
		expectedErrorCount   int
		expectedErrorSummary string
	}{
		{
			name:               "null list is valid",
			configValue:        types.SetNull(types.ObjectType{AttrTypes: map[string]attr.Type{attributeName: types.StringType}}),
			expectedErrorCount: 0,
		},
		{
			name:               "unknown list is valid",
			configValue:        types.SetUnknown(types.ObjectType{AttrTypes: map[string]attr.Type{attributeName: types.StringType}}),
			expectedErrorCount: 0,
		},
		{
			name:               "empty list is valid",
			configValue:        types.SetValueMust(types.ObjectType{AttrTypes: map[string]attr.Type{attributeName: types.StringType}}, []attr.Value{}),
			expectedErrorCount: 0,
		},
		{
			name:               "single item is valid",
			configValue:        newObjectSetWithAttributeValues(attributeName, []string{"value1"}),
			expectedErrorCount: 0,
		},
		{
			name:               "multiple unique items is valid",
			configValue:        newObjectSetWithAttributeValues(attributeName, []string{"value1", "value2", "value3"}),
			expectedErrorCount: 0,
		},
		{
			name:                 "duplicate items is not valid",
			configValue:          newObjectSetWithAttributeValues(attributeName, []string{"value1", "value2", "value1"}),
			expectedErrorCount:   1,
			expectedErrorSummary: duplicateAttributeErrorSummary,
		},
		{
			name:                 "multiple duplicates is not valid with multiple errors",
			configValue:          newObjectSetWithAttributeValues(attributeName, []string{"value1", "value1", "value2", "value2"}),
			expectedErrorCount:   2,
			expectedErrorSummary: duplicateAttributeErrorSummary,
		},
		{
			name:                 "missing attribute is not valid",
			configValue:          newObjectSetWithAttributeValues("other_attr", []string{"value1", "value2"}),
			expectedErrorCount:   1,
			expectedErrorSummary: missingAttributeErrorSummary,
		},
		{
			name:               "unknown attribute value should be skipped",
			configValue:        newObjectSetWithAttributeValues(attributeName, []string{"value1", " ", "value2"}),
			expectedErrorCount: 0,
		},
		{
			name: "wrong attribute type is not valid",
			configValue: types.SetValueMust(
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

	validator, _ := NewBlockStringAttributeUniquenessValidator(blockName, attributeName)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := tfvalidator.SetRequest{
				ConfigValue: tc.configValue,
			}
			resp := &tfvalidator.SetResponse{
				Diagnostics: diag.Diagnostics{},
			}

			validator.ValidateSet(context.Background(), req, resp)
			assert.Equal(t, tc.expectedErrorCount, len(resp.Diagnostics), "Expected %d errors, got %d. Diagnostics: %v", tc.expectedErrorCount, len(resp.Diagnostics), resp.Diagnostics)

			if tc.expectedErrorSummary != "" && len(resp.Diagnostics) > 0 {
				assertValidatorResponseDiagnosticsContainsErrorSummary(t, resp.Diagnostics, tc.expectedErrorSummary)
			}
		})
	}
}

func TestNewBlockStringAttributeUniquenessValidator(t *testing.T) {
	t.Parallel()

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
			t.Parallel()
			validator, err := NewBlockStringAttributeUniquenessValidator(tc.blockName, tc.attributeName)

			if tc.expectError {
				assert.Nil(t, validator, "Expected validator to be nil when error occurs, but got %T", validator)
				assert.NotNil(t, err, "Expected error for block name %q and attribute name %q, but got none", tc.blockName, tc.attributeName)
				assert.Contains(t, err.Error(), tc.errorContains, "Expected error to contain %q", tc.errorContains)
			} else {
				assert.NotNil(t, validator, "Expected validator to be non-nil for valid block name %q and attribute name %q", tc.blockName, tc.attributeName)
				assert.Nil(t, err, "Expected no error for block name %q and attribute name %q, but got error: %v", tc.blockName, tc.attributeName, err)
			}
		})
	}
}

func TestPackageSourceValidator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		value       string
		expectError bool
	}{
		{
			name:        "OCI reference with oci:// scheme is valid",
			value:       ociReferenceWithVersionTag,
			expectError: false,
		},
		{
			name:        "OCI reference with latest tag is valid",
			value:       ociReferenceWithLatestTag,
			expectError: false,
		},
		{
			name:        "OCI reference with digest is valid",
			value:       ociReferenceWithDigest,
			expectError: false,
		},
		{
			name:        "OCI reference with port is valid",
			value:       ociReferenceWithPort,
			expectError: false,
		},
		{
			name:        "OCI reference without oci:// scheme is invalid",
			value:       ociReferenceWithoutScheme,
			expectError: true,
		},
		{
			name:        "empty string is invalid",
			value:       "",
			expectError: true,
		},
		{
			name:        "all whitespace is invalid",
			value:       "  ",
			expectError: true,
		},
		{
			name:        "OCI reference without repository only is invalid",
			value:       ociReferenceWithRepositoryOnly,
			expectError: true,
		},
		{
			name:        "OCI reference with invalid characters in repository name is invalid",
			value:       ociReferenceWithInvalidRepoNameCharacters,
			expectError: true,
		},
		{
			name:        "OCI reference with invalid tag format is invalid",
			value:       ociReferenceWithInvalidTagFormat,
			expectError: true,
		},
		{
			name:        "HTTP reference is invalid",
			value:       httpPackageReference,
			expectError: true,
		},
	}

	expectedErrorContains := "Invalid Package Source"
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := validator.StringRequest{
				Path:           path.Root("test"),
				PathExpression: path.MatchRoot("test"),
				ConfigValue:    types.StringValue(tc.value),
			}
			response := &validator.StringResponse{}

			PackageSourceValidator().ValidateString(context.Background(), request, response)

			if tc.expectError {
				assert.True(t, response.Diagnostics.HasError(), "Expected error but got none")
				assertValidatorResponseDiagnosticsContainsErrorSummary(t, response.Diagnostics, expectedErrorContains)
			} else {
				assert.False(t, response.Diagnostics.HasError(), "Expected no error but got: %v", response.Diagnostics.Errors())
			}
		})
	}
}

func TestPackageSourceValidator_NullAndUnknown(t *testing.T) {
	t.Parallel()

	validator := PackageSourceValidator()

	for _, configValue := range []types.String{types.StringNull(), types.StringUnknown()} {
		request := tfvalidator.StringRequest{
			Path:           path.Root("test"),
			PathExpression: path.MatchRoot("test"),
			ConfigValue:    configValue,
		}
		response := &tfvalidator.StringResponse{}

		validator.ValidateString(context.Background(), request, response)

		assert.False(t, response.Diagnostics.HasError(), "Expected no error for null value but got: %v", response.Diagnostics.Errors())
	}
}

func TestValidateLocalFilePathPackageSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		value       string
		expectError bool
	}{
		// Valid cases
		{
			name:        "tar file without path is valid",
			value:       localTarFilePathWithoutPath,
			expectError: false,
		},
		{
			name:        "tar.zst file without path is valid",
			value:       localTarZstFilePathWithoutPath,
			expectError: false,
		},
		{
			name:        "tar file with explicit relative path is valid",
			value:       localTarFilePathWithExplicitRelativePath,
			expectError: false,
		},
		{
			name:        "tar.zst file with implicit relative path is valid",
			value:       localTarZstFilePathWithImplicitRelativePath,
			expectError: false,
		},
		{
			name:        "tar file with implicit relative path is valid",
			value:       localTarFilePathWithImplicitRelativePath,
			expectError: false,
		},
		{
			name:        "tar.zst file with explicit relative path is valid",
			value:       localTarZstFilePathWithExplicitRelativePath,
			expectError: false,
		},
		{
			name:        "tar file with absolute unix path is valid",
			value:       localTarFilePathWithAbsoluteUnixPath,
			expectError: false,
		},
		{
			name:        "tar.zst file with absolute unix path is valid",
			value:       localTarZstFilePathWithAbsoluteUnixPath,
			expectError: false,
		},
		{
			name:        "tar file with windows drive letter is valid",
			value:       localTarFilePathWithWindowsDriveLetter,
			expectError: false,
		},
		{
			name:        "tar.zst file with windows drive letter is valid",
			value:       localTarZstFilePathWithWindowsDriveLetter,
			expectError: false,
		},
		{
			name:        "file with underscores and dashes is valid",
			value:       localFilePathWithUnderscoresAndDashes,
			expectError: false,
		},
		{
			name:        "nested path with special chars is valid",
			value:       localFilePathWithPathSpecialCharacters,
			expectError: false,
		},
		// Invalid cases
		{
			name:        "tar file with unix home path is invalid",
			value:       localTarFilePathWithUnixHomePath,
			expectError: true,
		},
		{
			name:        "tar.zst file with unix home path is invalid",
			value:       localTarZstFilePathWithUnixHomePath,
			expectError: true,
		},
		{
			name:        "missing tar or tar.zst extension is invalid",
			value:       localFilePathWithoutTarOrTarZstExtension,
			expectError: true,
		},
		{
			name:        "no extension is invalid",
			value:       localFilePathWithoutExtension,
			expectError: true,
		},
		{
			name:        "empty string is invalid",
			value:       "",
			expectError: true,
		},
		{
			name:        "just tar extension is invalid",
			value:       localFilePathJustTarExtension,
			expectError: true,
		},
		{
			name:        "just tar.zst extension is invalid",
			value:       localFilePathJustTarZstExtension,
			expectError: true,
		},
		{
			name:        "invalid characters are invalid",
			value:       localFilePathWithInvalidCharacters,
			expectError: true,
		},
		{
			name:        "spaces in path is invalid",
			value:       localFilePathWithSpaces,
			expectError: true,
		},
		{
			name:        "double slash is invalid",
			value:       localFilePathWithDoubleSlash,
			expectError: true,
		},
		{
			name:        "ends with slash is invalid",
			value:       localFilePathEndsWithSlash,
			expectError: true,
		},
		{
			name:        "tar but not at end is invalid",
			value:       "package.tar.something",
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateLocalFilePathPackageSource(tc.value)

			if tc.expectError {
				assert.NotNil(t, err, "Expected error for value %q, but got none", tc.value)
			} else {
				assert.Nil(t, err, "Expected no error for value %q, but got: %v", tc.value, err)
			}
		})
	}
}

func TestValidateOCIReferencePackageSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		value       string
		expectError bool
	}{
		// Valid cases
		{
			name:        "value with version tag is valid",
			value:       ociReferenceWithVersionTag,
			expectError: false,
		},
		{
			name:        "OCI reference with latest tag is valid",
			value:       ociReferenceWithLatestTag,
			expectError: false,
		},
		{
			name:        "OCI reference with digest is valid",
			value:       ociReferenceWithDigest,
			expectError: false,
		},
		{
			name:        "OCI reference without tag or digest is valid",
			value:       ociReferenceWithoutTagOrDigest,
			expectError: false,
		},
		{
			name:        "OCI reference with port is valid",
			value:       ociReferenceWithPort,
			expectError: false,
		},
		{
			name:        "OCI reference with localhost is valid",
			value:       ociReferenceWithLocalhost,
			expectError: false,
		},
		{
			name:        "OCI reference with nested namespace is valid",
			value:       ociReferenceWithNestedNamespace,
			expectError: false,
		},
		// Invalid cases
		{
			name:        "OCI reference without scheme is invalid",
			value:       ociReferenceWithoutScheme,
			expectError: true,
		},
		{
			name:        "OCI reference with empty string is invalid",
			value:       "",
			expectError: true,
		},
		{
			name:        "OCI reference with only oci:// scheme is invalid",
			value:       ociReferenceWithOnlyScheme,
			expectError: true,
		},
		{
			name:        "OCI reference with only hostname is invalid",
			value:       ociReferenceWithOnlyHostname,
			expectError: true,
		},
		{
			name:        "OCI reference with invalid characters in repository name is invalid",
			value:       ociReferenceWithInvalidRepoNameCharacters,
			expectError: true,
		},
		{
			name:        "OCI reference with invalid tag with spaces is invalid",
			value:       ociReferenceWithInvalidTagFormat,
			expectError: true,
		},
		{
			name:        "OCI reference with invalid digest format is invalid",
			value:       ociReferenceWithInvalidDigest,
			expectError: true,
		},
		{
			name:        "OCI reference with double slash in path is invalid",
			value:       ociReferenceWithDoubleSlash,
			expectError: true,
		},
		{
			name:        "OCI reference with trailing slash is invalid",
			value:       ociReferenceWithTrailingSlash,
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateOCIReferencePackageSource(tc.value)

			if tc.expectError {
				assert.NotNil(t, err, "Expected error for value %q, but got none", tc.value)
			} else {
				assert.Nil(t, err, "Expected no error for value %q, but got: %v", tc.value, err)
			}
		})
	}
}

func TestDurationGreaterThanValidator(t *testing.T) {
	t.Parallel()

	minDuration := time.Hour // 1h
	invalidDurationErrorSummary := "Invalid Duration"
	durationTooShortErrorSummary := "Duration Too Short"

	tests := []struct {
		name                 string
		duration             string
		hasError             bool
		expectedErrorSummary string
	}{
		{
			name:     "duration value greater than minimum is valid",
			duration: (minDuration + time.Nanosecond).String(),
			hasError: false,
		},
		{
			name:                 "duration value equal to minimum is not valid",
			duration:             minDuration.String(),
			hasError:             true,
			expectedErrorSummary: durationTooShortErrorSummary,
		},
		{
			name:                 "duration less than minimum is not valid",
			duration:             (minDuration - time.Nanosecond).String(),
			hasError:             true,
			expectedErrorSummary: durationTooShortErrorSummary,
		},
		{
			name:                 "duration value missing unit is not valid",
			duration:             "30",
			hasError:             true,
			expectedErrorSummary: invalidDurationErrorSummary,
		},
		{
			name:                 "duration value with unknown unit is not valid",
			duration:             "30x",
			hasError:             true,
			expectedErrorSummary: invalidDurationErrorSummary,
		},
		{
			name:                 "empty duration value is not valid",
			duration:             "",
			hasError:             true,
			expectedErrorSummary: invalidDurationErrorSummary,
		},
		{
			name:                 "non-numeric duration value is not valid",
			duration:             "thirty minutes",
			hasError:             true,
			expectedErrorSummary: invalidDurationErrorSummary,
		},
	}

	v := DurationGreaterThanValidator(minDuration)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := validator.StringRequest{
				ConfigValue: types.StringValue(tc.duration),
			}
			resp := &validator.StringResponse{
				Diagnostics: diag.Diagnostics{},
			}

			v.ValidateString(context.Background(), req, resp)
			if tc.hasError {
				assert.True(t, resp.Diagnostics.HasError(), "Expected error for duration %q, but got none", tc.duration)
				assertValidatorResponseDiagnosticsContainsErrorSummary(t, resp.Diagnostics, tc.expectedErrorSummary)
			} else {
				assert.False(t, resp.Diagnostics.HasError(), "Expected no error for duration %q, but got: %v", tc.duration, resp.Diagnostics)
			}
		})
	}
}

func TestDurationGreaterThanValidator_NullAndUnknown(t *testing.T) {
	t.Parallel()

	minDuration := time.Hour
	v := DurationGreaterThanValidator(minDuration)

	tests := []struct {
		name  string
		value types.String
	}{
		{
			name:  "null value is valid",
			value: types.StringNull(),
		},
		{
			name:  "unknown value is valid",
			value: types.StringUnknown(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := validator.StringRequest{
				Path:           path.Root("test"),
				PathExpression: path.MatchRoot("test"),
				ConfigValue:    tc.value,
			}
			resp := &validator.StringResponse{}

			v.ValidateString(context.Background(), req, resp)
			assert.False(t, resp.Diagnostics.HasError(), "Expected no error for %s value, but got: %v", tc.name, resp.Diagnostics)
		})
	}
}
