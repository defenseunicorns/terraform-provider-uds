// Copyright 2024-2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package provider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/defenseunicorns/pkg/helpers/v2"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	zarfValue "github.com/zarf-dev/zarf/src/pkg/value"
)

func TestDynamicToPartialValuesKeepsKnownKeysWithUnknownValues(t *testing.T) {
	value := types.DynamicValue(types.ObjectValueMust(
		map[string]attr.Type{
			"allowed": types.StringType,
			"typo":    types.StringType,
			"nested": types.ObjectType{AttrTypes: map[string]attr.Type{
				"typo": types.StringType,
			}},
		},
		map[string]attr.Value{
			"allowed": types.StringValue("known"),
			"typo":    types.StringUnknown(),
			"nested": types.ObjectValueMust(
				map[string]attr.Type{"typo": types.StringType},
				map[string]attr.Value{"typo": types.StringUnknown()},
			),
		},
	))

	values, unknownPaths, err := dynamicToPartialValues("values", value)

	require.NoError(t, err)
	assert.Equal(t, "known", values["allowed"])
	assert.Contains(t, values, "typo")
	assert.Nil(t, values["typo"])
	nested, ok := values["nested"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, nested, "typo")
	assert.Nil(t, nested["typo"])
	assert.ElementsMatch(t, []string{"typo", "nested.typo"}, unknownPaths)
}

func TestDynamicToPartialValuesCoversUnknownShapes(t *testing.T) {
	collectionCases := []struct {
		name  string
		value types.Dynamic
	}{
		{
			name: "list element",
			value: types.DynamicValue(types.ObjectValueMust(
				map[string]attr.Type{"items": types.ListType{ElemType: types.StringType}},
				map[string]attr.Value{"items": types.ListValueMust(types.StringType, []attr.Value{
					types.StringValue("known"),
					types.StringUnknown(),
				})},
			)),
		},
		{
			name: "set element",
			value: types.DynamicValue(types.ObjectValueMust(
				map[string]attr.Type{"items": types.SetType{ElemType: types.StringType}},
				map[string]attr.Value{"items": types.SetValueMust(types.StringType, []attr.Value{types.StringUnknown()})},
			)),
		},
		{
			name: "map element",
			value: types.DynamicValue(types.ObjectValueMust(
				map[string]attr.Type{"items": types.MapType{ElemType: types.StringType}},
				map[string]attr.Value{"items": types.MapValueMust(types.StringType, map[string]attr.Value{
					"known":   types.StringValue("known"),
					"unknown": types.StringUnknown(),
				})},
			)),
		},
		{
			name: "tuple element",
			value: types.DynamicValue(types.ObjectValueMust(
				map[string]attr.Type{"items": types.TupleType{ElemTypes: []attr.Type{types.StringType, types.StringType}}},
				map[string]attr.Value{"items": types.TupleValueMust([]attr.Type{types.StringType, types.StringType}, []attr.Value{
					types.StringValue("known"),
					types.StringUnknown(),
				})},
			)),
		},
	}

	tests := []struct {
		name         string
		value        types.Dynamic
		unknownPaths []string
		wantError    string
	}{
		{name: "null", value: types.DynamicNull()},
		{name: "root unknown", value: types.DynamicUnknown(), unknownPaths: []string{""}},
		{
			name: "unknown intermediate object",
			value: types.DynamicValue(types.ObjectValueMust(
				map[string]attr.Type{
					"known":  types.StringType,
					"nested": types.ObjectType{AttrTypes: map[string]attr.Type{"value": types.StringType}},
				},
				map[string]attr.Value{
					"known":  types.StringValue("known"),
					"nested": types.ObjectUnknown(map[string]attr.Type{"value": types.StringType}),
				},
			)),
			unknownPaths: []string{"nested"},
		},
		{name: "root scalar", value: types.DynamicValue(types.StringValue("not a map")), wantError: "values must be a map or object"},
	}
	for _, collectionCase := range collectionCases {
		tests = append(tests, struct {
			name         string
			value        types.Dynamic
			unknownPaths []string
			wantError    string
		}{name: collectionCase.name, value: collectionCase.value, unknownPaths: []string{"items.1"}})
	}
	for i := range tests {
		if tests[i].name == "set element" {
			tests[i].unknownPaths = []string{"items.0"}
		}
		if tests[i].name == "map element" {
			tests[i].unknownPaths = []string{"items.unknown"}
		}
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			values, unknownPaths, err := dynamicToPartialValues("values", tc.value)

			if tc.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantError)
				return
			}

			require.NoError(t, err)
			assert.ElementsMatch(t, tc.unknownPaths, unknownPaths)
			if tc.name == "null" || tc.name == "root unknown" {
				assert.Empty(t, values)
			}
			// Collection values are intentionally represented as slices with nil
			// placeholders for unknown elements.
			if tc.name == "list element" || tc.name == "set element" || tc.name == "tuple element" {
				items, ok := values["items"].([]any)
				if assert.True(t, ok) {
					if tc.name == "set element" {
						assert.Len(t, items, 1)
					} else {
						assert.Len(t, items, 2)
						assert.Contains(t, items, "known")
					}
					assert.Contains(t, items, nil)
				}
			}
			if tc.name == "map element" {
				items, ok := values["items"].(map[string]any)
				if assert.True(t, ok) {
					assert.Equal(t, "known", items["known"])
					assert.Nil(t, items["unknown"])
				}
			}
		})
	}
}

