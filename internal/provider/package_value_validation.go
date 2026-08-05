// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package provider

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
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
// structure when an unknown map or object is represented by nil.
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
// value for plan-time validation. Unknown values are represented by nil, while
// unknownPaths records the affected subtrees for conservative error filtering.
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
		converted, paths, err := terraformValueToPartialGoValue(value, joinValuePath(valuePath, key))
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

// validatePartialPackageValuesAgainstSchema validates a partially known values
// document. It returns only errors that can be proven from known data; errors
// touching an unknown subtree defer until apply.
func validatePartialPackageValuesAgainstSchema(ctx context.Context, values zarfValue.Values, schemaPath string, unknownPaths []string) error {
	err := values.Validate(ctx, schemaPath, zarfValue.ValidateOptions{})
	if err == nil || len(unknownPaths) == 0 {
		return err
	}

	var schemaErr *zarfValue.SchemaValidationError
	if !errors.As(err, &schemaErr) {
		return err
	}

	deferredAggregatePaths := partialSchemaDeferredAggregatePaths(schemaErr, unknownPaths)
	filteredErrors := schemaErr.Errors[:0:0]
	for _, validationErr := range schemaErr.Errors {
		if partialSchemaErrorIsInDeferredAggregate(validationErr.Field(), deferredAggregatePaths) {
			continue
		}
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

func partialSchemaDeferredAggregatePaths(schemaErr *zarfValue.SchemaValidationError, unknownPaths []string) []string {
	var paths []string
	for _, validationErr := range schemaErr.Errors {
		if !isPartialSchemaBranchingError(validationErr.Type()) {
			continue
		}
		field := normalizeSchemaValuePath(validationErr.Field())
		if unknownIntersectsPathSubtree(field, unknownPaths) {
			paths = append(paths, field)
		}
	}
	return paths
}

func partialSchemaErrorIsInDeferredAggregate(field string, aggregatePaths []string) bool {
	field = normalizeSchemaValuePath(field)
	for _, aggregatePath := range aggregatePaths {
		if isValuePathAncestorOrEqual(aggregatePath, field) {
			return true
		}
	}
	return false
}

func shouldDeferPartialSchemaError(errorType, field, property string, unknownPaths []string) bool {
	field = normalizeSchemaValuePath(field)

	if isPartialSchemaPropertyError(errorType) {
		// An explicitly configured unknown value does not make its key unknown.
		return unknownAncestorMayChangePath(joinValuePath(field, property), unknownPaths)
	}

	if errorType == "required" {
		return unknownAtOrAbovePath(joinValuePath(field, property), unknownPaths)
	}

	if isPartialSchemaDirectValueError(errorType) {
		return unknownAtOrAbovePath(field, unknownPaths)
	}

	// JSON Schema can apply every other error type to arrays, objects, or
	// combinator branches. Keep it only when the entire affected subtree is known.
	return unknownIntersectsPathSubtree(field, unknownPaths)
}

func isPartialSchemaPropertyError(errorType string) bool {
	switch errorType {
	case "additional_property_not_allowed", "invalid_property_name", "invalid_property_pattern":
		return true
	default:
		return false
	}
}

func isPartialSchemaDirectValueError(errorType string) bool {
	switch errorType {
	case "invalid_type", "string_gte", "string_lte", "pattern", "format", "multiple_of", "number_gte", "number_gt", "number_lte", "number_lt":
		return true
	default:
		return false
	}
}

// These validators report child errors from a branch or candidate item that
// may become valid once an unknown value resolves. Their child errors must
// defer with the parent result.
func isPartialSchemaBranchingError(errorType string) bool {
	switch errorType {
	case "number_any_of", "number_one_of", "contains", "condition_then", "condition_else":
		return true
	default:
		return false
	}
}

func unknownAncestorMayChangePath(path string, unknownPaths []string) bool {
	for _, unknownPath := range unknownPaths {
		if unknownPath != path && isValuePathAncestorOrEqual(unknownPath, path) {
			return true
		}
	}
	return false
}

func unknownAtOrAbovePath(path string, unknownPaths []string) bool {
	for _, unknownPath := range unknownPaths {
		if isValuePathAncestorOrEqual(unknownPath, path) {
			return true
		}
	}
	return false
}

func unknownIntersectsPathSubtree(path string, unknownPaths []string) bool {
	for _, unknownPath := range unknownPaths {
		if isValuePathAncestorOrEqual(unknownPath, path) || isValuePathAncestorOrEqual(path, unknownPath) {
			return true
		}
	}
	return false
}

func normalizeSchemaValuePath(path string) string {
	if path == "(root)" {
		return ""
	}
	return path
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
	unknownPaths := append(valuesUnknownPaths, sensitiveUnknownPaths...)
	if err := validatePartialPackageValuesAgainstSchema(ctx, defaults, schemaPath, unknownPaths); err != nil {
		return fmt.Errorf("package values do not match the package values schema: %w", err)
	}
	return nil
}

func hasConfiguredPackageValues(model PackageResourceModel) bool {
	return isDynamicAttributeConfigured(model.Values) || isDynamicAttributeConfigured(model.SensitiveValues)
}
