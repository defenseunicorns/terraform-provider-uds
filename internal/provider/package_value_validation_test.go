// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package provider

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	zarfValue "github.com/zarf-dev/zarf/src/pkg/value"
)

func TestDynamicToPartialValuesKeepsKnownSiblings(t *testing.T) {
	value := types.DynamicValue(types.ObjectValueMust(
		map[string]attr.Type{
			"known": types.StringType,
			"nested": types.ObjectType{AttrTypes: map[string]attr.Type{
				"computed": types.StringType,
			}},
		},
		map[string]attr.Value{
			"known": types.StringValue("value"),
			"nested": types.ObjectValueMust(
				map[string]attr.Type{"computed": types.StringType},
				map[string]attr.Value{"computed": types.StringUnknown()},
			),
		},
	))

	values, unknownPaths, err := dynamicToPartialValues("values", value)

	require.NoError(t, err)
	assert.Equal(t, "value", values["known"])
	nested, ok := values["nested"].(map[string]any)
	require.True(t, ok)
	assert.Nil(t, nested["computed"])
	assert.Equal(t, []string{"nested.computed"}, unknownPaths)
}

func TestDynamicToPartialValuesHandlesUnknownShapes(t *testing.T) {
	tests := []struct {
		name         string
		value        types.Dynamic
		unknownPaths []string
		wantError    string
	}{
		{name: "null", value: types.DynamicNull()},
		{name: "root unknown", value: types.DynamicUnknown(), unknownPaths: []string{""}},
		{
			name: "unknown object",
			value: types.DynamicValue(types.ObjectValueMust(
				map[string]attr.Type{"config": types.ObjectType{AttrTypes: map[string]attr.Type{"enabled": types.BoolType}}},
				map[string]attr.Value{"config": types.ObjectUnknown(map[string]attr.Type{"enabled": types.BoolType})},
			)),
			unknownPaths: []string{"config"},
		},
		{
			name: "list, set, tuple, and map elements",
			value: types.DynamicValue(types.ObjectValueMust(
				map[string]attr.Type{
					"list":  types.ListType{ElemType: types.StringType},
					"set":   types.SetType{ElemType: types.StringType},
					"tuple": types.TupleType{ElemTypes: []attr.Type{types.StringType, types.StringType}},
					"map":   types.MapType{ElemType: types.StringType},
				},
				map[string]attr.Value{
					"list":  types.ListValueMust(types.StringType, []attr.Value{types.StringValue("known"), types.StringUnknown()}),
					"set":   types.SetValueMust(types.StringType, []attr.Value{types.StringUnknown()}),
					"tuple": types.TupleValueMust([]attr.Type{types.StringType, types.StringType}, []attr.Value{types.StringValue("known"), types.StringUnknown()}),
					"map":   types.MapValueMust(types.StringType, map[string]attr.Value{"unknown": types.StringUnknown()}),
				},
			)),
			unknownPaths: []string{"list.1", "set.0", "tuple.1", "map.unknown"},
		},
		{name: "root scalar", value: types.DynamicValue(types.StringValue("not a map")), wantError: "values must be a map or object"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			values, unknownPaths, err := dynamicToPartialValues("values", tc.value)

			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				return
			}

			require.NoError(t, err)
			assert.ElementsMatch(t, tc.unknownPaths, unknownPaths)
			if tc.name == "null" || tc.name == "root unknown" {
				assert.Empty(t, values)
			}
		})
	}
}

func TestValidatePartialPackageValuesAgainstSchemaRetainsKnownErrors(t *testing.T) {
	schemaPath := filepath.Join(t.TempDir(), "values.schema.json")
	require.NoError(t, os.WriteFile(schemaPath, []byte(`{
  "type": "object",
  "properties": {
    "typed": {"type": "string"},
    "name": {"type": "string", "pattern": "^valid", "minLength": 8},
    "replicas": {"type": "integer", "minimum": 2},
    "mode": {"enum": ["valid"]},
    "computed": {"type": "string"}
  }
}`), 0o600))

	err := validatePackageValuesAgainstSchema(
		context.Background(),
		zarfValue.Values{
			"typed":    1,
			"name":     "bad",
			"replicas": 1,
			"mode":     "invalid",
			"computed": nil,
		},
		schemaPath,
		[]string{"computed"},
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "typed")
	assert.Contains(t, err.Error(), "name")
	assert.Contains(t, err.Error(), "replicas")
	assert.Contains(t, err.Error(), "mode")
}

