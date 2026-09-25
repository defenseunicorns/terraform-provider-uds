## Context

See `proposal.md` for the unsafe-adoption motivation and `specs/package-adoption-safety/spec.md` for the behavior contract.

`uds_package` currently passes import IDs directly into state, uses `Read` to look up a deployed package, and copies the returned name into computed state. A later `Update` derives package identity again by loading the configured source; `upsert` replaces the planned name with source `metadata.name`, while component removal also loads package data from source or cluster. This permits an imported UDS CLI alias to be read under one identity and mutated under another.

The resource already has useful lifecycle boundaries: `getDeployedPackage` for cluster observation, `loadPackageLayoutForInspection` for metadata-only source access, `ModifyPlan` for package-dependent checks, an early `isStateOnlyUpdate` branch, and shared update timeouts. Zarf's deployed package contains the observed name, namespace override, and `Data.Metadata.Name`; the last value matters because Zarf removal derives its lookup from package metadata.

Standalone import cannot compare against source because the framework supplies only the import ID. Normal refresh can verify remote identity, and configured planning or apply can perform canonical comparison later.

## Goals / Non-Goals

**Goals:**

- Establish one explicit identity model at read, plan, update, and delete boundaries.
- Fail closed before source-derived operations can mutate a different Zarf identity.
- Keep validation timing compatible with `validate_packages_on_plan`, unknown values, import, state-only updates, and destroy.
- Preserve existing timeout budgets, diagnostic flow, logging, and canonical package behavior.
- Publish a guarded external migration runbook that distinguishes state removal from workload deletion and supports eligible ordinary-chart and raw-manifest packages without adding provider-managed migration behavior.

**Non-Goals:**

- Refactor the full deploy pipeline or eliminate the second source load during update.
- Pin one immutable source digest or guarantee same-name package-content consistency across multiple update loads.
- Add a configurable package name, alias persistence, Zarf takeover, or a migration action.
- Have the provider infer whether two package identities share Helm or Kubernetes resources.
- Define general replacement semantics when a package artifact changes canonical `metadata.name`.
- Guarantee migration eligibility, rollback, or recovery for every package topology.
- Cover namespace moves, changed-object migrations, multiple aliases converging on one canonical identity, or direct Helm storage editing.

## Decisions

### Separate remote, state, and source identity validation

Introduce small internal helpers with distinct trust boundaries:

- Remote verification parses the state ID, retrieves that exact package, and verifies returned name, namespace override, and `Data.Metadata.Name`. Its result carries the verified deployed package and identity needed by callers.
- State verification parses known prior-state ID and checks that prior computed name and namespace describe the same identity without contacting the cluster.
- Canonical verification loads metadata from configured source and compares source `metadata.name` with an already verified deployed or prior-state name.

Typed identity or mismatch errors will preserve allowlisted context for consistent lifecycle diagnostics while allowing absence to remain distinct from corruption or connectivity failure. Framework-facing diagnostics will categorize dependency failures rather than automatically exposing wrapped package, registry, transport, or cluster error strings; tests will use sentinel secrets to prove non-disclosure.

This separation avoids treating an import ID as authoritative returned data, avoids cluster access from `ModifyPlan`, and lets Delete use remote verification without source verification. A single all-purpose helper was rejected because each lifecycle operation has different availability and mutation guarantees.

### Verify observation before writing refreshed state

`Read` will replace its raw lookup with remote identity verification. Confirmed absence retains current state-removal behavior. A present but inconsistent package returns a blocking integrity diagnostic before computed identity is written.

The resource ID remains the lookup key and is not rewritten from untrusted returned fields. Once verified, the returned package remains authoritative for computed package metadata, connect strings, and component drift.

Create's post-deployment refresh and failed-deployment recovery are also observation boundaries. They begin from the canonical identity derived from the configured source, retrieve the corresponding deployed-package Secret, and must verify its returned name, namespace override, and package metadata name before writing refreshed or recovered state. Recovery may intentionally return verified state alongside the original Create error when Zarf recorded a failed deployment, but it must not persist an inconsistent returned identity.

