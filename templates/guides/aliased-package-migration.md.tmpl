---
page_title: "Migrate an Aliased UDS Package"
description: |-
  Guarded operator-run procedure for migrating an eligible UDS CLI package-name alias to its canonical Zarf package identity.
---

# Migrate an Aliased UDS Package

This guide describes a basic, operator-run migration for a package that UDS CLI deployed under a package entry `name` that differs from the Zarf package's canonical `metadata.name`. The `uds_package` resource cannot safely manage that alias because a later source-derived operation would target the canonical identity instead.

This procedure is only for packages whose canonical deployment preserves the intended Kubernetes object names and namespaces. It covers ordinary Helm charts and Zarf-generated Helm charts for raw manifests, including packages that contain both. It does not guarantee that a package is eligible, automate the migration, provide a universal rollback, or cover namespace moves, changed-object migrations, direct Helm storage editing, or multiple aliases converging on one canonical identity.

!> Read the entire guide, arrange a maintenance window, and establish an application-specific recovery plan before starting. Test the procedure in a representative non-production environment. A backup preserves evidence and recovery inputs; it does not prove that restoring one state layer will roll back the other layers safely.

Keep the provisional alias in Terraform state while performing the inventory and eligibility assessment. The provider's alias guard can block other operations in the same root module, but removing state early while the resource remains configured is more dangerous: a subsequent apply can plan the canonical package as a new resource. Schedule an exclusive handoff window from state removal through canonical import.

## Understand the state layers

An aliased deployment spans four independent state layers:

| Layer | Aliased state | Canonical handoff |
| --- | --- | --- |
| OpenTofu or Terraform | Resource state points to the alias | State is removed, then the canonical identity is imported |
| Zarf | A deployed-package Secret records the alias | Canonical deployment creates a canonical Secret; the verified stale alias Secret is deleted last |
| Helm | Releases and histories record installed charts | A release is reused or exact objects are adopted by a new generated release |
| Kubernetes | Live workload objects exist at specific names and namespaces | Those exact objects remain healthy under canonical Helm ownership |

Changing one layer does not safely migrate the others. In particular, `--take-ownership` is not a package rename. It tells Helm to claim exact objects rendered by the canonical deployment. It cannot rename an object, move a namespaced object, merge two releases, reconcile incompatible selectors or immutable fields, or make package actions and Helm hooks safe to replay.

Use the commands below as guarded examples. Substitute values appropriate for the deployment and verify the available flags in the installed UDS CLI before proceeding:

```shell
uds zarf package deploy --help
uds zarf package inspect manifests --help
```

Use `terraform` in place of `tofu` when the resource is managed by Terraform.

## 1. Record the migration identity

Record these values in the migration plan:

```text
Resource address:             uds_package.example
Aliased deployed name:        <aliased-name>
Canonical metadata.name:      <canonical-name>
Namespace override:           <none-or-namespace>
Canonical package source:     <local-path-url-or-oci-reference>
Package architecture/version: <architecture-and-version>
```

Record the exact source digest when the source supports immutable references. Confirm that only one aliased deployment is being migrated to the target canonical name and namespace. Multiple aliases cannot converge on one canonical Zarf identity through this procedure.

## 2. Back up every state layer

Backups can contain credentials, package variables, values, and other sensitive data. Create a restricted directory, store it according to organizational policy, and remove it securely when its retention period ends.

```shell
umask 077
mkdir -p migration-backup
tofu state pull > migration-backup/tofu-state.json
tofu state show <resource-address> > migration-backup/tofu-resource.txt
uds zarf package list --output-format yaml > migration-backup/zarf-packages.yaml
helm list --all-namespaces > migration-backup/helm-releases.txt
kubectl get namespaces -o yaml > migration-backup/namespaces.yaml
```

Discover candidate Zarf deployed-package Secrets by label. Do not infer the Secret name and delete it.

```shell
kubectl --namespace zarf get secrets \
  --selector 'package-deploy-info=<aliased-name>' \
  -o name
```

For every candidate, decode the `data` field and compare `.name`, `.namespaceOverride`, and `.data.metadata.name` with the recorded alias identity:

```shell
kubectl --namespace zarf get <candidate-secret> \
  -o jsonpath='{.data.data}' \
  | base64 --decode \
  > migration-backup/candidate-deployed-package.json

jq '{name, namespaceOverride, metadataName: .data.metadata.name, deployedComponents}' \
  migration-backup/candidate-deployed-package.json
```

Identify exactly one Secret whose decoded identity matches the aliased name and namespace override. Record its full Kubernetes name as `<verified-alias-secret>`, then back it up:

```shell
kubectl --namespace zarf get <verified-alias-secret> -o yaml \
  > migration-backup/verified-alias-secret.yaml
```

The decoded `.deployedComponents[].installedCharts[]` entries identify each installed Helm release and namespace. Back up the values, rendered manifest, status, and history for every release, not only the first release:

```shell
helm --namespace <release-namespace> get values <release-name> --all \
  > migration-backup/<release-name>-values.yaml
helm --namespace <release-namespace> get manifest <release-name> \
  > migration-backup/<release-name>-manifest.yaml
helm --namespace <release-namespace> status <release-name> \
  > migration-backup/<release-name>-status.txt
helm --namespace <release-namespace> history <release-name> \
  > migration-backup/<release-name>-history.txt
```

Also capture the live YAML, health, storage state, and application-specific evidence needed to verify every namespaced and cluster-scoped object affected by the package.

## 3. Reconstruct the deployment inputs

Recover the inputs used for the aliased UDS bundle deployment:

- exact package source, version, architecture, and verification settings;
- selected optional components;
- values files and sensitive values;
- package variables;
- namespace override;
- connectivity, registry, and package-specific deployment flags;
- expected package actions and Helm hooks.

Do not continue if an input cannot be reconstructed. A canonical deployment with different component selection, values, variables, or namespace behavior can render a different workload even when the package source is unchanged. Package actions and Helm hooks can run again during deployment unless the package prevents replay.

## 4. Inspect and classify every release

Inspect the canonical definition and render it with the reconstructed component selection, values, and variables. Use the same package verification options required for deployment.

```shell
uds zarf package inspect definition <canonical-source> \
  > migration-backup/canonical-definition.yaml

uds zarf package inspect manifests <canonical-source> \
  --components <component-list> \
  --values <values-file> \
  --set-values <key=value> \
  --set-variables <KEY=value> \
  > migration-backup/canonical-manifests.yaml
```

Omit flags that were not used, and repeat supported flags as required. Review `uds zarf package inspect manifests --help` because available flags can change between CLI versions.

Classify every installed release or component independently. A package can require both paths.

| Classification | Required evidence | Basic path |
| --- | --- | --- |
| Ordinary Helm chart | Canonical release name and namespace equal the installed release, and intended object identities remain the same | Canonical deployment reuses or upgrades that release |
| Raw manifest | Canonical rendering produces the same intended object identities, but the generated release changes with the package name | Canonical deployment creates a release that takes ownership of those exact objects |
| Changed or uncertain | Any required evidence is missing or a stop condition applies | Stop; use an application-specific migration |

Zarf wraps each raw manifest in a generated Helm release whose name is package-name-sensitive:

```text
zarf-<sha1("raw-<package-name>-<component-name>-<manifest-name>")>
```

An alias-to-canonical migration therefore changes the generated release for raw manifests even when the manifest renders the same object names and namespaces.

Compare the canonical render with the stored Helm manifests and the live cluster. Account for expected Zarf and Helm metadata changes, but require the intended object kind, API identity, name, and namespace to remain compatible. Review selectors, immutable fields, owner references, persistent storage, cluster-scoped objects, custom resources, admission behavior, actions, and hooks.

### Stop conditions

Stop this basic procedure and perform an application-specific assessment if any of these conditions applies:

- a rendered object name or namespace changes;
- a selector or immutable field is incompatible with an in-place handoff;
- canonical rendering omits a live object without an explicit, reviewed disposition;
- a hook or package action cannot be replayed safely;
- shared or cluster-scoped resource ownership and impact are unresolved;
- the namespace override is unsupported or differs from the aliased deployment;
- more than one alias would map to the same canonical name and namespace;
- any release, component, deployment input, or application recovery step remains uncertain.

Do not treat `--take-ownership` as a way around a stop condition.

## 5. Remove only the provisional Terraform state

After backups and eligibility review are complete, leave the workload running and remove only the aliased resource from state:

```shell
tofu state rm <resource-address>
```

Verify that the resource address is absent from `tofu state list` and that the Zarf package, Helm releases, Kubernetes objects, and application remain present and healthy.

Do not run another Terraform plan or apply against this configuration until the canonical identity has been deployed externally and imported. The resource remains configured, so Terraform can otherwise plan to create it from the canonical source.

!> Do not use `tofu destroy`, `uds zarf package remove`, `zarf package remove`, or `helm uninstall` instead of `tofu state rm`. Those commands can remove the workload or shared resources.

