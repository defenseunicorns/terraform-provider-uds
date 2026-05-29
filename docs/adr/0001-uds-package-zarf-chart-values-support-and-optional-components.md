# 1. uds_package: Zarf chart values support and optional_components (deprecate component block)

## Status

Proposed — 2026-05-27

> **Note:** The `values`, `sensitive_values`, and `optional_components` attributes are **alpha** in lockstep with Zarf's
> `feature.Values` alpha gate. They are available now but mutually exclusive with `component` blocks —
> the `component` block is not formally deprecated until at least Zarf promotes `feature.Values`
> to stable/GA. Behavior may change before that point.

---

## Changelog

| Date       | Author           | Description   |
|------------|------------------|---------------|
| 2026-05-27 | Erickson Moskito | Initial draft |

---

## Context

The `uds_package` resource's `component` dynamic block currently serves two unrelated purposes:

1. **Chart value overrides** — an `override` sub-block provides a path/value workaround to push Helm chart values into package components. The block can reference any component — required or optional. For required components, the block exists solely to configure overrides; for optional components, its presence also causes the component to be installed.
2. **Optional component selection** — when a `component` block references an optional component and contains no `override` sub-block, its sole purpose is to opt that component into the deployment.

Example of today's configuration:

```hcl
resource "uds_package" "example" {
  source = "oci://ghcr.io/example/package:1.0.0"

  component {
    name = "my-app"
    override {
      chart_name = "my-chart"
      values = [
        { path = "replicaCount", value = "3" },
        { path = "resources.requests.memory", value = "512Mi" },
      ]
    }
  }
}
```

The override flow: `flattenComponentOverrides()` → `DeployOptions.ValuesOverridesMap` → `r.packager.Deploy()`. This mechanism predates Zarf's first-class support for deploy-time chart value injection.

Zarf introduced the `feature.Values` alpha flag (since v0.64.0, currently v0.76.0 in this provider). When enabled, the Zarf SDK accepts `DeployOptions.Values` (`map[string]any`), deep-merged into chart values at deploy time according to `chart.values[].sourcePath → targetPath` mappings declared by the package author in `zarf.yaml`.

**Critical constraint:** `DeployOptions.Values` is not a raw Helm `--values` passthrough. Package authors must explicitly declare `chart.values` source/target mappings in `zarf.yaml`. Without those mappings, values supplied at deploy time have no effect. This is a prerequisite for the feature regardless of tooling — cli-next carries the same requirement. Tofu-author migration can proceed package-by-package as upstream packages are updated.

With the Zarf alpha feature superseding the override workaround, the `component` block's dual purpose becomes a design liability: once overrides are deprecated, a `component` block used solely for optional component selection is an unnecessarily verbose pattern, and the block's dual purpose makes it difficult to explain to users which concern to reach for.

### cli-next reference implementation

The next-generation `uds-cli-next` tool implements a similar feature for bundles:

```hcl
# bundle.uds.hcl (cli-next syntax — NOT compatible with this provider)
package "my-app" {
  source              = "oci://ghcr.io/example/package:1.0.0"
  optional_components = ["metrics", "debug-tools"]
  values_files        = ["base.yaml", "overrides.yaml"]
}
```

cli-next accepts file paths and runs a Go `text/template` rendering step before merging via `value.ParseFiles()`. cli-next is a Go application with a custom HCL schema — it has no access to Tofu's HCL evaluation engine and must read and render files itself. Bundle-level variables flow two ways: top-level scalar values (strings, floats, bools) are flattened and uppercased for `DeployOptions.SetVariables`; complex values (lists, nested objects) are available to values-file templates but skipped from `SetVariables`. The provider differs: `vars`/`sensitive_vars` are explicit per-resource name/value string pairs passed to `DeployOptions.SetVariables` largely as configured; lowercase normalization applies to duplicate detection and to computed exported `set_variables` state, not to the input values themselves.

---

## Decision

Add two new top-level attributes to `uds_package` and deprecate the `component` dynamic block. The two features are coupled — both are driven by the need to replace the `component` block's dual purpose.

