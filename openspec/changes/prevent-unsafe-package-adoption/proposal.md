## Why

The `uds_package` resource can import a Zarf deployment whose name was previously overridden by UDS CLI and later attempt to update it using the source package's different canonical name. The canonical redeployment would create a second Zarf package Secret while overlapping some resources owned by the aliased deployment and potentially duplicating or orphaning others, depending on how the package is constructed. These consequences are not exposed in the OpenTofu plan.

## What Changes

- **BREAKING** Reject source-derived management of an existing deployment when its verified deployed name differs from the configured source package's canonical `metadata.name`.
- Verify that package identity returned by Zarf agrees with the state ID, namespace override, and returned package metadata before persisting state or performing a remote mutation.
- Validate known canonical-name mismatches during planning when package validation on plan is enabled, while always repeating authoritative validation before a remote-mutating update.
- Preserve exact deletion of a freshly verified deployed identity without requiring source access, and preserve explicitly allowlisted state-only updates without cluster or source access.
- Keep the current import ID syntax and computed `name` attribute; do not add alias support, automatic canonicalization, takeover, or provider-managed migration.
- Document provisional standalone imports, why aliased deployments are unsupported, and a guarded operator-run migration workflow covering inventory and backups, deployment-input reconstruction, per-release classification, canonical render comparison, explicit stop conditions, eligible exact-object takeover, handoff verification, exact stale alias Secret cleanup, and canonical provider import. The provider does not automate this migration.

## Capabilities

### New Capabilities

- `package-adoption-safety`: Defines identity verification, canonical-name guards, safe lifecycle exceptions, diagnostics, and migration expectations for adopting existing Zarf packages into `uds_package` state.

### Modified Capabilities

None.

## Impact

- Affects `uds_package` import, read, plan modification, update, component removal, and delete behavior.
- Adds internal package identity and canonical-name validation around existing Zarf lookup and package-loading paths.
- Extends unit, framework, and acceptance coverage for canonical and non-canonical imports, stale or inconsistent identity, deferred validation, state-only updates, and exact deletion.
- Adds a generated Terraform Registry migration guide for deployments created with UDS CLI package-name overrides and manually validates its commands against disposable synthetic packages. Comprehensive topology-specific procedures, namespace moves, changed-object migrations, direct Helm storage surgery, and universal rollback guarantees remain outside scope.
- Adds no provider schema fields, external services, or new runtime dependencies.