func TestValidatePartialPackageValuesAgainstSchemaDefersUnknownTrees(t *testing.T) {
	t.Run("collection content constraint", func(t *testing.T) {
		schemaPath := filepath.Join(t.TempDir(), "values.schema.json")
		require.NoError(t, os.WriteFile(schemaPath, []byte(`{
  "type": "object",
  "properties": {
    "items": {
      "type": "array",
      "contains": {"const": "expected"}
    }
  }
}`), 0o600))

		assert.NoError(t, validatePackageValuesAgainstSchema(
			context.Background(),
			zarfValue.Values{"items": []any{"known", nil}},
			schemaPath,
			[]string{"items.1"},
		))
	})

	t.Run("parent anyOf", func(t *testing.T) {
		schemaPath := filepath.Join(t.TempDir(), "values.schema.json")
		require.NoError(t, os.WriteFile(schemaPath, []byte(`{
  "type": "object",
  "anyOf": [
    {"properties": {"mode": {"const": "resource"}}},
    {"properties": {"mode": {"const": "data"}}}
  ]
}`), 0o600))

		assert.NoError(t, validatePackageValuesAgainstSchema(
			context.Background(),
			zarfValue.Values{"mode": nil},
			schemaPath,
			[]string{"mode"},
		))
	})

	t.Run("allOf child error", func(t *testing.T) {
		schemaPath := filepath.Join(t.TempDir(), "values.schema.json")
		require.NoError(t, os.WriteFile(schemaPath, []byte(`{
  "type": "object",
  "allOf": [
    {"properties": {"mode": {"const": "expected"}}}
  ]
}`), 0o600))

		assert.NoError(t, validatePackageValuesAgainstSchema(
			context.Background(),
			zarfValue.Values{"mode": nil},
			schemaPath,
			[]string{"mode"},
		))
	})

	t.Run("conditional", func(t *testing.T) {
		schemaPath := filepath.Join(t.TempDir(), "values.schema.json")
		require.NoError(t, os.WriteFile(schemaPath, []byte(`{
  "type": "object",
  "if": {"properties": {"mode": {"const": "enabled"}}},
  "then": {"required": ["settings"]}
}`), 0o600))

		assert.NoError(t, validatePackageValuesAgainstSchema(
			context.Background(),
			zarfValue.Values{"mode": nil},
			schemaPath,
			[]string{"mode"},
		))
	})

	t.Run("parent combinator", func(t *testing.T) {
		schemaPath := filepath.Join(t.TempDir(), "values.schema.json")
		require.NoError(t, os.WriteFile(schemaPath, []byte(`{
  "type": "object",
  "oneOf": [
    {"properties": {"mode": {"const": "resource"}}},
    {"properties": {"mode": {"const": "data"}}}
  ]
}`), 0o600))

		assert.NoError(t, validatePackageValuesAgainstSchema(
			context.Background(),
			zarfValue.Values{"mode": nil},
			schemaPath,
			[]string{"mode"},
		))
	})

	t.Run("collection uniqueness", func(t *testing.T) {
		schemaPath := filepath.Join(t.TempDir(), "values.schema.json")
		require.NoError(t, os.WriteFile(schemaPath, []byte(`{
  "type": "object",
  "properties": {
    "items": {"type": "array", "uniqueItems": true}
  }
}`), 0o600))

		assert.NoError(t, validatePackageValuesAgainstSchema(
			context.Background(),
			zarfValue.Values{"items": []any{nil, nil}},
			schemaPath,
			[]string{"items.1"},
		))
	})
}