Canonical source comparison does not run in `Read`: ordinary refresh must remain possible for provisional imports and destroy, and source may not be configured or available. Comparing there would also make every refresh depend on registry or local package access.

### Reuse refreshed prior state for plan-time canonical comparison

`ModifyPlan` will read prior state for an existing resource and validate its internal identity before comparing it with source metadata when `validate_packages_on_plan` is enabled. It will use prior ID, name, and namespace rather than computed plan identity or a planned replacement namespace.

Canonical comparison will join the existing package-dependent plan-check path so package metadata is loaded once when multiple checks are active. The check runner will receive enough prior identity context to run the name comparison even when optional-component, value-schema, and signature checks are not otherwise needed. The result will expose identity/name failures separately so diagnostics can be specific.

Unknown source or incomplete identity defers this check. Disabling package validation skips source loading and comparison. No plan path performs another cluster lookup; a normal refreshed plan has already passed `Read`, while `-refresh=false` is an explicit stale-state trade-off.

Persisted Terraform state cannot contain unknown values. Prior-state validation therefore distinguishes complete known identity from null or otherwise incomplete imported state; unknown handling applies to configuration and plan values only.

Loading source independently just for name validation was rejected because it duplicates registry work already centralized in `runPackagePlanChecks`.

### Put the authoritative guard after the state-only branch and before all mutations

`Update` will retain the existing early `isStateOnlyUpdate` return. Every other update will, under the shared update timeout, freshly verify the existing package using prior-state ID and then compare it with metadata loaded from configured source before calculating or executing component removal or deployment. If component removal or deployment reloads source package data, the canonical name from each mutation-bearing package definition or layout will be compared with the same verified deployed identity immediately after loading and before that package can reach `Remove` or `Deploy`.

The verified identity will be passed into the update path, and the update path will reject a missing or inconsistent identity before `removeComponents` or `upsert`. It will also pass the verified deployed name to those source-derived mutation paths so every later load can repeat the canonical comparison before removal, assignment to computed identity, or deployment. This makes the mutation boundary explicit and prevents a later refactor from accidentally moving component removal ahead of validation. The initial implementation may retain separate metadata, removal, and deployment loads to avoid broadening ownership and cleanup of package layouts across the update pipeline.

Relying only on `ModifyPlan` was rejected because plan validation can be disabled, deferred by unknowns, bypassed with stale state, or separated in time from apply. Mutating the loaded source package name to preserve aliases was rejected because it adopts UDS CLI alias semantics and does not guarantee independent Helm or Kubernetes resources.

### Delete the freshly verified exact identity without source access

`Delete` will parse prior-state ID, freshly retrieve and verify the exact deployed package, then call Zarf Remove with that returned package data and verified namespace. Confirmed absence succeeds. Connectivity or identity-integrity failures block deletion.

On successful or already-absent Delete, the provider returns without writing response state or calling `RemoveResource`; the Plugin Framework removes the resource from Terraform state when Delete completes without an error. On failure, leaving response state untouched retains the resource for retry.

The current source-or-cluster package loading path will not be used for Delete because it can involve source, architecture, transport, and signature configuration that are irrelevant to exact removal and can reconstruct a different identity. Canonical-name comparison is intentionally omitted: an explicit destroy of a provisionally imported alias is valid when the user intends to remove that workload.

Automatically deleting only Zarf state or canonicalizing before removal was rejected because either action could orphan or remove shared resources without a safe ownership transfer.

### Keep import provisional and migration external

`ImportState` remains ID passthrough, preserving `name` and `namespace:name`. Standalone import may therefore write verified alias state after `Read`; the next configured plan or remote-mutating apply blocks if source is non-canonical.

Canonical import tests will verify that the following `Read` hydrates complete recoverable state and that the configured resource reaches a no-op plan. Alias import tests will verify provisional hydration followed by configured rejection, rather than treating passthrough ID assignment alone as successful adoption.

