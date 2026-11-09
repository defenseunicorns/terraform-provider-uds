# Local package builds

## When to use local builds

Use local builds for small, environment-specific packages kept alongside your
OpenTofu configuration, such as a “glue” package of manifests and environment
configuration. The provider handles building and deploying without prerequisite
scripts or `local-exec`; you decide when package inputs have changed.

This is a deployment convenience, not a general-purpose artifact build or release
system. It trades artifact reuse, automatic change detection, and release
assurances for a small, explicit build-and-deploy workflow. Use normal Zarf/CI
workflows for artifact testing, retention, OCI publication, signing, and reuse
across deployments, then deploy the resulting artifacts with `source`.

For a complete runnable configuration, see the
[package build example](../../package_build/).

## Configure a build

Use `resource-build.tf` as an alternative to the source-mode examples. Supply a
local Zarf package directory and a required, opaque `revision` string. Specify
exactly one of `source` or `build`; omit `signature_verification` in build mode,
even `verify = false`. Source-mode signature verification remains unchanged.

The provider preserves the configured directory and revision. A constant or
manually incremented revision is valid: it is a user-declared deployment trigger,
not a package version or provider-generated content hash. `build_on_plan` is
optional and defaults to `false`.

Files, remote dependencies, and other build inputs can change without triggering
deployment. Reflect relevant changes in `build.revision`, either manually or with
an expression covering the inputs you want to track. The package's Zarf version
can remain unchanged, including a placeholder version such as `dev`.

## Build runner and output

Builds run Zarf `onCreate` actions with normal source-relative behavior; the
provider does not change its process working directory. The runner must supply
any host commands those actions require. Missing commands fail the build; the
provider does not install or bootstrap dependencies.

The runner must also provide registry credentials where needed, network access
to package dependencies, and sufficient temporary disk space. These requirements
apply to both deployment builds and opted-in plan builds.

Archives are written outside the source directory in unique operation-local
temporary directories and cleaned up after use, including failures and
cancellation. This does not sandbox package actions: actions can intentionally
write into the source directory or elsewhere. Temporary-output ownership applies
to provider-owned archives/layouts, not arbitrary action side effects or Zarf's
normal dependency cache.

The built archive is not an independently managed output: it is not retained,
published, signed, or made available for reuse across deployments.

Builds use the existing Zarf logging and error-wrapping behavior. Package authors
and Zarf control command output; command text and stdout/stderr can reach provider
logs or captured-output diagnostics according to action and logging settings.
There is no new secret-suppression guarantee. Resource variables and values,
including sensitive variants, are not supplied to builds.

Built alpha (`v1alpha1`) packages retain the existing deployment-variable
support through `vars` and `sensitive_vars`. Beta (`v1beta1`) packages reject
those inputs; use `values` and `sensitive_values` instead. Values are merged for
deployment and component/package removal. Deletion uses the verified deployed
definition and stored removal values, without rebuilding or accessing the local
source directory.

## Deployment and migration

Create and every deployment-affecting update build a fresh archive, including
changes to architecture, values, supported variables, component selection,
directory, or revision. Revision changes do not require a package-version bump.
Changes only to `build_on_plan`, timeouts, or both are state-only at apply and do
not build or deploy. Read and delete never build, even if the directory is gone.
Create/update timeout budgets include the build and subsequent deployment work.
The provider passes the operation deadline and cancellation to Zarf, but build
actions that spawn child processes can delay return and temporary-file cleanup
beyond the deadline if those children keep output pipes open. This is a known
[upstream cancellation limitation](https://github.com/zarf-dev/zarf/issues/1461),
not a guarantee of prompt subprocess termination.

Switch between source and build in place by changing the mutually exclusive
input and its signature configuration. The new package-defined name must match
the verified deployed identity; a mismatch or unverifiable remote identity blocks
mutation. Returning to source mode restores normal signature requirements.

Confirmed remote absence removes refreshed state without building. A subsequent
normal apply rebuilds/recreates from unchanged configuration, even with the same
revision, subject to duplicate protection. Lookup/identity errors instead preserve
state and report an error; they do not authorize recreation.

## Plan builds and revision expressions

By default, build-dependent validation waits until apply. Opting into plan builds
can run the build twice—once for validation and again for deployment.

`build_on_plan = true` runs a fresh validation build on every normal plan,
including unchanged resources and operational-only changes, independently of
`validate_packages_on_plan`. Unknown required build inputs defer the build.
Unknown deployment values defer only checks that need those leaves. Build success
and package metadata are not content-change detection and do not request deployment.

Plan artifacts are discarded. Apply builds independently only when deployment is
scheduled; there is no guarantee that mutable inputs are identical between plan
and apply. Refresh-only plan/apply never build. Applying a saved no-op plan does
not build; `tofu apply` without a saved plan normally includes a normal planning
phase, which still performs opted-in validation builds.

Opted-in plans require registry credentials where dependencies need authentication,
package-required network access, temporary disk, runner-provided action commands,
and potentially significant time. Builds may execute actions or fetch mutable
dependencies during planning. Flavor selection and provider-configured create-time
templating interfaces are deferred; resource variables/values remain deployment
inputs. Prefer signed OCI release pipelines when artifact provenance matters.

`resource-build-hash.tf` shows a user-authored hash of selected files and
architecture as an alternative to the constant/manual revision in
`resource-build.tf`. The provider does not infer whether a hash expression covers
all dependencies or build inputs. Exclude action-generated files from your own
hash expression if they should not trigger deployment.