### `values` attribute (alpha)

A new optional `values` attribute accepts an HCL map of arbitrary depth (`schema.DynamicAttribute`). The provider encodes the map internally to `map[string]any` and passes it to `DeployOptions.Values` at deploy time. No YAML parsing or template rendering occurs in the provider — all evaluation is handled by Tofu's HCL engine before the provider receives the value:

```hcl
resource "uds_package" "example" {
  source = "oci://ghcr.io/example/package:1.0.0"

  values = {
    replicaCount = 3
    resources = {
      requests = {
        memory = "512Mi"
        cpu    = "250m"
      }
    }
  }
}
```

Users who need to compose values from multiple sources use Tofu's native `merge()` function:

```hcl
values = merge(
  jsondecode(file("${path.module}/base-values.json")),
  { replicaCount = var.replica_count }
)
```

Behavior:

- **Type: `schema.DynamicAttribute`.** Accepts any HCL map type at runtime — scalars, nested objects, lists, and mixed types are all valid. The Terraform plugin framework does not enforce a static type constraint; structural errors surface at apply time or through provider validation. Tofu cannot preview deferred map values (e.g., values sourced from other resource outputs or data sources) in plan output — they appear as `(known after apply)` until apply.
- **Merging is the user's responsibility.** When values must be composed from multiple sources, use Tofu's built-in `merge()` function before assigning to the attribute. The provider does not own merge semantics; a single composed expression is passed to the provider and stored in state.
- **Source is unrestricted.** Because the attribute accepts any HCL expression, values can originate from local files, secrets managers, other provider data sources, resource outputs, or any Tofu-evaluable expression. The provider is decoupled from the storage mechanism entirely.
- **State tracking.** The attribute value is stored as-is in Tofu state. Tofu's native map comparison drives plan diffs — structural changes trigger a re-deploy; changes that produce the same resolved map do not.
- **Content is case-sensitive.** Key casing is preserved as configured.
- **Validation hooks.** Input-shape checks and key-conflict detection (duplicate keys between `values` and `sensitive_values`) are performed in `ValidateConfig` for statically known values. The provider should also validate that every chart value in `values` and `sensitive_values` corresponds to an exposed `chart.values[].sourcePath` mapping in the package, surfacing them early at plan time. This validation requires loading package metadata and is performed in `ModifyPlan`. If `values` contains unknowns at plan time (e.g. sourced from another resource output), key validation is deferred to apply.
- **Mutual exclusivity.** If `values` is specified (including as an empty map) and any `component` block exists — with or without an `override` sub-block — schema validation emits an error:
  > `values cannot be specified together with component blocks`

  An explicitly empty `values = {}` is a meaningful signal: the user has opted into the values paradigm, even if no values are currently supplied. This intentionally disallows `component` blocks. Callers who do not intend to use `values` must omit the attribute or set it to `null` — not `{}`. In particular, when wiring `values = var.package_values`, the variable must default to `null` rather than `{}` to avoid unintentionally triggering mutual exclusivity in environments that do not supply values.

  This is also a deliberate policy decision around migration. A name-only `component` block alongside `values` is technically valid — optional component selection and chart value customization are independent concerns. Allowing it would permit incremental migration (adopt `values` first, migrate to `optional_components` separately). The strict rule is chosen instead because it avoids a mixed-paradigm configuration that is harder to document and reason about during the deprecation window, and because it nudges users toward a clean cut-over to both new attributes together. Users who want to migrate incrementally should complete both changes in the same `tofu apply`.
- **No drift detection for chart values.** Chart value drift is not detectable. Zarf's deployed package Kubernetes secret records package metadata and installed components, not the applied chart values. If chart configuration diverges from stored `values` through out-of-band Helm operations or a failed partial deploy, the provider's Read operation will not detect it and `tofu plan` will show no diff. Recovery requires forcing redeployment via `tofu apply -replace=<resource address>`.
- **Zarf feature flag.** The provider enables `feature.Values` before calling `Deploy`, mirroring the cli-next approach.
- **Alpha status.** Documented as alpha; semantics may change until Zarf promotes `feature.Values` to stable/GA.

