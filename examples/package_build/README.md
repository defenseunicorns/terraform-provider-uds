# Local package build example

This example uses `uds_package.build` to build the included Podinfo package and
deploy it to Kubernetes. No separate package creation command is required. It
also demonstrates deployment values, a manual revision trigger, and optional
plan-time build validation.

Local builds are intended for small, environment-specific packages kept alongside
OpenTofu configuration, not as a general-purpose artifact build or release system.
The provider handles building and deploying; you track relevant input changes
through `build.revision`. Use a Zarf/CI release pipeline and `source` for tested,
retained, published, signed artifacts intended for reuse across deployments.

## Prerequisites

- OpenTofu installed.
- A disposable Kubernetes cluster accessible through your current kubeconfig.
- Network access to fetch the Zarf init package and Podinfo chart and image,
  with any required registry credentials available to the process running OpenTofu.
- Sufficient local temporary disk space for package builds.

This configuration manages both Zarf init and Podinfo. Do not run it against a
cluster where another configuration manages either deployment.

## Deploy

Run these commands from this directory. The default architecture is `arm64`.
For an `amd64` cluster, set `TF_VAR_architecture=amd64` before running the commands.

```sh
tofu init
tofu plan
tofu apply
```

The provider verifies the signed Zarf init package, then builds `./podinfo` and
deploys it into the `podinfo` namespace. Locally built packages do not use the
`source` or `signature_verification` attributes.

## Rebuild after editing the package

The `revision` input is an explicit deployment trigger, not the package version.
After changing files under `podinfo/`, change the revision to request a fresh
build and deployment:

```sh
export TF_VAR_revision=manual-2
tofu plan
tofu apply
```

Keep that revision in your environment or a `terraform.tfvars` file for subsequent
commands. File edits alone do not trigger deployment when the revision remains
unchanged. Changes to deployment values, such as `replica_count`, also request a
fresh build and deployment.

This includes changes to architecture or optional component selection, even when
the package content has not changed. The revision is not a content digest or
Zarf package version; a package can keep a version such as `dev`.

## Validate builds during planning

By default, builds run only when creating or updating a deployment. To also build
and validate during every normal plan, including plans with no deployment changes:

```sh
export TF_VAR_build_on_plan=true
tofu plan
```

Plan builds can download dependencies and execute package-authored build actions.
Their artifacts are discarded; an apply that changes the deployment builds again.
Enabling plan builds does not make file edits an automatic deployment trigger.

## Build and deployment inputs

The provider writes package archives to temporary storage and removes its artifacts
after the operation. The `values` and `sensitive_values` attributes configure
deployment; they are not passed into the package build. Package-authored build
actions require their host-command dependencies to be installed on the runner.
Archives are not retained, published, signed, or reused across deployments.
Flavor selection and additional provider-configured create-time templating are
not supported; use the Zarf CLI or a CI workflow when those options are needed.

## Clean up

```sh
tofu destroy
```

This removes both Podinfo and the Zarf init deployment managed by this example.
