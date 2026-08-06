# 2. Validate `uds_package` values with the Zarf package values schema

## Status

Proposed, 2026-08-06

---

## Changelog

| Date       | Author      | Description   |
|------------|-------------|---------------|
| 2026-08-06 | Micah Nagel | Initial draft |

---

## Context

The `uds_package` resource accepts `values` and `sensitive_values` as dynamic Terraform maps. Zarf merges those maps into the package's default `values.yaml` during deploy. A package can also include a `values.schema.json` file, which Zarf validates during deploy time.

Before this decision, the provider performed a custom plan-time check for "value correctness". It loaded the package metadata, collected configured value paths, and required each path to match a chart `values[].sourcePath` entry. That check did not validate types, required properties, enums, numeric ranges, etc (anything that would be in the JSON schema).

The source-path check also assumed chart value mappings were the only package consumer of values. A package can consume values through `templatedValuesFiles` and other templating paths. The provider cannot infer every valid consumer from chart mappings, so source-path failures could reject valid package input.

Terraform plans contain unknown values when a UDS package value depends on another resource's output, a data source, or something else created in the same apply. Plan validation needs to keep these unknowns in mind when validating values to not reject configuration that will be valid once known.

## Decision

When `values` or `sensitive_values` is configured and the loaded package contains `values.schema.json`, the provider validates the effective package values during `ModifyPlan`.

The provider builds the plan-time document in this order:

1. Read the package `values.yaml` defaults.
2. Convert `values` and `sensitive_values` to partial Go maps.
3. Merge the partial maps, then deep-merge them over the package defaults.
4. Validate the resulting document with Zarf's JSON Schema validator.

The partial converter preserves known map keys and values. It represents unknown leaves or subtrees as `nil` and records their dot paths. A partial merge does not let an unknown value from one attribute erase a known value from the other attribute.

The provider leverages Zarf code to perform the validation (against the JSON schema) and then "filters" any returned schema errors taking into account the unknown paths:

- Keep type, string, format, numeric, and scalar enum errors for known values.
- Keep additional-property and property-name errors for explicitly configured keys, including keys whose values are unknown.
- Ignore a required-property error when an unknown parent could supply the missing property at apply.
- Ignore `oneOf`, `anyOf`, `contains`, and conditional aggregate errors, plus their child errors, when an unknown can change the aggregate result.
- Ignore other errors when the error path intersects an unknown subtree.

At apply time no additional validation is performed by the provider - Zarf's deploy library code will perform JSON schema validation making any provider code duplicative at apply time.

> [!NOTE]
> Validations performed before this ADR (source path checks as described in context) will also be removed in favor of JSON Schema checking. Additional rationale behind the full removal is below in consequences and alternatives.

## Consequences

### Positive

- Users receive schema diagnostics during plan for known invalid values.
- The provider validates package defaults and both override attributes as one document.
- Package authors can use JSON Schema for constraints that chart source-path inspection cannot express.
- Packages can consume values through chart mappings, `templateValuesFiles`, and other Zarf templating without provider plan failures.
- Unknown Terraform outputs defer only errors that require the unresolved value. Known sibling errors remain visible.
- Zarf repeats validation at apply with resolved values, so deferred plan errors cannot bypass the schema.

### Negative

- Packages without `values.schema.json` lose the former source-path failure. The provider cannot safely replace it with a hard failure because chart source paths do not enumerate all valid value consumers.
- The error filter depends on error paths and gojsonschema error types. New error shapes may need explicit tests and handling.
- An unknown under an aggregate schema could skip over certain schema errors. The provider will err on the side of caution in preventing false schema errors where unknowns are concerned.

## Alternatives Considered

### Validate schemas when present and fall back to chart source-path validation when absent

Rejected. This keeps two plan-validation systems (more code complexity). It also relies too heavily on source path as the source of truth for "values shape". Chart `values[].sourcePath` entries do not cover `templateValuesFiles` or other package templating consumers, so the provider could only use source-path mismatches as warnings. A warning-only fallback adds code and user-facing noise without a reliable correctness guarantee.

### Skip schema validation entirely whenever any value is unknown

Rejected. End users will often pass resource outputs, data sources, and generated secrets into package values. Skipping the entire check would remove fast feedback for known errors (ex: known invalid types such as `replicas = "three"` or `mode = "unsupported"`).

### Validate the partial document and return every raw schema error

Rejected. The partial document uses `nil` for an unknown Terraform value. JSON Schema reports type, enum, required, and branch errors against that placeholder even though the value can resolve validly at apply. `oneOf`, `anyOf`, `contains`, and conditionals can also report failures on a parent path or speculative child branch. Returning those errors would make valid plans fail.

### Validate only during apply

Rejected. Zarf already validates during deploy, but apply-time failures spend time loading the package, preparing the deployment, and resolving dependencies before reporting a configuration error. Plan-time validation gives users feedback at the point where they edit Terraform while preserving Zarf's full validation at apply.
