// Copyright 2024-2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package provider

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	zarfLayout "github.com/zarf-dev/zarf/src/pkg/packager/layout"
	zarfValue "github.com/zarf-dev/zarf/src/pkg/value"
)

func deepMergePackageValues(values ...zarfValue.Values) zarfValue.Values {
	merged := zarfValue.Values{}
	for _, value := range values {
		merged.DeepMerge(value)
	}
	return merged
}

// mergePartialPackageValues combines plan-time values while preserving known
// structure when an unknown map/object is represented by nil. Exact conflicts
// are checked separately from the original Terraform values before this merge.
func mergePartialPackageValues(values ...zarfValue.Values) zarfValue.Values {
	merged := zarfValue.Values{}
	for _, value := range values {
		mergePartialPackageValueMaps(merged, value)
	}
	return merged
}

func mergePartialPackageValueMaps(destination, source map[string]any) {
	for key, sourceValue := range source {
		destinationValue, exists := destination[key]
		if !exists {
			destination[key] = sourceValue
			continue
		}

		sourceMap, sourceIsMap := sourceValue.(map[string]any)
		destinationMap, destinationIsMap := destinationValue.(map[string]any)
		if sourceIsMap && destinationIsMap {
			mergePartialPackageValueMaps(destinationMap, sourceMap)
			continue
		}

		// A nil value represents an unknown plan-time subtree. Preserve any
		// known value already present so its schema constraints remain visible.
		if sourceValue == nil {
			continue
		}
		destination[key] = sourceValue
	}
}

func joinValuePath(prefix string, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

// dynamicToPartialValues converts the known portion of a Terraform dynamic
// value for plan-time schema validation. Unknown values are represented as
// nil so their containing keys remain available for structural schema checks.
// The returned paths identify values whose constraints must be deferred until
// apply because their eventual values are not known during planning.
func dynamicToPartialValues(attrName string, value types.Dynamic) (zarfValue.Values, []string, error) {
	if value.IsNull() || value.IsUnderlyingValueNull() {
		return zarfValue.Values{}, nil, nil
	}
	if value.IsUnknown() || value.IsUnderlyingValueUnknown() {
		return zarfValue.Values{}, []string{""}, nil
	}

	converted, unknownPaths, err := terraformValueToPartialGoValue(value.UnderlyingValue(), "")
	if err != nil {
		return zarfValue.Values{}, nil, fmt.Errorf("failed to convert %s: %w", attrName, err)
	}

	values, ok := converted.(map[string]any)
	if !ok {
		return zarfValue.Values{}, nil, fmt.Errorf("%s must be a map or object", attrName)
	}

	return zarfValue.Values(values), unknownPaths, nil
}

// terraformValueToPartialGoValue is the plan-time counterpart to
// terraformValueToGoValue. It preserves map/object keys even when their
// values are unknown, allowing additionalProperties and other structural
// checks to run without pretending to know the values themselves.
func terraformValueToPartialGoValue(value attr.Value, valuePath string) (any, []string, error) {
	if value == nil || value.IsNull() {
		return nil, nil, nil
	}
	if value.IsUnknown() {
		return nil, []string{valuePath}, nil
	}

	switch v := value.(type) {
	case types.String:
		return v.ValueString(), nil, nil
	case types.Bool:
		return v.ValueBool(), nil, nil
	case types.Number:
		converted, err := numberToGoValue(v.ValueBigFloat())
		return converted, nil, err
	case types.Map:
		return terraformMapToPartialGoMap(v.Elements(), valuePath)
	case types.Object:
		return terraformMapToPartialGoMap(v.Attributes(), valuePath)
	case types.List:
		return terraformCollectionToPartialGoSlice(v.Elements(), valuePath)
	case types.Set:
		return terraformCollectionToPartialGoSlice(v.Elements(), valuePath)
	case types.Tuple:
		return terraformCollectionToPartialGoSlice(v.Elements(), valuePath)
	case types.Dynamic:
		if v.IsUnknown() || v.IsUnderlyingValueUnknown() {
			return nil, []string{valuePath}, nil
		}
		if v.IsNull() || v.IsUnderlyingValueNull() {
			return nil, nil, nil
		}
		return terraformValueToPartialGoValue(v.UnderlyingValue(), valuePath)
	default:
		return nil, nil, fmt.Errorf("unsupported value type %T", value)
	}
}

func terraformMapToPartialGoMap(elements map[string]attr.Value, valuePath string) (map[string]any, []string, error) {
	result := make(map[string]any, len(elements))
	var unknownPaths []string
	for key, value := range elements {
		keyPath := joinValuePath(valuePath, key)
		converted, paths, err := terraformValueToPartialGoValue(value, keyPath)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", key, err)
		}
		result[key] = converted
		unknownPaths = append(unknownPaths, paths...)
	}
	return result, unknownPaths, nil
}

