# 1. uds_package: helm values attribute and optional_components attribute (deprecate component block)

## Status

Proposed — 2026-05-27

> **Note:** The `values` attribute described in this ADR is **alpha** in lockstep with Zarf's
> `feature.Values` alpha gate. Behavior may change until Zarf promotes the feature to stable.

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

A new optional `values = list(string)` attribute accepts an ordered list of raw YAML strings. Each string is a fully rendered YAML document; the provider performs no template processing. Users supply content using Tofu's built-in functions:

```hcl
resource "uds_package" "example" {
  source = "oci://ghcr.io/example/package:1.0.0"

  # Values are pre-rendered YAML strings. Use file(), templatefile(),
  # yamlencode(), data source outputs, or inline HCL expressions.
  values = [
    file("${path.module}/base-values.yaml"),
    templatefile("${path.module}/env-values.yaml", {
      replica_count = var.replica_count
      image_tag     = var.image_tag
    }),
  ]
}
```

Behavior:

- **Order matters.** Index 0 is the base layer; each subsequent element overrides the previous. Objects deep-merge (Helm convention); scalar and list values replace. Implementation: each string is decoded directly into `value.Values` (`map[string]any`) using a YAML decoder, then `value.DeepMerge()` from the Zarf SDK is called in order to produce the final merged map, which is passed to `DeployOptions.Values`. cli-next uses `value.ParseFiles()` — appropriate there because its input is local file paths. The provider's input is already-evaluated string content, so bypassing the file layer and using `value.DeepMerge()` directly is the more correct fit and avoids temp file lifecycle concerns entirely.
- **Source is unrestricted.** Because the attribute accepts string content rather than file paths, values can originate from local files, remote object storage, secrets managers, other provider data sources, or any Tofu expression that evaluates to a string. The provider is decoupled from the storage mechanism entirely.
- **State tracking.** Stored as `list(string)`. Tofu's native diff compares content and order — referencing a different filename with identical content does not trigger re-deploy.
- **Content is case-sensitive.** YAML key casing is preserved as-is.
- **Mutual exclusivity.** If `values` is specified (including as an empty list) and any `component` block exists — with or without an `override` sub-block — schema validation emits an error:
  > `values cannot be specified together with component blocks`

  An explicitly empty `values = []` is a meaningful signal: the user has opted into the values paradigm, even if no values are currently supplied. This intentionally disallows `component` blocks. Callers who do not intend to use `values` must omit the attribute or set it to `null` — not `[]`. In particular, when wiring `values = var.package_values`, the variable must default to `null` rather than `[]` to avoid unintentionally triggering mutual exclusivity in environments that do not supply values.

  This is also a deliberate policy decision around migration. A name-only `component` block alongside `values` is technically valid — optional component selection and chart value customization are independent concerns. Allowing it would permit incremental migration (adopt `values` first, migrate to `optional_components` separately). The strict rule is chosen instead because it avoids a mixed-paradigm configuration that is harder to document and reason about during the deprecation window, and because it nudges users toward a clean cut-over to both new attributes together. Users who want to migrate incrementally should complete both changes in the same `tofu apply`.
- **No drift detection for chart values.** Chart value drift is not detectable. Zarf's deployed package Kubernetes secret records package metadata and installed components, not the merged chart values that were applied. If chart configuration diverges from stored `values` through out-of-band Helm operations or a failed partial deploy, the provider's Read operation will not detect it and `tofu plan` will show no diff. Recovery requires forcing redeployment via `tofu apply -replace=<resource address>`.
- **Zarf feature flag.** The provider enables `feature.Values` before calling `Deploy`, mirroring the cli-next approach.
- **Sensitive by default.** The `values` attribute is marked sensitive in the schema, which redacts its content from Tofu plan and apply output. This does not encrypt the values in Tofu state — state remains plaintext and should be protected at rest via standard state backend security controls. A parallel `sensitive_values` attribute (mirroring the `vars`/`sensitive_vars` split) is not viable here: merge order is significant, and splitting sensitive and non-sensitive content across two lists makes interleaved ordering impossible to express without additional metadata. Marking `values` uniformly sensitive is the simplest correct approach.
- **Alpha status.** Documented as alpha; semantics may change until Zarf promotes `feature.Values` to stable.

