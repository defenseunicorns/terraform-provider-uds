// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package validator provides custom validation functions for Terraform attributes.
package validator

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"oras.land/oras-go/v2/registry"
)

var localPathRegex *regexp.Regexp = regexp.MustCompile(`^(?:[a-zA-Z]:)?\/?(?:[a-zA-Z0-9._-]+\/)*[a-zA-Z0-9._-]+\.tar(?:\.zst)?$`)

// BlockStringAttributeUniquenessValidator validates that string attributes within blocks are unique.
type BlockStringAttributeUniquenessValidator struct {
	blockName     string
	attributeName string
}

// NewBlockStringAttributeUniquenessValidator creates a new block string attribute uniqueness validator.
func NewBlockStringAttributeUniquenessValidator(blockName string, attributeName string) (*BlockStringAttributeUniquenessValidator, error) {
	if err := validateBlockName(blockName); err != nil {
		return nil, err
	}
	if err := validateAttributeName(attributeName); err != nil {
		return nil, err
	}
	return &BlockStringAttributeUniquenessValidator{
		blockName:     blockName,
		attributeName: attributeName,
	}, nil
}

// Description returns a plain text description of the validator.
func (v BlockStringAttributeUniquenessValidator) Description(_ context.Context) string {
	return fmt.Sprintf("Ensures that all %q blocks have unique values for the %q attribute", v.blockName, v.attributeName)
}

// MarkdownDescription returns the markdown description for the validator.
func (v BlockStringAttributeUniquenessValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateList validates that all blocks in the list have unique values for the specified attribute.
func (v BlockStringAttributeUniquenessValidator) ValidateList(ctx context.Context, req validator.ListRequest, resp *validator.ListResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	var rawElements []types.Object
	diags := req.ConfigValue.ElementsAs(ctx, &rawElements, false)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	v.validateElements(rawElements, &resp.Diagnostics)
}

// ValidateSet validates that all blocks in the set have unique values for the specified attribute.
func (v BlockStringAttributeUniquenessValidator) ValidateSet(ctx context.Context, req validator.SetRequest, resp *validator.SetResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	var rawElements []types.Object
	diags := req.ConfigValue.ElementsAs(ctx, &rawElements, false)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	v.validateElements(rawElements, &resp.Diagnostics)
}

// validateElements contains the common validation logic for both List and Set validators
func (v BlockStringAttributeUniquenessValidator) validateElements(rawElements []types.Object, diagnostics *diag.Diagnostics) {
	seenValues := make(map[string]struct{})
	for _, element := range rawElements {
		elementMap := element.Attributes()
		attrVal, exists := elementMap[v.attributeName]
		if !exists {
			diagnostics.AddError(
				"Missing block attribute.",
				fmt.Sprintf("%s block does not have %q attribute.", v.blockName, v.attributeName),
			)
			continue
		}

		// The attribute check for IsUnknown is a safeguard in case it is a computed value
		if attrVal.IsUnknown() {
			continue
		}

		attrString, ok := attrVal.(types.String)
		if !ok {
			diagnostics.AddError(
				"Incorrect block attribute type.",
				fmt.Sprintf("Expected %s block %q attribute to be a string type, but got %T.", v.blockName, v.attributeName, attrVal),
			)
			continue
		}

		value := attrString.ValueString()
		if _, found := seenValues[value]; found {
			diagnostics.AddError(
				"Duplicate block attribute.",
				fmt.Sprintf("Multiple %s blocks found with %q set to value %q. %q attribute must be unique between %s blocks.", v.blockName, v.attributeName, value, v.attributeName, v.blockName),
			)
		}
		seenValues[value] = struct{}{}
	}
}

func validateBlockName(name string) error {
	return validateName(name, "block")
}

func validateAttributeName(name string) error {
	return validateName(name, "attribute")
}

// validateName validates that a name follows Go identifier rules
func validateName(name, nameType string) error {
	if name == "" {
		return fmt.Errorf("%s name cannot be empty", nameType)
	}

	validName := regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	if !validName.MatchString(name) {
		return fmt.Errorf("invalid %s name %q: must start with a letter or underscore, followed by letters, digits, or underscores", nameType, name)
	}

	return nil
}

type packageSourceValidator struct{}

func (v packageSourceValidator) Description(_ context.Context) string {
	return "value must be a valid OCI distribution reference (including oci:// scheme) or local file path (absolute or relative)"
}

func (v packageSourceValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v packageSourceValidator) ValidateString(_ context.Context, request validator.StringRequest, response *validator.StringResponse) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}

	value := request.ConfigValue.ValueString()
	if ValidateLocalFilePathPackageSource(value) == nil {
		return
	}
	if ValidateOCIReferencePackageSource(value) == nil {
		return
	}
	response.Diagnostics.AddAttributeError(
		request.Path,
		"Invalid Package Source",
		fmt.Sprintf("The provided value %q must be a valid OCI distribution reference (including oci:// scheme) or local file path (absolute or relative).", value),
	)
}

// PackageSourceValidator creates a new package source validator.
func PackageSourceValidator() validator.String {
	return packageSourceValidator{}
}

// ValidateLocalFilePathPackageSource validates that a value is a valid package source local file path.
func ValidateLocalFilePathPackageSource(value string) error {
	if !localPathRegex.MatchString(value) {
		return fmt.Errorf("%q is not a valid local file path", value)
	}
	return nil
}

// ValidateOCIReferencePackageSource validates that a value is a valid package source OCI reference.
func ValidateOCIReferencePackageSource(value string) error {
	if !strings.HasPrefix(value, "oci://") {
		return fmt.Errorf("%q missing oci:// scheme", value)
	}
	ref, err := registry.ParseReference(strings.TrimPrefix(value, "oci://"))
	if err != nil {
		return err
	}
	err = ref.Validate()
	if err != nil {
		return err
	}
	return nil
}

// DurationGreaterThanValidator creates a validator that ensures a duration is greater than a minimum value.
func DurationGreaterThanValidator(minDuration time.Duration) validator.String {
	return durationGreaterThanValidator{
		minDuration: minDuration,
	}
}

// durationGreaterThanValidator validates that a duration is greater than a minimum value.
type durationGreaterThanValidator struct {
	minDuration time.Duration
}

// Description returns a plain text description of the validator.
func (v durationGreaterThanValidator) Description(_ context.Context) string {
	return fmt.Sprintf("value must be a valid duration greater than %s", v.minDuration)
}

// MarkdownDescription returns the markdown description for the validator.
func (v durationGreaterThanValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateString validates that the duration is at least the minimum value.
func (v durationGreaterThanValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	value := req.ConfigValue.ValueString()

	duration, err := time.ParseDuration(value)
	if err != nil {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Duration",
			fmt.Sprintf("The %s value %q is not a valid duration. Valid examples: \"1h\", \"30m\", \"1h30m\", \"90s\". Error: %s", req.Path.String(), value, err.Error()),
		)
		return
	}

	if duration <= v.minDuration {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Duration Too Short",
			fmt.Sprintf("The %s duration value %q must be greater than %s.", req.Path.String(), value, v.minDuration),
		)
	}
}