**Not interchangeable with cli-next.** cli-next accepts YAML file paths and applies Go `text/template` rendering because it is a Go application with no access to Tofu's HCL evaluation engine. The provider accepts HCL map expressions — a fundamentally different input mechanism. Users cannot share values configurations between tools: the input format (HCL map expression vs. YAML file), the variable reference syntax, and the scoping model all differ.

### `sensitive_values` attribute (alpha)

A new optional `sensitive_values` attribute mirrors `values` in type (`schema.DynamicAttribute`) but is marked `Sensitive: true` in the schema. Its content is redacted from Tofu plan and apply output. The same alpha status and `DynamicAttribute` trade-offs apply.

```hcl
resource "uds_package" "example" {
  source = "oci://ghcr.io/example/package:1.0.0"

  values = {
    replicaCount = 3
  }
  sensitive_values = {
    auth = {
      token = var.api_token
    }
  }
}
```

`values` and `sensitive_values` may both be specified simultaneously. Before passing to `DeployOptions.Values`, the provider merges both maps. **Duplicate top-level keys between `values` and `sensitive_values` are a validation error** caught at `ValidateConfig` for statically known values, and deferred to apply for values not known at plan time:

> `duplicate key "<key>" found in both values and sensitive_values`

The mutual exclusivity rule applies to `sensitive_values` independently:

> `sensitive_values cannot be specified together with component blocks`

An explicitly empty `sensitive_values = {}` carries the same meaning as `values = {}` — it is a signal that the user has opted into the values paradigm and intentionally disallows `component` blocks. Callers must omit the attribute or set it to `null` when they do not intend to use `sensitive_values`.

As with `values`, chart value drift is not detectable — the same recovery path applies (`tofu apply -replace=<resource address>`).

### `optional_components` attribute

A new optional `optional_components = set(string)` attribute selects optional package components for installation. A set type is used because order is irrelevant — Tofu's set diff correctly detects additions and removals without being sensitive to list position:

```hcl
resource "uds_package" "example" {
  source              = "oci://ghcr.io/example/package:1.0.0"
  optional_components = ["metrics-server", "debug-tools"]
}
```

Behavior:

- **Optional.** Omitting the attribute means no optional components are installed.
- **Order-insensitive.** Set semantics ensure Tofu does not treat reordering as a change.
- **Case-insensitive input matching.** User-supplied names are compared case-insensitively against package metadata. The provider stores and passes the canonical component name from the package metadata to Zarf, not the user-supplied casing. Zarf component matching is case-sensitive, so passing the canonical name is required for correct behavior.
- **Validation against package metadata.** Component name validation requires loading package metadata — either from a local path or OCI registry. This may require registry access at plan time. If package access is unavailable during plan, validation is deferred to apply.
  - Unknown name → error: `component "<name>" not found in package`
  - Required component name → error: `component "<name>" is required and cannot be specified in optional_components`
- **Drift detection.** `optional_components` is stored in state as the set of canonical component names. The provider's Read operation compares the stored set against the list of installed components recorded in the deployed package's Kubernetes secret. If any stored component is absent from the deployed state, Read updates the refreshed state so the next `tofu plan` detects drift and `tofu apply` redeploys.
- **Redeploy on change.** Adding or removing a name causes a plan diff and re-deploy on `tofu apply`.
- **Mutual exclusivity.** If `optional_components` is specified (including as an empty set) and any `component` block exists, schema validation emits an error:
  > `optional_components cannot be specified together with component blocks`

  As with `values`, an explicitly empty `optional_components = []` signals that the user has opted into the new paradigm. Callers must omit the attribute or set it to `null` — not `[]` — when they do not intend to use `optional_components`.

### `component` block deprecation

The `component` block is not formally deprecated until Zarf promotes `feature.Values` to stable/GA. At that point, the schema gains a `DeprecationMessage`:

> `The component block is deprecated. Use optional_components to select optional components and values to supply chart values. The component block will be removed in a future version.`

Until then, `values` and `optional_components` are available as alpha features but cannot coexist with `component` blocks — the mutual exclusivity rules are enforced regardless of deprecation status. This avoids marking the `component` block deprecated while the replacement feature is still alpha, preventing users from being nudged toward an unstable API.

Any code paths that exclusively support the `component` block — schema definitions, validators, override flattening, deploy wiring — must be marked with a `TODO` comment referencing the future removal:

```go
// TODO: remove when component block is removed
```

This keeps removal work discoverable and ensures deprecated code does not silently persist past the removal milestone.

### Migration path

The two paradigms (`component` blocks vs. `values` + `optional_components`) are mutually exclusive by schema validation. There is no conversion of one set of configuration to the other — the values HCL map is new content written against the package's `chart.values` mappings, not a transformation of override path/value pairs. Switching paradigms removes the old configuration from state and replaces it with the new; switching always triggers a full package redeploy on the next `tofu apply`.

**Forward (component blocks → values + optional_components):**

1. Confirm the target package has shipped `chart.values[].sourcePath → targetPath` mappings in `zarf.yaml` (required prerequisite — package author work, independent of Tofu changes).
2. Remove all `component` blocks from the resource configuration.
3. Author a `values` HCL map targeting the package's exposed `sourcePath` keys.
4. Add `values = { ... }` and (if applicable) `optional_components = [...]`.
5. Run `tofu plan` — plan shows a full redeploy. Run `tofu apply`.

**Backward (values → component blocks):**

Remove `values` and `optional_components`, re-add `component` blocks with `override` sub-blocks, run `tofu apply`. As with the forward direction, this requires re-authoring configuration from scratch — `{path, value}` override pairs are structurally different from the HCL map values used in `values`. There is no mechanical conversion in either direction; both paths require new configuration content. No conversion tooling is provided.

---

## Consequences

### Positive

- The provider aligns with Zarf's first-class deploy-time values mechanism; `ValuesOverridesMap` becomes a legacy path.
- Concerns are cleanly separated: `optional_components` selects components; `values` customizes chart configuration.
- Tofu-native rendering gives users the full HCL ecosystem (secrets managers, data sources, resource cross-references, `templatefile()`) with no additional provider surface.
- Per-package migration: consumers migrate resources one at a time as upstream packages are updated — no big-bang cutover required.
- Conceptually aligns with cli-next (`optional_components`, layered values), reducing cognitive overhead for users working across both tools.

### Negative

- **Migration effort.** Users must author new values HCL maps targeting the package's `chart.values[].sourcePath` mappings. This content cannot be derived from existing override path/value pairs — it is new configuration written against the updated package schema.
- **Not interchangeable with cli-next.** cli-next accepts YAML file paths with Go `text/template` rendering; the provider accepts HCL map expressions. Input format, variable reference syntax, and scoping model all differ — values configurations cannot be shared between tools. Duplicate maintenance for organizations using both tools.
- **`DynamicAttribute` defers type validation.** The provider cannot enforce a type constraint on `values` or `sensitive_values` at schema validation time. Structural errors surface at apply time or through `ValidateConfig`/`ModifyPlan` hooks. Plan output shows `(known after apply)` for deferred map values from resource outputs or data sources, reducing plan legibility for dynamic configurations.
- **No chart value drift detection.** Unlike `optional_components`, chart value drift cannot be detected by the Read operation — Zarf's package state does not record applied chart values. Out-of-band Helm changes are invisible to `tofu plan`; recovery requires `tofu apply -replace=<resource address>`.
- **Alpha dependency.** `values` inherits Zarf's alpha status. If Zarf changes the `feature.Values` API before GA, the provider must adapt.
- **Name ambiguity (transitional).** The top-level `values` attribute and the `override` block's nested `values` attribute share the same name during the deprecation window. Mutual exclusivity prevents coexistence; the conflict fully resolves when the `component` block is removed.

---

## Alternatives Considered

