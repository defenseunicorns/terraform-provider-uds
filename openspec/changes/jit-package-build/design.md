# Design

## Context

See `proposal.md` for motivation and `specs/local-package-build/spec.md` for the behavior contract.

### Original implementation baseline

Before this feature, `internal/provider/resource_package.go` required `source`, defaulted signature verification to enabled, and gated package-dependent plan checks with provider-wide `validate_packages_on_plan`. Create loaded a layout before duplicate detection. Update first inspected the source for its canonical name, then loaded a deployment layout; the build path must avoid turning this into two builds. The state-only allowlist contained `timeouts`. Read and delete already relied on verified cluster state, not package sources.

The packager abstraction in `internal/packager/zarf_packager.go` wrapped Zarf APIs with provider logging. Initial development started on Zarf v0.89.0 and temporarily pinned commit `049d85579fc52159cb642a2633954cd3313d0f8d` (November 3, 2026), containing PR #5498, to obtain per-operation architecture options without global architecture state.

### Current dependency baseline

The branch is rebased onto main's released Zarf `v1.0.1` baseline. That release contains the required architecture commit; the temporary pseudo-version has been removed. `packager.Create(ctx, packagePath, output, opts)` returns the archive path and accepts `CreateOptions.Architecture`. Preserve main's version-neutral definition accessors, alpha/beta input validation, values during deployment and removal, compressed deployment-record handling, and lifecycle-event status mapping. Beta packages reject `vars` and `sensitive_vars`; these remain supported deployment inputs only for alpha packages.

Plan-time values-schema checks support partial unknown values in `internal/provider/package_value_validation.go`. Zarf enforces resolved values at deploy; preserve that division.

## Goals / Non-Goals

**Goals:**
- Add a second layout-acquisition path while sharing deployment and identity safeguards.
- Preserve configuration/plan/state semantics, including null and unknown build inputs and historical source-only state.
- Bound temporary artifact ownership to one lifecycle call and pass architecture explicitly per operation.

**Non-Goals:**
- New build-specific variable, flavor, signing, or publication interfaces. Flavor selection and other provider-configured create-time templating are deferred to future iterations. Existing variables remain deployment inputs only for supported package versions; beta packages reject them. Values remain deployment/removal inputs. Do not silently reinterpret any resource input as a build-time variable or templating setting.
- Persistent artifact state, plan-to-apply artifact reuse, package-content fingerprints, unrelated dependency upgrades, or source-directory sandboxing. The development dependency pin and its completed replacement with released Zarf v1.0.1 are in scope.
- Installing dependencies for package-authored host commands; those belong to the runner environment.
- Broad restructuring of existing resource CRUD, deployment orchestration, or source-mode code. Separate the new build concerns without turning this feature into a provider-wide refactor.

## Decisions

### 1. Use an optional nested attribute and explicit configuration validation

Add `Build types.Object` and a build model with required string `source_dir`, required string `revision`, and optional/computed boolean `build_on_plan` defaulting to false. Make `source` optional and validate exactly one input. Preserve the configured directory string in state; resolve an absolute path only for execution. Check that it is a readable package directory when a build is actually required, not during refresh or delete.

Validate source/build and build/signature exclusivity against raw configuration, before plan defaults. Null means omitted; an unknown configured object is not an empty object. Reject known conflicting configuration immediately and defer checks that truly require unknown values. Explicit non-null signature objects are invalid in build mode, including empty objects and `verify = false`.

Do not make `source` computed or overwrite it with an archive path. Keep schema additions compatible with source-only prior state, filling omitted build as null under the expanded schema; verify this through upgrade/refresh coverage rather than introducing a schema-version bump without need.

Alternative: a separate build resource/data source would expose artifact ownership and retention concerns outside the existing resource and would not match the confirmed interface.

### 2. Bypass implicit signature enforcement only for build mode

Keep source-mode defaults and verification unchanged. Effective verification must return disabled for known build mode, independently of the default signature object inserted into the plan. Retain defaulted schema values consistently in plan/state rather than mutating a known planned object during apply. Explicit signature configuration is still rejected by configuration validation.