func TestMergePartialPackageValuesPreservesKnownSubtrees(t *testing.T) {
	knownValues := zarfValue.Values{
		"config": map[string]any{
			"public": "known",
		},
	}
	unknownValues := zarfValue.Values{
		"config": nil,
	}

	assert.Equal(t, knownValues, mergePartialPackageValues(knownValues, unknownValues))
	assert.Equal(t, knownValues, mergePartialPackageValues(unknownValues, knownValues))
}

func TestValidatePlannedPackageValuesDefersUnknownMapConflicts(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, layout.ValuesYAML), []byte("config: {}\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, layout.ValuesSchema), []byte(`{
  "type": "object",
  "properties": {
    "config": {
      "type": "object",
      "properties": {"public": {"type": "string"}},
		"additionalProperties": false
    }
  }
}`), 0o600))
	valuesHash, err := helpers.GetSHA256OfFile(filepath.Join(dir, layout.ValuesYAML))
	require.NoError(t, err)
	schemaHash, err := helpers.GetSHA256OfFile(filepath.Join(dir, layout.ValuesSchema))
	require.NoError(t, err)
	checksums := fmt.Sprintf("%s %s\n%s %s\n", valuesHash, layout.ValuesYAML, schemaHash, layout.ValuesSchema)
	require.NoError(t, os.WriteFile(filepath.Join(dir, layout.Checksums), []byte(checksums), 0o600))
	aggregateChecksum, err := helpers.GetSHA256OfFile(filepath.Join(dir, layout.Checksums))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, layout.ZarfYAML), []byte(`
apiVersion: zarf.dev/v1alpha1
kind: ZarfPackageConfig
metadata:
  name: partial-merge-test
  version: 1.0.0
  aggregateChecksum: `+aggregateChecksum+`
