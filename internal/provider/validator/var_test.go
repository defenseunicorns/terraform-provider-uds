package validator

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	tfvalidator "github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestBlockObjectNameUniquenessValidator_ValidateList(t *testing.T) {
	attributeName := "name"

	tests := []struct {
		name               string
		configValue        types.List
		expectedErrorCount int
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
			configValue: types.ListValueMust(
				types.ObjectType{AttrTypes: map[string]attr.Type{attributeName: types.StringType}},
				[]attr.Value{},
			),
			expectedErrorCount: 0,
		},
		{
			name:               "single item is valid",
			configValue:        newObjectListWithAttributeValues(attributeName, []string{"FOO"}),
			expectedErrorCount: 0,
		},
		{
			name:               "multiple unique names are valid",
			configValue:        newObjectListWithAttributeValues(attributeName, []string{"FOO", "BAR", "BAZ"}),
			expectedErrorCount: 0,
		},
		{
			name:               "one duplicate name is not valid",
			configValue:        newObjectListWithAttributeValues(attributeName, []string{"FOO", "BAR", "FOO"}),
			expectedErrorCount: 1,
		},
		{
			name:               "two distinct duplicates yield two errors",
			configValue:        newObjectListWithAttributeValues(attributeName, []string{"FOO", "FOO", "BAR", "BAR"}),
			expectedErrorCount: 2,
		},
	}

	validator := NewUniqueVarNameValidator()
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

			if len(resp.Diagnostics) != tc.expectedErrorCount {
				t.Errorf("Received %d errors when we expected %d", resp.Diagnostics, tc.expectedErrorCount)
			}
		})
	}
}