Alternative: requiring users to write `verify = false` contradicts the required mutually exclusive interface. Changing the default globally would weaken source-mode verification.

### 3. Acquire one owned package layout per apply operation

Introduce a shared package-input acquisition boundary that returns a layout and an operation-scoped cleanup function. Source mode uses the existing loading behavior. Build mode creates a unique temporary output directory, calls a logged/noninteractive Zarf build wrapper, loads the returned archive, and owns both the loaded layout and output directory. Use the returned path rather than reconstructing a filename from package version or architecture.

Keep the new build/acquisition path separate from Framework lifecycle handling:

- The resource owns configuration/plan/state decoding, null/unknown handling, rebuild and plan-validation decisions, lifecycle sequencing, timeout selection, identity/mutation safeguards, and conversion of errors into Framework diagnostics.
- Build/acquisition helpers accept ordinary Go execution inputs and a context with the selected operation budget. They own temporary storage, build/load orchestration, and cleanup ownership, returning results and errors without receiving Framework models, requests, responses, state, or diagnostic containers. They do not decide whether a revision requires deployment or write resource state.
- The existing Zarf adapter owns Zarf API options/calls, logger integration, and error wrapping. Reuse it rather than introducing a second integration stack.

Plan and apply reuse the same build/acquisition implementation but retain their separate lifecycle policies. Translate resolved Framework inputs at the resource boundary. Keep new helpers in focused files using existing package conventions; no new service hierarchy or general-purpose orchestration framework is required. Existing source loading and deployment helpers may remain Framework-aware. Extract or adjust them only where required to integrate the new path safely, share applicable validation, or avoid duplicate builds; broader cleanup belongs to a separate change.

Register cleanup immediately after temporary storage is allocated and on partial failures. Cleanup must attempt all owned removals even if one fails; report cleanup failures safely without hiding the primary operation failure. Propagate operation context into build and load calls. Create/update timeout budgets include build, load, preflight, and deploy; plan builds receive the supplied planning context. These contexts are not hard limits on subprocess lifetime: upstream action cancellation can delay return and cleanup while child processes hold output pipes open. Source actions retain their normal directory-relative semantics; avoid process-wide working-directory changes.

Zarf `onCreate` actions execute host commands using the runner's available tools and dependencies. The provider does not install missing dependencies or retry by bootstrapping the runner. A missing required command is a build failure, using the same diagnostic, cleanup, and no-deployment path as other build errors.

In update, verify the recorded remote identity first, then acquire once and verify the actual layout's canonical name before component removal or deployment. Reuse that exact layout for both phases. Replace the duplicate inspection path for build mode without weakening the existing mutable-source mutation-boundary check. Create still rejects existing name/namespace deployments. Build failure never reaches mutation.

Alternative: building inside each existing source loader would duplicate update builds and could inspect different bytes from those deployed. Retaining the archive in state or a cross-operation cache would create an unwanted artifact lifecycle.

### 4. Use the released architecture API and pass architecture per operation

Use main's released Zarf `v1.0.1` dependency baseline. The initial immutable development pin to commit `049d85579fc52159cb642a2633954cd3313d0f8d` provided the architecture API while awaiting a release; it is no longer an installation or completion step. Release ancestry, intervening compatibility changes, and renewed verification are recorded below.

Pass the effective resource architecture, including the provider default, into `CreateOptions.Architecture` and retain explicit architecture for applicable source-loading/inspection calls. The explicit build option overrides package metadata and host architecture. Do not rely on inherited CLI configuration: empty library options no longer inherit it. Local archives retain their embedded architecture under normal Zarf loading semantics.

No provider-owned architecture override or process-wide architecture gate is needed. Do not reference removed `config.CLIArch`/`config.GetArch`, mutate manifests, or change process working directory to select architecture. Verify actual built metadata and overlapping amd64/arm64 builds alongside source loads, including race-enabled coverage and cancellation propagation.