**Not interchangeable with cli-next.** cli-next uses Go `text/template` syntax (`{{ .vars.domain }}`) because it is a Go application with no access to Tofu's HCL evaluation engine. The provider keeps the interface idiomatic for OpenTofu by delegating rendering to HCL expressions and built-in functions (`${var.domain}`, `templatefile()`, `yamlencode()`, data sources, etc.). Users migrating between tools must rewrite template syntax — this is an intentional trade-off that avoids introducing a second variable system into the provider.

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

The `component` dynamic block is deprecated. Its schema gains a `DeprecationMessage`:

> `The component block is deprecated. Use optional_components to select optional components and values to supply chart values. The component block will be removed in a future version.`

The block remains functional during the deprecation window. Both cross-field rules above enforce that `component` blocks cannot coexist with either new attribute.

Any code paths that exclusively support the deprecated `component` block — schema definitions, validators, override flattening, deploy wiring — must be marked with a `TODO` comment referencing the deprecation:

```go
// TODO: remove when component block is removed
```

This keeps removal work discoverable and ensures deprecated code does not silently persist past the removal milestone.

### Migration path

The two paradigms (`component` blocks vs. `values` + `optional_components`) are mutually exclusive by schema validation. There is no conversion of one set of configuration to the other — the values YAML is new content written against the package's `chart.values` mappings, not a transformation of override path/value pairs. Switching paradigms removes the old configuration from state and replaces it with the new; switching always triggers a full package redeploy on the next `tofu apply`.

**Forward (component blocks → values + optional_components):**

1. Confirm the target package has shipped `chart.values[].sourcePath → targetPath` mappings in `zarf.yaml` (required prerequisite — package author work, independent of Tofu changes).
2. Remove all `component` blocks from the resource configuration.
3. Author `values` YAML content targeting the package's exposed `sourcePath` keys via Tofu functions.
4. Add `values = [...]` and (if applicable) `optional_components = [...]`.
5. Run `tofu plan` — plan shows a full redeploy. Run `tofu apply`.

**Backward (values → component blocks):**

Remove `values` and `optional_components`, re-add `component` blocks with `override` sub-blocks, run `tofu apply`. As with the forward direction, this requires re-authoring configuration from scratch — `{path, value}` override pairs are structurally different from the YAML keyed to `chart.values[].sourcePath` mappings used in `values`. There is no mechanical conversion in either direction; both paths require new configuration content. No conversion tooling is provided.

---

## Consequences

### Positive

- The provider aligns with Zarf's first-class deploy-time values mechanism; `ValuesOverridesMap` becomes a legacy path.
- Concerns are cleanly separated: `optional_components` selects components; `values` customizes chart configuration.
- Tofu-native rendering gives users the full HCL ecosystem (secrets managers, data sources, resource cross-references, `templatefile()`) with no additional provider surface.
- Per-package migration: consumers migrate resources one at a time as upstream packages are updated — no big-bang cutover required.
- Conceptually aligns with cli-next (`optional_components`, layered values), reducing cognitive overhead for users working across both tools.

### Negative

- **Verbosity vs. file-path ergonomics.** Accepting string content requires users to wrap each source in a Tofu function (`file()`, `templatefile()`, etc.), which is more verbose than a flat list of filenames would be. This is the cost of flexibility — the trade-off is intentional but worth acknowledging.
- **Migration effort.** Users must author new values YAML keyed to the package's `chart.values[].sourcePath` mappings. This content cannot be derived from existing override path/value pairs — it is new configuration written against the updated package schema.
- **Not interchangeable with cli-next.** Duplicate maintenance for organizations using both tools. Teams must maintain separate values file templates. This divergence is rooted in tooling: cli-next has no access to Tofu's HCL engine and implements Go `text/template`; the provider gets HCL interpolation natively and Go `text/template` would be a foreign addition.
- **No chart value drift detection.** Unlike `optional_components`, chart value drift cannot be detected by the Read operation — Zarf's package state does not record applied chart values. Out-of-band Helm changes are invisible to `tofu plan`; recovery requires `tofu apply -replace=<resource address>`.
- **Alpha dependency.** `values` inherits Zarf's alpha status. If Zarf changes the `feature.Values` API before GA, the provider must adapt.
- **Name ambiguity (transitional).** The top-level `values` attribute and the `override` block's nested `values` attribute share the same name during the deprecation window. Mutual exclusivity prevents coexistence; the conflict fully resolves when the `component` block is removed.

