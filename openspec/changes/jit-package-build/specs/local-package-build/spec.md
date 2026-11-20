# Spec Delta

## Purpose

Enable deployment of local Zarf package directories through `uds_package`, with user-controlled rebuild triggers and temporary build artifacts integrated into the resource lifecycle.

## ADDED Requirements

### Requirement: Exactly one package input
The resource SHALL accept exactly one of an existing archive or OCI `source`, or a `build` object. Build configuration SHALL require a local `source_dir` and string `revision`, with `build_on_plan` defaulting to false.

#### Scenario: Valid build configuration
- **WHEN** build contains a source directory and revision and source is omitted
- **THEN** the configuration is accepted and omitted build_on_plan resolves to false

#### Scenario: Invalid input combinations
- **WHEN** both source and build are configured, neither is configured, or a required build field is omitted
- **THEN** configuration validation reports an error

#### Scenario: Deferred inputs
- **WHEN** a configured input is unknown during planning
- **THEN** validation defers checks that require its resolved value rather than treating it as an empty or absent input

### Requirement: User-owned revision tracking
The provider SHALL persist revision unchanged and use equality alone to detect revision changes. It SHALL NOT automatically hash source contents, synthesize a revision, or require revision to match the Zarf package version.

#### Scenario: Revision changes without a package version change
- **WHEN** revision changes while the package-defined name and version remain unchanged
- **THEN** apply builds and redeploys the package in place

#### Scenario: Content changes without a declared trigger
- **WHEN** directory contents change but revision and deployment-affecting configuration do not change
- **THEN** the provider does not schedule redeployment based on those content changes

#### Scenario: Constant revision
- **WHEN** a user supplies a constant revision
- **THEN** the provider accepts and stores it without interpreting its format

### Requirement: Fresh builds for deployment
In build mode the provider SHALL build a fresh package for create and every deployment-affecting update, regardless of whether revision changed. Each apply operation SHALL use one successful build for its package-dependent preflight and deployment.

#### Scenario: Deployment inputs change
- **WHEN** architecture, values, sensitive_values, variables supported by the package version, or component selection changes in build mode
- **THEN** apply builds again and deploys with the resolved configuration

#### Scenario: Failed build
- **WHEN** the build fails before deployment
- **THEN** the operation reports an error and performs no package deployment or component removal

### Requirement: Ephemeral build artifacts
The provider SHALL write built archives into operation-local temporary storage, not source_dir, and clean up provider-owned artifacts on success, error, or cancellation. It SHALL NOT retain a plan artifact for apply or persist a temporary archive path as source.

#### Scenario: Temporary build output
- **WHEN** a build succeeds and is deployed or inspected
- **THEN** its archive resides outside source_dir and provider-owned temporary storage is removed when the operation finishes

#### Scenario: Build cancellation or failure
- **WHEN** a build operation fails or is cancelled
- **THEN** cleanup is attempted for all provider-owned temporary build storage

#### Scenario: Package-authored actions
- **WHEN** a package build action intentionally writes a file into its source directory
- **THEN** normal Zarf action behavior remains allowed; the provider's output-location guarantee applies to the built archive

### Requirement: Runner-provided action dependencies
Package onCreate actions SHALL execute host commands using runner-provided dependencies. The provider SHALL NOT install missing host-command dependencies. A required command that is unavailable SHALL cause a build error without package deployment or component removal.

#### Scenario: Missing host command during build
- **WHEN** an onCreate action requires a host command unavailable on the runner during an opted-in plan build or apply build
- **THEN** the build reports an error, the provider does not install the command, temporary artifact cleanup is attempted, and no deployment or component removal occurs

### Requirement: Independent plan-build opt-in
When build_on_plan is true and required build inputs are known, the provider SHALL build and validate on every normal plan, including unchanged resources, independently of validate_packages_on_plan. When false it SHALL defer built-package-dependent checks to apply. Destroy plans SHALL NOT build.

#### Scenario: Default plan behavior
- **WHEN** build_on_plan is omitted or false
- **THEN** normal planning does not build the directory even if validate_packages_on_plan is true

#### Scenario: Explicit plan build overrides provider validation policy
- **WHEN** build_on_plan is true and validate_packages_on_plan is false
- **THEN** normal planning still builds and validates the package

#### Scenario: Unchanged resource
- **WHEN** build_on_plan is true and the resource has no deployment-affecting changes
- **THEN** the normal plan builds and validates without creating an update solely from the build result

#### Scenario: Unknown build input
- **WHEN** an input needed to build is unknown during plan
- **THEN** the build and dependent checks are deferred until apply

#### Scenario: Destroy plan
- **WHEN** the resource is planned for destruction
- **THEN** no package build occurs

#### Scenario: Refresh-only operation with plan builds enabled
- **WHEN** a refresh-only plan or apply runs with build_on_plan=true
- **THEN** the provider refreshes state without building, deploying, or removing package components

#### Scenario: No-op apply execution
- **WHEN** a plan schedules no resource creation or deployment-affecting update and its apply phase executes
- **THEN** apply execution does not build or deploy, even with build_on_plan=true
- **AND** an accompanying normal planning phase can still perform the opted-in validation build

### Requirement: Plan-time package validation
An opted-in plan build SHALL report build failures and run applicable package-defined name, optional-component, values-schema, and package-version-specific variable checks against the resulting package. Unknown deployment values SHALL defer only validation that depends on them. Plan validation SHALL NOT establish an apply-time artifact identity.

#### Scenario: Invalid package values
- **WHEN** a successful plan build contains a values schema and configured known values violate it
- **THEN** planning reports a values-schema error

#### Scenario: Deferred value dependency
- **WHEN** configured deployment values contain unknown leaves
- **THEN** known values are validated using existing partial-value semantics and dependent checks remain deferred