The required transition to a tagged release containing #5498 is complete: upstream ancestry was verified and the branch now uses v1.0.1, with compatibility, build, and acceptance verification passing. This dependency transition does not change the public build interface or revision semantics. Never use a floating main reference or restore the development-only pseudo-version as the completed feature's dependency.

Historical alternative: retaining v0.89.0 would have required a global-state workaround; waiting for a release would have blocked development. The immutable pin allowed development to proceed, but a released dependency remained mandatory for completion.

### 5. Separate plan-validation policy from deployment change detection

After configuration normalization, select plan checks by input mode:

| Mode | Policy | Package acquisition during normal plan |
| --- | --- | --- |
| Archive/OCI | Existing validate_packages_on_plan | Existing behavior |
| Build | build_on_plan false | None |
| Build | build_on_plan true, required inputs known | Fresh build on every plan |
| Build | Necessary inputs unknown | Defer build and dependent checks |

Explicitly execute opted-in build validation even when the current state-only shortcut would return early. A successful build must still occur when no schema or optional-component checks are needed: build success itself is the requested validation. Never build for a null/destroy plan.

Reuse the layout-based canonical-name, optional-component, partial values-schema, and beta-variable checks, extracting them from source-specific acquisition as needed. Use configured partial values and variables as existing plan validation does. Beta packages reject configured legacy variables during opted-in planning and before apply-time component removal or deployment; alpha packages retain existing supported variable behavior. Do not pass these deployment inputs into Create options. Build errors attach to `build.source_dir`; source errors remain attached to `source`. Inspecting an unchanged package must not write fresh build metadata into the plan or create computed-only drift. When a real deployment change exists, leave deployment-derived outputs unknown as required by Framework planning semantics.

Plan outputs are discarded. Apply builds again only if create or a deployment-affecting update is scheduled. Changes to files or mutable dependencies between plan and apply are therefore intentionally possible.

Refresh-only operations do not perform normal package planning or deployment: keep Read build-free even with build_on_plan=true. A no-op apply execution likewise performs no build or deployment. Distinguish the apply phase from the CLI command: `tofu apply` without a saved plan normally includes a planning phase, which still performs opted-in validation builds. Verify refresh-only apply and saved no-op plan application through the existing acceptance framework. Standalone refresh-only planning is manually verified with the Podinfo build example; automated coverage is intentionally omitted rather than extending the framework or introducing a custom CLI harness.

Alternative: letting the provider-wide switch veto an explicit build opt-in conflicts with the confirmed precedence. Publishing plan build digests into state would turn validation into automatic change detection.

### 6. Classify nested operational changes precisely

Extend `isStateOnlyUpdate` to allow exactly `build.build_on_plan`, alongside `timeouts`; do not allowlist the whole build object. Changes to `build.revision`, `build.source_dir`, build presence, source presence, architecture, or deployment inputs require deployment. An unknown revision remains a potential deployment change until resolved; it is not compared as an empty string.

State-only update must copy planned timeouts and build_on_plan while preserving deployment-derived state and the rest of the user-owned build object. Combined operational and deployment changes use the normal deployment path. Revision is stored exactly as planned, including a constant; a failed build must not commit the new revision as successfully deployed.

Mode transitions use the same verified-identity update path. Names and namespace identity cannot change via a transition; existing namespace replacement behavior remains unchanged. Transitioning to source restores source-mode verification requirements. Refresh preserves build settings; import remains ID-only and cannot infer directory or revision.

Preserve Read's existing confirmed-absence distinction: only a confirmed missing deployment removes the resource from refreshed state. If configuration remains, the next normal apply uses Create, rebuilding even when revision is unchanged and enforcing ordinary duplicate/identity preflight. Lookup failures or inconsistent/conflicting identities report errors and preserve state; they must not trigger replacement. A refresh-only operation can record absence but never recreates the deployment itself.

Alternative: treating every build-object difference as deployment would rebuild on policy-only changes; treating the whole object as operational would miss real rebuild triggers.

## Risks / Trade-offs