### `values` and `sensitive_values` as ordered lists (layered merge)

Define `values` and `sensitive_values` as `list(map)` or `list(string)` so multiple value documents can be supplied in order, with the provider merging them layer-by-layer — mirroring cli-next's `values_files` layered approach where index 0 is base and each subsequent entry overrides the previous.

Not adopted. We believe single-map configurations will be the primary usage pattern, though real-world adoption may prove otherwise. Tofu's native `merge()` function covers multi-source composition when needed and gives users explicit control over merge logic, ordering, and conflict resolution at the call site — the provider does not need to own those semantics. A single map attribute is simpler to reason about, simpler to store in state, and consistent with how other Tofu providers model structured configuration.

### `values` as raw YAML strings

Accept `values` as a raw YAML string (or `list(string)` of YAML documents), with the provider parsing YAML internally before passing to `DeployOptions.Values`. Users would supply content via `file()`, `yamlencode()`, or inline strings.

Not adopted. `DeployOptions.Values` is typed as `value.Values` (`map[string]any`) — Zarf takes a Go map directly, not raw YAML. Accepting YAML strings would require parsing them back into the same `map[string]any` that an HCL map expression produces natively, adding a YAML parsing layer with no benefit. An HCL map is the natural fit: it integrates with Tofu's type system and plan diffing, requires no provider-side parsing, and avoids forcing users to reason about two syntaxes within the same configuration file. The `schema.DynamicAttribute` type makes an HCL-native map viable without a fixed type constraint.

### `values_files = list(string)` of file paths

Accept a list of local file paths, with the provider reading the files at deploy time. This mirrors the cli-next `values_files` surface. A file-path attribute restricts sources to the local filesystem regardless of what is stored in state. Accepting an HCL map expression instead opens the attribute to secrets managers, remote storage, other provider data sources, and any Tofu-evaluable expression — with no provider-side file I/O. cli-next reads file paths because it is a Go application with no access to Tofu's evaluation engine; the provider has no such constraint.

### `merge_values()` provider function

Expose a provider-registered `merge_values()` function accepting multiple maps and returning the deep-merged result, with `values` accepting a single map. The function name makes layering explicit at the call site.

Not adopted — superseded by the decision to accept a single HCL map. Tofu's native `merge()` function covers multi-source composition at the call site without adding a provider-registered function to the public API surface. The HCL-native approach eliminates any YAML parsing layer entirely.

### Deprecate overrides only; retain name-only `component` blocks for optional component selection

Deprecate the `override` sub-block and introduce `values`, but keep the `component` block (name only, no override) as the mechanism for selecting optional components, avoiding the introduction of `optional_components`. This works mechanically but the user experience is awkward: after deprecating overrides, the block's sole purpose would be optional component selection, yet users would write a block construct (`component { name = "metrics-server" }`) to express what is semantically list membership. A flat `optional_components = ["metrics-server"]` attribute communicates intent more directly and aligns with the cli-next surface.

### Cross-tool compatible templates (Go `text/template` in provider)

Implement Go `text/template` pre-processing in the provider so that values file templates work with both cli-next and this provider. Full compatibility is unachievable regardless: cli-next uses bundle-scoped global variables (`{{ .vars.domain }}`); the provider uses resource-scoped Tofu variables (`${var.domain}`). The scoping model is structurally different — values files referencing variables must be rewritten when moving between tools regardless. Beyond that, Go `text/template` syntax is a poor fit in the Tofu ecosystem, and implementing it would require a companion attribute for template context, introducing a second variable system alongside `vars` and `sensitive_vars`.

### `values_files` as the attribute name

Mirror the cli-next attribute name. `values_files` implies the attribute accepts file paths; to keep the name accurate the provider would need to accept file paths, read files internally, and store content as state — which reintroduces the local-filesystem restriction and provider-side file I/O. `DeployOptions.Values` in the Zarf SDK is also not Helm- or file-specific; a name suggesting "files" misrepresents the underlying mechanism. `values` with mutual exclusivity enforcement is the cleaner resolution.