func terraformCollectionToPartialGoSlice(elements []attr.Value, valuePath string) ([]any, []string, error) {
	result := make([]any, 0, len(elements))
	var unknownPaths []string
	for idx, value := range elements {
		converted, paths, err := terraformValueToPartialGoValue(value, joinValuePath(valuePath, fmt.Sprintf("%d", idx)))
		if err != nil {
			return nil, nil, fmt.Errorf("%d: %w", idx, err)
		}
		result = append(result, converted)
		unknownPaths = append(unknownPaths, paths...)
	}
	return result, unknownPaths, nil
}

func validatePackageValuesAgainstSchema(ctx context.Context, values zarfValue.Values, schemaPath string) error {
	return values.Validate(ctx, schemaPath, zarfValue.ValidateOptions{})
}

// validatePartialPackageValuesAgainstSchema validates the known portion of a
// package values document. Unknown values are retained as nil during schema
// validation so their keys can still be checked. Only errors attached directly
// to an unknown value are deferred; container and key-structure errors remain
// enforceable at plan time.
func validatePartialPackageValuesAgainstSchema(ctx context.Context, values zarfValue.Values, schemaPath string, unknownPaths []string) error {
	err := values.Validate(ctx, schemaPath, zarfValue.ValidateOptions{})
	if err == nil || len(unknownPaths) == 0 {
		return err
	}

	var schemaErr *zarfValue.SchemaValidationError
	if !errors.As(err, &schemaErr) {
		return err
	}

	filteredErrors := schemaErr.Errors[:0:0]
	for _, validationErr := range schemaErr.Errors {
		property, _ := validationErr.Details()["property"].(string)
		if shouldDeferPartialSchemaError(validationErr.Type(), validationErr.Field(), property, unknownPaths) {
			continue
		}
		filteredErrors = append(filteredErrors, validationErr)
	}

	if len(filteredErrors) == 0 {
		return nil
	}

	filteredSchemaErr := *schemaErr
	filteredSchemaErr.Errors = filteredErrors
	return &filteredSchemaErr
}

func shouldDeferPartialSchemaError(errorType, field, property string, unknownPaths []string) bool {
	// These errors describe known keys or container shape, not an unknown value.
	switch errorType {
	case "additional_property_not_allowed", "invalid_property_name", "invalid_property_pattern":
		return false
	}

	if errorType == "required" {
		missingPath := joinValuePath(field, property)
		for _, unknownPath := range unknownPaths {
			if isValuePathAncestorOrEqual(unknownPath, missingPath) {
				return true
			}
		}
		return false
	}

	return slices.Contains(unknownPaths, field)
}

func isValuePathAncestorOrEqual(ancestor, descendant string) bool {
	return ancestor == "" || ancestor == descendant || strings.HasPrefix(descendant, ancestor+".")
}

func (r *PackageResource) validatePlannedPackageValuesAgainstSchema(ctx context.Context, plan PackageResourceModel, pkgLayout *zarfLayout.PackageLayout) error {
	if !pkgLayout.HasValuesSchema() {
		return nil
	}

	values, valuesUnknownPaths, err := dynamicToPartialValues("values", plan.Values)
	if err != nil {
		return err
	}
	sensitiveValues, sensitiveUnknownPaths, err := dynamicToPartialValues("sensitive_values", plan.SensitiveValues)
	if err != nil {
		return err
	}
	overrides := mergePartialPackageValues(values, sensitiveValues)

	defaults, err := zarfValue.ParseLocalFile(ctx, filepath.Join(pkgLayout.DirPath(), layout.ValuesYAML))
	if err != nil {
		return fmt.Errorf("failed to parse package values: %w", err)
	}
	defaults = deepMergePackageValues(defaults, overrides)

	schemaPath := filepath.Join(pkgLayout.DirPath(), layout.ValuesSchema)
	unknownPaths := slices.Concat(valuesUnknownPaths, sensitiveUnknownPaths)
	if err := validatePartialPackageValuesAgainstSchema(ctx, defaults, schemaPath, unknownPaths); err != nil {
		return fmt.Errorf("package values do not match the package values schema: %w", err)
	}
	return nil
}

func hasConfiguredPackageValues(model PackageResourceModel) bool {
	return isDynamicAttributeConfigured(model.Values) || isDynamicAttributeConfigured(model.SensitiveValues)
}
