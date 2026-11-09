# Proposal

## Why

Authors of environment-specific glue packages currently need to build an archive with a script or `local-exec` before deploying it with `uds_package`. Native just-in-time builds remove that extra orchestration while leaving users in control of which changes warrant deployment.

## What Changes

- Add an optional `build` nested attribute with required `source_dir` and opaque `revision`, plus `build_on_plan` defaulting to `false`.
- Require exactly one of `source` or `build`; preserve existing local-archive and OCI behavior.
- Build into an operation-local temporary directory for create and every deployment-affecting update, then use the existing deployment pipeline. Do not retain artifacts or write archives into the source directory.
- Use revision equality, not package-content inspection, to request in-place rebuild/redeployment. Package versions need not change.
- Allow switching between source and build in place only when deployed identity and package-defined name remain unchanged.
- Independently honor `build_on_plan` on every normal plan, including unchanged resources, without using build results for change detection. Changing only that setting or timeouts is state-only at apply.
- Reject explicit `signature_verification` with build, including `verify = false`; skip implicit verification defaults in build mode.
- Keep refresh and deletion independent of local build inputs.

## Capabilities

### New Capabilities

- `local-package-build`: Local Zarf directory builds integrated into package planning, deployment, state, and lifecycle safety.

### Modified Capabilities

None. The current OpenSpec capability inventory is empty; existing archive/OCI contracts remain compatibility constraints.

## Impact

- Resource schema, model, validation, planning, update classification, and package-layout acquisition in `internal/provider/resource_package.go`.
- Zarf build integration and logging in `internal/packager/zarf_packager.go`, using per-operation architecture options from upstream PR #5498. Temporarily pin the Go pseudo-version for Zarf commit `049d85579fc52159cb642a2633954cd3313d0f8d` (main as of November 3, 2026) for development only; replace it with a tagged release containing that commit and rerun verification before this change is complete.
- Preserve the provider's Zarf v0.89.0 compatibility baseline: version-neutral package definitions, alpha/beta input validation, and values passed during removal. Reuse existing value-schema and beta-variable validation for plan-time built packages.
- Unit coverage, OpenTofu acceptance fixtures, examples, and generated resource/provider documentation.

## Non-goals

Automatic directory hashing, content-diff intelligence, provider-generated revisions, artifact retention or caching between plan and apply, flavor selection or other provider-configured create-time templating, signing, OCI publication, release-package workflows, installation of host-command dependencies, and sandboxing package-authored build actions. Flavor and create-time templating interfaces are deferred to future iterations. Normal Zarf dependency caching is not an artifact-retention contract.

## Operational Considerations

Opted-in JIT planning needs registry credentials where dependencies require authentication, network access as required by the package, temporary disk, and potentially significant time. Package `onCreate` actions run host commands; the runner must supply their dependencies, not the provider.

Local builds do not provide the provenance guarantees of signed immutable artifacts. Production release pipelines should continue to prefer signed OCI packages when those guarantees matter.