- [Builds execute package-authored actions and can fetch dependencies] -> Keep builds noninteractive, propagate cancellation, document the upstream delayed-return limitation and other side effects, and keep archive output outside source_dir. The runner must provide dependencies for onCreate host commands; the provider does not install them.
- [Opted-in plans require build-capable runners] -> Document registry credentials where dependencies require authentication, package-required network access, adequate temporary disk, and potentially significant time. These prerequisites apply at plan time, not just apply time.
- [Opted-in plans may be slow and rerun actions, even with no deployment change] -> Default build_on_plan to false and document every-plan behavior explicitly.
- [Plan and apply builds can differ] -> State clearly that revision is a user-declared trigger, not an artifact integrity guarantee; no plan artifact is retained.
- [Local builds lack the provenance guarantees of signed immutable artifacts] -> Do not imply local build success or revision tracking provides those guarantees; recommend signed OCI packages through production release pipelines when provenance matters.
- [Dependency transition can change existing behavior] -> Use released Zarf v1.0.1 with verified ancestry for #5498, preserve main's compatibility adjustments, and rerun source-mode and build verification. The unreleased development-pin risk has been retired; do not automatically follow main.
- [Architecture defaults can differ between CLI and Go callers] -> Pass effective architecture explicitly for builds and applicable loads, and test actual metadata and concurrent cross-resource isolation without a global override.
- [Builds can expose secret-bearing output] -> Preserve the existing Zarf logging/error-wrapping behavior for this change, as explicitly agreed during implementation. The current adapter forwards raw Zarf messages and scalar fields to provider logs and can append captured output to errors surfaced in diagnostics; it does not provide general secret redaction. Build actions can therefore expose command text and stdout/stderr, depending on action and logging settings. Do not add new direct logging of resource variable/value maps. Broader Zarf logging safety and secret-suppression guarantees are deferred to separate future work, not completion criteria for JIT builds.
- [Treating every change inside build the same could redeploy unnecessarily or miss a required redeploy] -> Changing only build.build_on_plan must not rebuild/redeploy at apply, while changing build.revision or build.source_dir must. If build_on_plan and revision change together, revision still requires deployment. Test these individual fields and combinations rather than classifying the entire build object as one kind of change.
- [Removing optional components remains a partial-update boundary] -> Preserve existing verified-layout and failure-state behavior; do not broaden this feature into a new recovery strategy.

## Migration Plan

### Dependency verification record

Resolved commit `049d85579fc52159cb642a2633954cd3313d0f8d` through Go module resolution to `v0.89.1-0.20261103205608-049d85579fc5`; the module origin hash matches the requested commit and its commit subject identifies PR #5498. The v0.89.0-to-pin review includes per-operation architecture selection, removal of the SBOM viewer and artifact server, dev-command changes, and required transitive dependency updates. The provider does not use the removed viewer/artifact-server APIs; existing load options already pass architecture explicitly. No provider compatibility edits were necessary. `uds run test-unit --no-progress` passed before JIT implementation, including source signatures, alpha/beta definitions and inputs, removal values, inspection layers, identity safety, and deployment mutation policy coverage.

Release transition (November 20, 2026): rebased onto main `f60f04b`, which already adopts Zarf `v1.0.1`. Verified in the upstream Git repository that `049d85579fc52159cb642a2633954cd3313d0f8d` is an ancestor of tag `v1.0.1`. The provider's go.mod/go.sum exactly match main's released dependency baseline, with no pseudo-version or local replacement. Main's compatibility changes include nested removal configuration, typed chart-value overrides, component-filter API changes, lifecycle-event status mapping, and compressed deployment-record handling. Those changes and their regression coverage are preserved. The build helper's narrow execution-input API compiles unchanged against the release; beta build acceptance and the runnable example use the beta init package and its publishing workflow identity. Unit tests, focused build/provider race tests, acceptance compilation, documentation generation, lint, and the full renewed cluster acceptance suite pass. Details are recorded in the verification section below.

