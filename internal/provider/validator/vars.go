package validator

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// UniqueVarNameValidator validates the Name field within a list of Objects are all unique.
type UniqueVarNameValidator struct{}

// Description returns a plain text description of the validator.
func (v UniqueVarNameValidator) Description(ctx context.Context) string {
	return "Ensures each object's `name` is unique within the list"
}

// MarkdownDescription returns the markdown description for the validator.
func (v UniqueVarNameValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateList validates that all blocks in the list have unique values for the specified attribute.
func (v UniqueVarNameValidator) ValidateList(ctx context.Context, req validator.ListRequest, resp *validator.ListResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() || req.ConfigValue.ElementType(ctx) == nil {
		return
	}

	// Decode list of objects
	var rawValues []types.Object
	diags := req.ConfigValue.ElementsAs(ctx, &rawValues, false)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	seen := map[string]path.Path{}
	for i, value := range rawValues {
		attributes := value.Attributes()
		nameVal, ok := attributes["name"].(types.String)
		if !ok || nameVal.IsNull() || nameVal.IsUnknown() {
			// TODO: Should i add an error to the diagnostic? This isn't computed and is a required field.
			continue
		}

		key := nameVal.ValueString()
		key = strings.ToLower(key)

		p := req.Path.AtListIndex(i).AtName("name")
		if first, exists := seen[key]; exists {
			resp.Diagnostics.AddAttributeError(
				p,
				"Duplicate variable name",
				fmt.Sprintf("The name %q is duplicated in this list. First seen at %s.", key, first.String()),
			)
		} else {
			seen[key] = p
		}
	}
}

// NewUniqueVarNameValidator creates a new validator.
func NewUniqueVarNameValidator() validator.List {
	return UniqueVarNameValidator{}
}
