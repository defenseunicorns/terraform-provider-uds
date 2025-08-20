// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package validator provides custom validation functions for Terraform attributes.
package validator

import (
	"context"
	"fmt"
	"regexp"
	"strings"

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

func (v BlockStringAttributeUniquenessValidator) Description(_ context.Context) string {
	return fmt.Sprintf("Ensures that all %q blocks have unique values for the %q attribute", v.blockName, v.attributeName)
}

func (v BlockStringAttributeUniquenessValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

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

	seenValues := make(map[string]struct{})
	for _, element := range rawElements {
		elementMap := element.Attributes()
		attrVal, exists := elementMap[v.attributeName]
		if !exists {
			resp.Diagnostics.AddError(
				"Missing block attribute.",
				fmt.Sprintf("%s block does not have %q attribute.", v.blockName, v.attributeName),
			)
			continue
		}

		// The attribute check for IsUnknown is a safeguard in case the it is a computed value that Terraform/OpenTofu
		// could be unknown if it is computed by Terraform and realized later.
		if attrVal.IsUnknown() {
			continue
		}

		attrString, ok := attrVal.(types.String)
		if !ok {
			resp.Diagnostics.AddError(
				"Incorrect block attribute type.",
				fmt.Sprintf("Expected %s block %q attribute to be a string type, but got %T.", v.blockName, v.attributeName, attrVal),
			)
			continue
		}

		value := attrString.ValueString()
		if _, found := seenValues[value]; found {
			resp.Diagnostics.AddError(
				"Duplicate block attribute.",
				fmt.Sprintf("Multiple %s blocks found with %q set to value %q. %q attribute must be unique between %s blocks.", v.blockName, v.attributeName, value, v.attributeName, v.blockName),
			)
		}
		seenValues[value] = struct{}{}
	}
}

// NewBlockStringAttributeUniquenessValidator creates a new block string attribute uniqueness validator.
func NewBlockStringAttributeUniquenessValidator(blockName string, attributeName string) (validator.List, error) {
	if err := validateBlockName(blockName); err != nil {
		return nil, err
	}
	if err := validateAttributeName(attributeName); err != nil {
		return nil, err
	}
	return BlockStringAttributeUniquenessValidator{
		blockName:     blockName,
		attributeName: attributeName,
	}, nil
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

func (v packageSourceValidator) ValidateString(ctx context.Context, request validator.StringRequest, response *validator.StringResponse) {
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

func PackageSourceValidator() validator.String {
	return packageSourceValidator{}
}

func ValidateLocalFilePathPackageSource(value string) error {
	if !localPathRegex.MatchString(value) {
		return fmt.Errorf("%q is not a valid local file path", value)
	}
	return nil
}

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