Documentation will instruct users preserving the workload after provisional import to remove both active resource configuration and its state entry with `tofu state rm`, explain that Legacy UDS aliases require migration before package-defined import, and warn against using destroy, Zarf remove, or Helm uninstall as substitutes for state removal.

The migration guide will treat Terraform state, Zarf package state, Helm release history, and live Kubernetes objects as independent layers. Operators must inventory and back up each relevant layer, reconstruct the original deployment inputs, and classify every installed release or component before selecting a migration path. A package-level label alone is insufficient because one package can contain ordinary Helm charts, generated charts for raw manifests, or both.

For an ordinary-chart-only package, Legacy UDS bundle redeployment is a basic path only when the package-defined deployment retains the same Helm release names and namespaces and renders the same intended Kubernetes object identities. It preserves bundle chart overrides during handoff, but the final provider configuration must also reproduce all required inputs. For a raw or mixed package, Zarf's generated Helm release name includes the package name, so package-defined deployment creates a different release; direct Zarf `--take-ownership` may transfer exact rendered objects to that release only when all deployment inputs can be reproduced without Legacy bundle-only behavior. It cannot rename objects, move namespaced resources, reconcile incompatible selectors or immutable fields, or make hooks and actions safe. Legacy `uds deploy` does not offer takeover, while direct Zarf package values are not a substitute for arbitrary Legacy per-chart overrides; these combinations need a custom migration. Package maintainers should prefer exposing customization through Zarf package values, with any conversion tested independently before alias migration.

The guide will gate canonical deployment on comparison of canonical rendering with the live deployment and stored Helm manifests. Operators must stop for changed object identities or namespaces, unsafe hooks or actions, unresolved shared or cluster-scoped resources, incompatible selectors or immutable fields, omitted resources without an explicit disposition, or multiple aliases converging on one canonical name and namespace.

The guarded sequence is: back up state, bundle configuration, and deployment metadata; reconstruct values, variables, selected components, namespace and chart overrides, and other package inputs; compare the proposed rendering with stored Helm manifests and the live workload; keep any provisional state until an exclusive handoff window; remove both the active resource configuration and provisional state (if present) without destroying the workload; perform the eligible whole-package bundle or direct Zarf deployment; verify package-defined Zarf, Helm, Kubernetes, and application state; delete only the exact verified stale alias Zarf Secret; then restore configuration, import the package-defined identity, and review the resulting plan. No automated apply may run during handoff: removing configuration while leaving state plans destruction, and restoring configuration before import can plan a new deployment. `tofu destroy`, alias package removal, and old-release uninstall are not substitutes for state removal or cleanup.

For raw-manifest takeover, stale old Helm release history remains after exact-object ownership transfer. The basic procedure preserves and documents that history rather than uninstalling it or editing Helm storage because either action can delete or corrupt objects now owned by the canonical release. Deleting the stale alias Zarf Secret relinquishes Zarf's normal removal and recovery path for the alias, so its exact identity must be decoded, compared, and backed up immediately before cleanup.

A high-level warning without an executable path was rejected because it leaves eligible users to improvise the most hazardous handoff steps. Provider automation was also rejected because package eligibility and application safety require operator judgment across all four state layers. The published procedure will instead be manually exercised on disposable synthetic packages that cover an ordinary chart and a raw manifest; this documentation validation does not require provider runtime tests or permanent migration-only fixtures.

### Preserve the public schema

`name` remains computed and no deployed-name or takeover attribute is added. This keeps canonical identity as the only supported managed model and avoids persisting a switch whose semantics would be hazardous on every later update. Because the state shape does not change, the schema version remains unchanged and no state upgrader is introduced.

## Risks / Trade-offs