func TestValidatePartialPackageValuesAgainstSchemaRetainsExplicitInvalidKeys(t *testing.T) {
	schemaPath := filepath.Join(t.TempDir(), "values.schema.json")
	require.NoError(t, os.WriteFile(schemaPath, []byte(`{
  "type": "object",
  "properties": {"known": {"type": "string"}},
  "additionalProperties": false
}`), 0o600))

	err := validatePackageValuesAgainstSchema(
		context.Background(),
		zarfValue.Values{"typo": nil},
		schemaPath,
		[]string{"typo"},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "typo")
}

func TestValidatePartialPackageValuesAgainstSchemaPathRelationships(t *testing.T) {
	t.Run("required error remains when a sibling is unknown", func(t *testing.T) {
		schemaPath := filepath.Join(t.TempDir(), "values.schema.json")
		require.NoError(t, os.WriteFile(schemaPath, []byte(`{
  "type": "object",
  "properties": {"computed": {"type": "string"}},
  "required": ["requiredValue"]
}`), 0o600))

		err := validatePackageValuesAgainstSchema(context.Background(), zarfValue.Values{"computed": nil}, schemaPath, []string{"computed"})
		require.ErrorContains(t, err, "requiredValue")
	})

	t.Run("required error defers when its parent is unknown", func(t *testing.T) {
		schemaPath := filepath.Join(t.TempDir(), "values.schema.json")
		require.NoError(t, os.WriteFile(schemaPath, []byte(`{
  "type": "object",
  "required": ["requiredValue"]
}`), 0o600))

		assert.NoError(t, validatePackageValuesAgainstSchema(context.Background(), zarfValue.Values{}, schemaPath, []string{""}))
	})

	t.Run("known child error remains inside an object with an unknown sibling", func(t *testing.T) {
		schemaPath := filepath.Join(t.TempDir(), "values.schema.json")
		require.NoError(t, os.WriteFile(schemaPath, []byte(`{
  "type": "object",
  "properties": {
    "config": {
      "type": "object",
      "properties": {
        "known": {"type": "string"},
        "computed": {"type": "string"}
      }
    }
  }
}`), 0o600))

		err := validatePackageValuesAgainstSchema(
			context.Background(),
			zarfValue.Values{"config": map[string]any{"known": 1, "computed": nil}},
			schemaPath,
			[]string{"config.computed"},
		)
		require.ErrorContains(t, err, "known")
	})

	t.Run("direct value error defers at an unknown value", func(t *testing.T) {
		schemaPath := filepath.Join(t.TempDir(), "values.schema.json")
		require.NoError(t, os.WriteFile(schemaPath, []byte(`{
  "type": "object",
  "properties": {"computed": {"type": "string", "pattern": "^valid"}}
}`), 0o600))

		assert.NoError(t, validatePackageValuesAgainstSchema(
			context.Background(), zarfValue.Values{"computed": nil}, schemaPath, []string{"computed"},
		))
	})

	t.Run("property-name error remains for an explicit unknown key", func(t *testing.T) {
		schemaPath := filepath.Join(t.TempDir(), "values.schema.json")
		require.NoError(t, os.WriteFile(schemaPath, []byte(`{
  "type": "object",
  "propertyNames": {"pattern": "^[a-z]+$"}
}`), 0o600))

		err := validatePackageValuesAgainstSchema(context.Background(), zarfValue.Values{"INVALID": nil}, schemaPath, []string{"INVALID"})
		require.ErrorContains(t, err, "INVALID")
	})
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
	unknownPaths := []string{}

	assert.Error(t, validatePackageValuesAgainstSchema(context.Background(), overrides, schemaPath, unknownPaths), "partial overrides should not be validated as the complete document")
	assert.NoError(t, validatePackageValuesAgainstSchema(context.Background(), deepMergePackageValues(defaults, overrides), schemaPath, unknownPaths))
}

func TestMergePartialPackageValuesPreservesKnownValuesWhenAnotherSourceIsUnknown(t *testing.T) {
	values := zarfValue.Values{
		"config": map[string]any{
			"host": "known.example.test",
			"port": 8080,
		},
	}
	sensitiveValues := zarfValue.Values{
		"config": map[string]any{
			"host": nil,
		},
	}

	assert.Equal(t, values, mergePartialPackageValues(values, sensitiveValues))
}