---

## Alternatives Considered

### `values_files = list(string)` of file paths

Accept a list of local file paths, with the provider reading the files at deploy time. This mirrors the cli-next `values_files` surface. The provider could store either the file paths or the resolved file contents in state — storing contents would correctly handle the rename case (same content, different filename, no re-deploy), so long as order is preserved. We choose not to store file names in state for exactly this reason. Even so, the file-path approach restricts sources to the local filesystem regardless of what is stored in state; accepting string content instead opens the attribute to secrets managers, remote storage, and any other Tofu-evaluable expression. cli-next reads file paths because it is a Go application with no access to Tofu's evaluation engine — the provider has no such constraint.

### `merge_values()` provider function with `values = string`

Expose a provider-registered `merge_values()` function accepting an ordered list of YAML strings and returning the deep-merged result. The `values` attribute would accept a single string; `merge_values()` is called only when multiple documents need layering:

```hcl
# Single document — no merge needed
values = file("${path.module}/values.yaml")

# Multiple documents — explicit merge, order visible at the call site
values = merge_values(
  file("${path.module}/base-values.yaml"),
  templatefile("${path.module}/env-values.yaml", { replica_count = var.replica_count }),
)
```

The function name is intuitive and makes the merge operation explicit at the call site rather than implicit in list ordering. Users who don't need to merge set `values` directly without calling it. The trade-off is adding a provider-registered function to the public API surface.

Not adopted for initial implementation. The `list(string)` approach is simpler to ship and the ergonomic difference is modest. The primary open question — how often users will have a single values document versus multiple — remains unknown; if single-document usage proves dominant in practice, `merge_values()` is a candidate for a follow-on release. See Open Questions.

### Deprecate overrides only; retain name-only `component` blocks for optional component selection

Deprecate the `override` sub-block and introduce `values`, but keep the `component` block (name only, no override) as the mechanism for selecting optional components, avoiding the introduction of `optional_components`. This works mechanically but the user experience is awkward: after deprecating overrides, the block's sole purpose would be optional component selection, yet users would write a block construct (`component { name = "metrics-server" }`) to express what is semantically list membership. A flat `optional_components = ["metrics-server"]` attribute communicates intent more directly and aligns with the cli-next surface.

### Cross-tool compatible templates (Go `text/template` in provider)

Implement Go `text/template` pre-processing in the provider so that values file templates work with both cli-next and this provider. Full compatibility is unachievable regardless: cli-next uses bundle-scoped global variables (`{{ .vars.domain }}`); the provider uses resource-scoped Tofu variables (`${var.domain}`). The scoping model is structurally different — values files referencing variables must be rewritten when moving between tools regardless. Beyond that, Go `text/template` syntax is a poor fit in the Tofu ecosystem, and implementing it would require a companion attribute for template context, introducing a second variable system alongside `vars` and `sensitive_vars`.

### `values_files` as the attribute name

Mirror the cli-next attribute name. `values_files` implies the attribute accepts file paths; to keep the name accurate the provider would need to accept file paths, read files internally, and store content as state — which reintroduces the local-filesystem restriction and provider-side file I/O. `DeployOptions.Values` in the Zarf SDK is also not Helm- or file-specific; a name suggesting "files" misrepresents the underlying mechanism. `values` with mutual exclusivity enforcement is the cleaner resolution.

---

## Open Questions

- **`merge_values()` provider function.** Deferred from initial implementation. If real-world usage shows that single-document `values` is the common case and multi-document merging is rare, a `merge_values()` provider function is a candidate for a follow-on release. It would make layering explicit at the call site and avoid the implicit "merge of 1" semantics of the `list(string)` approach.