- [Plan-time source access can fail before apply] -> Respect `validate_packages_on_plan`; when enabled, use the existing metadata-only load and provide a source-scoped diagnostic, while Delete and state-only updates remain independent.
- [Standalone import can leave provisional Terraform state] -> Keep import read-only, block later mutation, and document `tofu state rm` before external migration.
- [The alias guard can block unrelated changes while assessment is in progress] -> Keep the fail-closed state until migration is ready; if it must be removed early, remove both resource configuration and provisional state, then schedule an exclusive handoff window through package-defined import.
- [A package can change between refresh and apply] -> Repeat remote and canonical verification immediately before any remote-mutating update or delete.
- [A source package name can change between update loads] -> Revalidate the canonical name from each package instance used for removal or deployment and block that mutation when it differs from the verified deployed name.
- [Same-name package content can change between update loads] -> Keep immutable digest pinning and source snapshot consistency outside this adoption-safety change; consolidate or pin update loads in follow-on work.
- [Package metadata may be loaded multiple times during update] -> Accept temporary duplicate I/O to keep the guard small and auditable; all loads share the lifecycle timeout and can be consolidated in follow-on work.
- [Stricter validation breaks management of existing aliases] -> Treat this as an intentional safety break, provide actionable diagnostics and migration guidance, and keep exact Delete available.
- [External takeover can still damage shared or application-specific resources] -> Gate the basic procedure on exact rendered identity and package-specific safety checks, require backups and multi-layer verification, and direct ineligible or uncertain cases to stop for application-specific investigation.
- [A partial migration can leave the four state layers inconsistent] -> Require operators to stop and investigate the observed package-specific state; backups preserve evidence and recovery inputs but are not presented as a universal rollback.
- [Deleting the alias Secret removes Zarf's normal recovery and removal path] -> Require exact Secret discovery, decoded identity comparison, immediate backup, and successful handoff verification before deletion.
- [Stale raw-manifest Helm history can prompt a future unsafe uninstall] -> Preserve and explicitly document the stale history, including that it rendered objects now adopted by the canonical release.
- [Documented CLI flags or behavior can drift] -> Verify both Legacy `uds deploy` and direct `uds zarf` flags against the installed CLI; bundle `--force-conflicts` is not Helm takeover and bundle deployment does not expose `--take-ownership` in the pinned CLI.
- [String matching for Zarf not-found errors is fragile] -> Preserve existing absence detection initially and cover it with tests; improve upstream error typing separately if available.
- [Dependency errors can echo source references, paths, or credentials] -> Build diagnostics from allowlisted identity metadata and safe error categories, retain raw details only in channels proven safe, and use sentinel-secret tests.

## Migration Plan

1. Release the provider with lifecycle guards, tests, generated resource documentation, and the generated guarded migration guide in the same version.
2. Canonical resources and imports require no state migration and continue normal lifecycle behavior.
3. Existing or newly imported aliases fail configured plan when plan validation is enabled, or fail before mutation during apply when it is disabled or deferred.
4. Users who want to preserve an alias deployment assess every release and component against the guide's eligibility and stop conditions before changing state or workloads.
5. Eligible users back up all relevant state, reconstruct inputs, compare package-defined rendering, remove any provisional resource configuration and state entry together, and follow the eligible whole-package bundle or direct Zarf deployment path.
6. After verifying the canonical Zarf, Helm, Kubernetes, and application state, users remove only the exact stale alias Zarf Secret, preserve any stale raw-manifest Helm history, and import the canonical identity.
7. Ineligible, uncertain, or partially failed migrations remain unmanaged while operators perform package-specific investigation; backups are recovery inputs, not a guaranteed rollback procedure.
8. Users who intentionally want to remove the alias may use normal resource deletion after evaluating shared Helm and Kubernetes ownership.

Rollback of the provider release restores the prior unsafe behavior and is therefore not a safe migration strategy for aliases. If implementation rollout must be reverted, affected aliases should remain removed from Terraform state until the guard is restored or an external migration is completed. Migration rollback itself is package-specific: after a partial handoff, operators must inspect all four state layers rather than assume that restoring one backup or uninstalling one release safely reverses the operation.