## 6. Deploy the canonical package externally

Only after every release has an eligible, reviewed path, deploy the canonical package with the reconstructed inputs. Verify the exact flags supported by the installed CLI before running the command.

```shell
uds zarf package deploy <canonical-source> \
  --namespace <namespace-override> \
  --components <component-list> \
  --values <values-file> \
  --set-values <key=value> \
  --set-variables <KEY=value> \
  --take-ownership \
  --confirm
```

Omit `--namespace` when the alias had no namespace override. Omit other unused flags, and include all package verification, connectivity, registry, value, variable, and component options established during reconstruction. `--confirm` suppresses interactive review, so use it only after the package and inputs have been reviewed.

For an ordinary chart with unchanged release identity, Helm reuses or upgrades the existing release. For a raw manifest, the canonical deployment creates a differently named generated release and uses `--take-ownership` to claim the exact matching objects. Neither path changes the object identity.

If deployment reports an error or any unexpected create, replacement, deletion, hook, action, or ownership behavior, stop. Do not delete either Zarf Secret, remove either package, uninstall a release, edit Helm storage, or import the provider resource while the handoff is uncertain.

## 7. Verify the handoff

Do not clean up the alias until every applicable check passes:

- `uds zarf package list --output-format yaml` shows the canonical name and expected namespace override;
- the canonical deployed-package Secret decodes to the canonical `.name`, `.data.metadata.name`, namespace override, components, and installed releases;
- every expected Helm release has the intended status, manifest, values, history, and ownership annotations;
- every expected Kubernetes object retains the intended name, namespace, selectors, labels, ownership, storage, and health;
- application readiness, connectivity, persistence, and package-specific behavior pass their established checks;
- no unexpected duplicate or missing resource exists.

For a raw manifest, additionally verify that each live object now carries Helm ownership metadata for the canonical generated release and that the old generated release history still exists. Record that the old history rendered objects now owned by the canonical release and must not be uninstalled.

Capture the post-handoff state in a separate restricted backup before cleanup.

## 8. Delete only the verified stale alias Secret

Repeat discovery instead of relying only on the previously recorded Secret name:

```shell
kubectl --namespace zarf get secrets \
  --selector 'package-deploy-info=<aliased-name>' \
  -o name
```

Decode the candidate again and compare `.name`, `.namespaceOverride`, and `.data.metadata.name` with the recorded alias. Confirm that it is not the canonical Secret. Back up that exact current Secret immediately before deletion:

```shell
kubectl --namespace zarf get <verified-alias-secret> -o yaml \
  > migration-backup/verified-alias-secret-before-delete.yaml

kubectl --namespace zarf delete <verified-alias-secret>
```

Deleting this Secret relinquishes Zarf's normal package removal and recovery path for the alias. It does not uninstall the workload.

!> After ownership transfer, never run package removal for the alias or uninstall an old release that rendered adopted objects. Do not delete or edit Helm release storage as part of this procedure. Preserve stale raw-manifest Helm history and its operational warning.

Verify again that the canonical Zarf identity, Helm releases, Kubernetes objects, and application remain healthy after alias Secret deletion.

## 9. Import the canonical identity

Import the canonical package name. Include the original namespace override when one was used:

```shell
tofu import <resource-address> <canonical-name>
```

```shell
tofu import <resource-address> <namespace-override>:<canonical-name>
```

Then refresh and review the provider state and plan:

```shell
tofu state show <resource-address>
tofu plan
```

Verify that state contains the canonical name and expected namespace. A non-empty first plan can be expected because Zarf's deployed state does not reconstruct every provider configuration input. Review every proposed change and the first post-import deployment rather than assuming the migration produces a no-op plan.

## Partial failure handling

If any step fails or verification does not pass, stop and record the observed state of all four layers. Do not assume that restoring Terraform state, restoring a Zarf Secret, uninstalling a Helm release, or rerunning deployment will reverse changes made in the other layers.

- Before canonical deployment, keep the alias workload unmanaged while inputs and eligibility are investigated.
- During or after a failed canonical deployment, preserve both Zarf records and all Helm histories until the exact package-specific outcome is understood.
- After successful canonical deployment but before alias cleanup, do not invoke removal for the alias; it can reference releases or objects now used by the canonical deployment.
- After verified alias cleanup but before import, leave the canonical workload intact and import only after its identity and health are reconfirmed.

Use the restricted backups, cluster events, Helm histories, package logs, and application recovery plan to develop the next package-specific action. Escalate uncertain ownership, shared-resource, or data-integrity conditions instead of continuing the basic procedure.