#### Scenario: Plan and apply builds
- **WHEN** a package was built successfully during plan and a deployment is required at apply
- **THEN** apply performs a fresh build rather than reusing the plan artifact

### Requirement: Operational-only updates
Changes solely to build_on_plan, timeouts, or both SHALL update stored configuration without an apply-time build, deployment, or component removal, while preserving deployment-derived state. Deployment-affecting changes combined with these settings SHALL still build and redeploy.

#### Scenario: Toggle plan validation
- **WHEN** only build_on_plan changes
- **THEN** apply stores the new setting without rebuilding or redeploying

#### Scenario: Timeout-only update
- **WHEN** only timeouts change in build mode
- **THEN** apply preserves the deployed package and computed outputs without building

#### Scenario: Operational and deployment changes together
- **WHEN** revision changes alongside build_on_plan or timeouts
- **THEN** apply builds and redeploys and stores all planned settings

### Requirement: Build mode excludes signature configuration
The provider SHALL reject explicitly configured signature_verification with build, including verify=false. It SHALL bypass implicit signature-verification defaults for build mode while preserving verification behavior for archive and OCI sources.

#### Scenario: Build without signature configuration
- **WHEN** build is configured and signature_verification is omitted
- **THEN** the provider accepts the configuration and deploys the successfully built package without requiring signature material

#### Scenario: Explicit signature configuration in build mode
- **WHEN** build and a non-null signature_verification object are explicitly configured, including verify=false
- **THEN** validation rejects the combination

### Requirement: Stable identity across updates and mode transitions
The provider SHALL allow in-place source/build transitions only when the recorded deployed identity and package-defined name remain unchanged. It SHALL verify identity and the new package name before any cluster mutation and retain existing duplicate-deployment protections for create.

#### Scenario: Transition into build mode
- **WHEN** source is removed, build is configured, explicit signature configuration is removed, and the built package name matches the verified deployed identity
- **THEN** apply redeploys the same managed package in place without destroying and recreating it

#### Scenario: Transition into source mode
- **WHEN** build is removed and a source satisfying existing verification requirements defines the same verified deployed identity
- **THEN** apply redeploys that same managed package in place

#### Scenario: Identity mismatch
- **WHEN** the new built or source package defines a different name or deployed identity cannot be verified
- **THEN** the update fails before component removal or deployment

#### Scenario: Duplicate create
- **WHEN** a build-mode create targets a package name and namespace already deployed
- **THEN** creation fails rather than adopting or overwriting the existing package

### Requirement: Resource-specific build architecture
Each build SHALL honor the effective resource architecture, including its provider default, without leaking that architecture into other resource operations.

#### Scenario: Different architectures in one provider process
- **WHEN** independent resources build packages for amd64 and arm64
- **THEN** each package uses its own effective architecture regardless of operation overlap

### Requirement: Refresh and deletion do not need build inputs
Read and delete SHALL use verified deployed cluster state without building or requiring source_dir to exist. Refresh SHALL preserve user-owned build configuration and revision rather than inferring them from remote package metadata.

#### Scenario: Source directory removed after deployment
- **WHEN** a build resource is refreshed or destroyed after its source directory disappears
- **THEN** those lifecycle operations require no local package build

#### Scenario: Build configuration survives refresh
- **WHEN** deployed metadata is refreshed
- **THEN** stored source_dir, revision, and build_on_plan remain unchanged

### Requirement: Confirmed remote absence permits recreation
When refresh confirms the recorded deployment is absent, the provider SHALL remove the resource from state without building. If configuration remains, a subsequent normal apply SHALL build and create it again subject to normal preflight checks. Lookup failures or unverifiable/conflicting identity SHALL NOT be treated as absence.

#### Scenario: Missing deployment is recreated
- **WHEN** refresh confirms the deployed package no longer exists and build configuration remains unchanged
- **THEN** the resource is removed from refreshed state without a build
- **AND** a subsequent normal apply builds and deploys it through the create path, even with an unchanged revision

#### Scenario: Lookup failure or identity conflict
- **WHEN** refresh cannot verify absence because lookup fails or returned package identity conflicts with recorded state
- **THEN** the provider reports an error, preserves the resource in state, and does not build or deploy a replacement

#### Scenario: Refresh-only confirms absence
- **WHEN** a refresh-only operation confirms the deployed package is absent
- **THEN** the refreshed state records absence without rebuilding or redeploying; recreation requires a subsequent normal apply

### Requirement: Existing package inputs remain compatible
Adding build mode SHALL preserve existing archive and OCI deployment, signature-verification, provider plan-validation, import, identity-safety, and timeout behavior. Built packages SHALL use version-neutral definitions and preserve existing package-version-specific input validation. Resource variables SHALL NOT be reinterpreted as build-time inputs. Values and sensitive_values SHALL retain their existing deployment and removal semantics.

#### Scenario: Existing source configuration
- **WHEN** a resource uses a local archive or OCI source without build
- **THEN** its existing validation, deployment, refresh, and deletion behavior remains supported

#### Scenario: Beta package rejects legacy variables
- **WHEN** a built v1beta1 package is configured with vars or sensitive_vars
- **THEN** opted-in plan validation reports the unsupported inputs
- **AND** apply rejects them before component removal or deployment, including when plan builds are disabled

#### Scenario: Alpha package retains supported variables
- **WHEN** a built v1alpha1 package uses supported vars or sensitive_vars
- **THEN** the existing deployment-variable behavior is preserved without passing those resource inputs as build-time variables

#### Scenario: Values remain available during removal
- **WHEN** components are removed during a build-mode update or the resource is deleted
- **THEN** values and sensitive_values are merged and supplied to removal using existing semantics
- **AND** deletion uses the verified deployed definition without rebuilding or requiring source_dir