components: []
`), 0o600))

	pkgLayout, err := layout.LoadFromDir(context.Background(), dir, layout.PackageLayoutOptions{
		VerificationStrategy: layout.VerifyNever,
	})
	require.NoError(t, err)

	model := NewTestPackageResourceModel(
		WithValues(types.DynamicValue(types.ObjectValueMust(
			map[string]attr.Type{
				"config": types.ObjectType{AttrTypes: map[string]attr.Type{"public": types.StringType}},
			},
			map[string]attr.Value{
				"config": types.ObjectValueMust(
					map[string]attr.Type{"public": types.StringType},
					map[string]attr.Value{"public": types.StringValue("known")},
				),
			},
		))),
		WithSensitiveValues(types.DynamicValue(types.ObjectValueMust(
			map[string]attr.Type{"config": types.MapType{ElemType: types.StringType}},
			map[string]attr.Value{"config": types.MapUnknown(types.StringType)},
		))),
	)

	r := &PackageResource{}
	assert.NoError(t, r.validatePlannedPackageValuesAgainstSchema(context.Background(), model, pkgLayout))
}

func TestValidatePackageValuesAgainstSchema(t *testing.T) {
	schemaPath := filepath.Join(t.TempDir(), "values.schema.json")
	require.NoError(t, os.WriteFile(schemaPath, []byte(`{
  "type": "object",
  "properties": {
    "global": {
      "type": "object",
      "properties": {
        "domain": {"type": "string"}
      }
    }
  },
  "required": ["global"],
  "additionalProperties": false
}`), 0o600))

	assert.NoError(t, validatePackageValuesAgainstSchema(context.Background(), zarfValue.Values{
		"global": map[string]any{"domain": "unicorndemo.dev"},
	}, schemaPath))
	assert.Error(t, validatePackageValuesAgainstSchema(context.Background(), zarfValue.Values{
		"global": map[string]any{"domain": 123},
	}, schemaPath))
	assert.Error(t, validatePackageValuesAgainstSchema(context.Background(), zarfValue.Values{}, schemaPath), "required values should be enforced when the effective document is known")
}

func TestValidatePartialPackageValuesAgainstSchemaPreservesUnknownKeys(t *testing.T) {
	schemaPath := filepath.Join(t.TempDir(), "values.schema.json")
	require.NoError(t, os.WriteFile(schemaPath, []byte(`{
  "type": "object",
  "properties": {
    "allowed": {"type": "string"},
    "service": {
      "type": "object",
      "properties": {"host": {"type": "string"}},
      "additionalProperties": false
    }
  },
  "additionalProperties": false
}`), 0o600))

	assert.NoError(t, validatePartialPackageValuesAgainstSchema(
		context.Background(),
		zarfValue.Values{"allowed": nil},
		schemaPath,
		[]string{"allowed"},
	))

	err := validatePartialPackageValuesAgainstSchema(
		context.Background(),
		zarfValue.Values{"typo": nil},
		schemaPath,
		[]string{"typo"},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "typo")

	err = validatePartialPackageValuesAgainstSchema(
		context.Background(),
		zarfValue.Values{"service": map[string]any{"typo": nil}},
		schemaPath,
		[]string{"service.typo"},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "typo")
}

func TestValidatePartialPackageValuesAgainstSchemaDefersOnlyRelatedRequiredErrors(t *testing.T) {
	schemaPath := filepath.Join(t.TempDir(), "values.schema.json")
	require.NoError(t, os.WriteFile(schemaPath, []byte(`{
  "type": "object",
  "properties": {
    "requiredValue": {"type": "string"},
    "computedValue": {"type": "string"}
  },
  "required": ["requiredValue"],
  "additionalProperties": false
}`), 0o600))

	err := validatePartialPackageValuesAgainstSchema(
		context.Background(),
		zarfValue.Values{"computedValue": nil},
		schemaPath,
		[]string{"computedValue"},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requiredValue")

	assert.NoError(t, validatePartialPackageValuesAgainstSchema(
		context.Background(),
		zarfValue.Values{},
		schemaPath,
		[]string{""},
	))
}

func TestValidatePartialPackageValuesAgainstSchemaRetainsContainerErrors(t *testing.T) {
	schemaPath := filepath.Join(t.TempDir(), "values.schema.json")
	require.NoError(t, os.WriteFile(schemaPath, []byte(`{
  "type": "object",
  "propertyNames": {"pattern": "^[a-z]+$"},
  "properties": {
    "items": {
      "type": "array",
      "maxItems": 1,
      "items": {"type": "string"}
    }
  }
}`), 0o600))

	err := validatePartialPackageValuesAgainstSchema(
		context.Background(),
		zarfValue.Values{"items": []any{"known", nil}},
		schemaPath,
		[]string{"items.1"},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at most 1 item")

	err = validatePartialPackageValuesAgainstSchema(
		context.Background(),
		zarfValue.Values{"INVALID": nil},
		schemaPath,
		[]string{"INVALID"},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "INVALID")
}

func TestMergePackageValuesWithDefaultsBeforeSchemaValidation(t *testing.T) {
	schemaPath := filepath.Join(t.TempDir(), "values.schema.json")
	require.NoError(t, os.WriteFile(schemaPath, []byte(`{
  "type": "object",
  "properties": {
    "service": {
      "type": "object",
      "minProperties": 2,
      "properties": {
        "host": {"type": "string"},
        "port": {"type": "integer"}
      }
    }
  }
}`), 0o600))

	defaults := zarfValue.Values{"service": map[string]any{"host": "localhost"}}
	overrides := zarfValue.Values{"service": map[string]any{"port": 8080}}

	assert.Error(t, validatePackageValuesAgainstSchema(context.Background(), overrides, schemaPath), "partial overrides should not be validated as the complete document")
	assert.NoError(t, validatePackageValuesAgainstSchema(context.Background(), deepMergePackageValues(defaults, overrides), schemaPath))
}