The original sequence established compatibility with the immutable pin before implementing build behavior. The completed rebase and release transition preserve version-neutral definitions, existing source verification, alpha-variable support, beta-variable rejection, values during removal, granular inspection-layer selection, and deployment mutation-policy behavior on v1.0.1.

Existing source configurations need no edits. Introduce build mode through examples and generated docs; users migrating remove source and explicit signature configuration and provide a directory plus revision. Verify the expanded schema accepts historical source-only state without perpetual diffs.

To move back to source mode, remove build and supply an existing archive/OCI source and appropriate verification configuration defining the same package identity. This is an in-place redeployment. Before reverting to a provider version that does not support build state, migrate resources to source mode and verify compatibility; do not assume older providers can consume the new state shape.

## Example Interface

```hcl
locals {
  package_dir          = "${path.module}/package"
  package_architecture = "amd64"
  package_files        = sort(tolist(fileset(local.package_dir, "**")))
  package_revision = sha256(jsonencode({
    files = {
      for file in local.package_files :
      file => filesha256("${local.package_dir}/${file}")
    }
    architecture = local.package_architecture
  }))
}

resource "uds_package" "example" {
  build = {
    source_dir    = local.package_dir
    revision      = local.package_revision
    build_on_plan = true # Optional; defaults to false.
  }
  architecture = local.package_architecture
  # Omit source and signature_verification in build mode.
}
```

The example hash is user-authored and covers only the listed files and architecture. Users can instead select a package version, dependency identifier, CI run identifier, manually incremented variable, or `timestamp()` for every-apply deployment; the provider does not infer completeness of any revision expression.

## Verification Results and Accepted Limitations

- Acceptance verification (November 6, 2026): the initial setup timed out under node `DiskPressure`; after user cleanup, `uds run test-acc --no-progress` successfully provisioned the authorized local `k3d-uds` cluster. Unsetting the inherited `TF_CLI_CONFIG_FILE` avoids its missing development config breaking the framework's CLI-version check. The final combined framework suite passed via `env -u TF_CLI_CONFIG_FILE uds run test-acc-reuse --no-progress` (441 seconds), including both JIT tests and all existing tests. The initial JIT fixture incorrectly used deployment actions only, so it did not require a Zarf cluster connection or record generation; its unmanaged marker also escaped normal removal. Replacing marker deployment/removal with a real, deploy-time-templated ConfigMap manifest resolved that issue without changing generation assertions. The only retained action observes onCreate build counts. No custom CLI harness, alternate cluster, or alternate credentials were used.
- Upstream cancellation: the development-pin regression took approximately 30 seconds to return from an `onCreate` shell action containing `sleep 30` despite a 100ms context deadline, and the returned error did not preserve `context.DeadlineExceeded`. The expected-behavior regression is retained with an explicit skip; subprocess termination and error preservation are deferred upstream by agreement, not completion blockers for this feature. Zarf v1.0.1 still waits for output readers before returning, so the release alone does not justify removing the skip. Build contexts and cleanup remain covered independently, and user-facing timeout documentation describes the limitation.
- Framework-only acceptance coverage: use the existing `terraform-plugin-testing` framework, not a custom CLI/provider-process harness. Saved-plan application and refresh-only apply have automated coverage. The user manually verified standalone refresh-only planning with the Podinfo build example; automated coverage of that specific operation is intentionally omitted by agreement. This manual verification predates the v1.0.1 rebase.
- Rebased verification (November 20): full unit tests, focused packager/provider build race tests, acceptance compilation, documentation generation, lint (zero issues, current generated docs), formatting, whitespace checks, and strict OpenSpec validation pass on the released v1.0.1 baseline. `env -u TF_CLI_CONFIG_FILE uds run test-acc --no-progress` provisioned a fresh authorized disposable k3d cluster and passed the full framework acceptance suite in 443.245 seconds, including both build tests and all existing source-mode tests. Final implementation review covered configuration/plan/state diagnostics, identity and mutation boundaries, temporary ownership, coverage, dependency scope, and docs/examples. User code review remains pending: the earlier Crit session was closed without approval. Nothing is archived or pushed by this verification.
