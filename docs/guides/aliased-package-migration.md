---
page_title: "Migrate a Legacy UDS Package Alias"
description: |-
  Migrate an eligible UDS CLI Legacy deployment alias to the name defined by its source package while preserving the deployed workload.
---

# Migrate a Package Previously Deployed by UDS CLI Using an Overridden Package Name

A Zarf package source defines its name in `metadata.name`. In UDS CLI **Legacy mode**, when a bundle gives that package entry a different `name` in `uds-bundle.yaml`, UDS CLI applies the deployment alias to the package definition before handing it to Zarf. The resulting Zarf package record, including its embedded `data.metadata.name`, reflects the deployment alias rather than the name from the source artifact. UDS CLI Next mode does not apply this Legacy name override.

For example, if the source package defines `metadata.name: podinfo`, this Legacy bundle entry deploys it as `podinfo-a`:

```yaml
kind: UDSBundle
metadata:
  name: example
  version: 0.1.0
packages:
  - name: podinfo-a
    repository: ghcr.io/example/packages/podinfo
    ref: 0.1.0
```

This creates two relevant identities:

- the **package-defined name** from `metadata.name` in the source package;
- the **deployment alias** applied by the UDS bundle.

The `uds_package` resource loads the package from source, does not allow modifications to the package's name, and therefore expects the package-defined name to be used. It cannot safely manage an existing aliased deployment that UDS CLI previously created because a subsequent provider deployment would create a second Zarf package record while changing resources already used by the alias. The provider blocks that operation rather than attempting to guess how the workload should be migrated.

This guide walks through how to perform that migration manually. The existing workload remains in place while you deploy the source package under its package-defined name, verify the result, retire only the stale alias Zarf package record, and import the new deployed identity into a `uds_package` resource managed by OpenTofu or Terraform.

## How package contents affect the migration

A Zarf package can install ordinary Helm charts, raw Kubernetes manifests, or both. Zarf ultimately manages both forms through Helm, but the package name affects their Helm release identities differently. This distinction determines how each part of the workload moves from the deployment alias to the package-defined name.

Zarf records every deployed package in a Kubernetes Secret. That record identifies the package and the Helm releases installed for each component.

### Ordinary Helm charts

An ordinary chart uses `components[].charts[].releaseName` from the source package's `zarf.yaml`, or its chart name when `releaseName` is not set. The Zarf package name is not normally part of that Helm release name.

If the aliased deployment and a new deployment under the package-defined name use the same chart release and namespace, the new deployment upgrades or reuses the existing Helm release. Until the alias Zarf package record is retired, two Zarf package records refer to the same Helm release. This is why running Zarf package removal against the alias after the handoff is unsafe.

### Raw manifests

Zarf installs raw manifests by wrapping each manifest entry in a generated Helm chart. Its generated release name is derived from the package, component, and manifest names:

```text
zarf-<sha1("raw-<package-name>-<component-name>-<manifest-name>")>
```

Changing from a deployment alias to the package-defined name therefore creates a different Helm release for a raw manifest, even when both releases render the same Kubernetes objects. The new release created by the package-defined deployment must take ownership of those exact objects. The old release history remains in the cluster and must not be uninstalled because it still describes objects now owned by the new release.

For raw-manifest takeover, `uds zarf package deploy --take-ownership` passes the ownership option to Helm so the new release can claim exact objects that already exist. It does not rename objects, move namespaced objects, merge releases, reconcile incompatible selectors or immutable fields, or make package actions and Helm hooks safe to replay. Legacy `uds deploy` does not offer this takeover option; `--force-conflicts` controls a different kind of conflict and is not a substitute.

## State involved in the migration

The handoff changes four related but independent state layers:

| State layer | What it records |
| --- | --- |
| OpenTofu or Terraform | Which Zarf package identity the `uds_package` resource manages |
| Zarf | The deployed package name, namespace override, components, and installed Helm releases stored in a Kubernetes Secret |
| Helm | Release ownership, rendered manifests, values, status, and revision history |
| Kubernetes | The live workload objects and their Helm ownership metadata |

Changing one layer does not automatically update the others. The procedure backs up and verifies each layer before retiring the alias Zarf package record.

## Migration overview

!> Read the entire guide, arrange a maintenance window, and establish an application-specific recovery plan before starting. Before migrating a production deployment, test the complete procedure in a development or staging environment that closely matches production. Backups preserve evidence and recovery inputs, but they do not guarantee that restoring one state layer will safely roll back the others.

The migration has seven phases:

```text
Assess the package and reconstruct its deployment inputs
                            |
                            v
Back up OpenTofu, Zarf, Helm, and Kubernetes state
                            |
                            v
Remove provisional OpenTofu state, if present, and keep the resource configuration inactive
                            |
                            v
Use the eligible Legacy bundle or direct Zarf deployment path
                            |
                            v
Verify the package, releases, objects, and application
                            |
                            v
Delete only the stale alias Zarf record
                            |
                            v
Import the new deployed package identity
```

When possible, perform this migration before adding or importing the `uds_package` resource. If an attempted import already placed the alias in state, keep that provisional state while assessing the package. The provider's safety check can block other operations in the same root module. If you must unblock those operations before the handoff window, remove both the resource configuration and its provisional state; removing state while leaving the resource configured can cause a subsequent apply to deploy the package-defined identity prematurely.

## When this guide applies

Use this procedure only when deployment under the package-defined name preserves the existing Kubernetes object names and namespaces. Assess every installed release and component independently because a package can contain ordinary charts, raw manifests, or both.

An ordinary-chart-only package is a potential candidate for **Legacy bundle redeployment** when every Helm release name, release namespace, and Kubernetes object identity remains unchanged. This path can retain the bundle's chart overrides and resolved variables during the handoff.

A package with raw manifests (including a mixed chart-and-manifest package) is a potential candidate for **direct Zarf takeover** when each new generated release renders the same Kubernetes object identities and every required deployment input is reproducible using the source package and direct Zarf options. Those objects can then move to the new release with `uds zarf package deploy --take-ownership`.

Legacy UDS bundle `overrides` target specific charts and can set Helm values or chart namespaces. Direct Zarf `--values` and `--set-values` set **Zarf package values**, not those chart-specific Legacy overrides. If a package with raw manifests also needs Legacy chart overrides, chart namespace overrides, or other bundle-only behavior that cannot be reproduced directly, neither path in this guide preserves both the inputs and takeover. Do not rename its bundle entry and run `uds deploy`: it cannot take ownership of the old raw-manifest objects. This case requires a package-specific migration.

For **either path**, the final `uds_package` configuration must also reproduce the required deployment inputs on its first update. The provider supports Zarf package values and variables and currently offers deprecated `component.override` chart-value configuration, but it cannot reproduce every Legacy bundle setting (such as a per-chart namespace override). If the provider cannot preserve the intended workload on its next apply, do not import it through this basic procedure, even if the handoff deployment succeeds.

The preferred longer-term approach is for package maintainers to expose supported customization through Zarf package values so operators can provide the same inputs through the provider or direct Zarf deployment. Test a package version using those values separately and compare its rendering with the live workload before using it for this handoff. Do not combine an unverified change in package behavior with the alias migration. A future Legacy UDS CLI bundle-level takeover option could support remaining override-dependent cases during the handoff, but would not by itself make unsupported provider inputs manageable.

This basic procedure does not apply when:

- a rendered object name or namespace changes, which would create a different object instead of adopting the existing one;
- a selector or immutable field is incompatible with an in-place handoff;
- the new rendering omits an existing object and it is unclear whether that object must remain, be deleted, or be migrated separately;
- replaying a package action or Helm hook would repeat a one-time operation or otherwise produce an unsafe side effect;
- you cannot determine which deployment owns a shared or cluster-scoped resource, or whether takeover would affect another workload;
- the package does not support the existing namespace override, or the proposed override differs from the alias deployment;
- the selected deployment path or the final provider configuration cannot reproduce all required bundle variables, chart values or namespaces, imports, exports, or other deployment inputs;
- multiple aliases would map to the same package-defined name and namespace;
- you cannot reconstruct the deployed components or inputs well enough to predict the resulting releases and objects;
- you do not have a recovery plan for the application-specific state affected by the handoff.

These cases require a custom migration designed for the package and workload. Do not treat `--take-ownership` as a way around an eligibility problem.

## Before you begin

Record the identities involved in the migration:

```text
Resource address:             uds_package.example
Deployment alias:             <deployment-alias>
Package-defined name:         <package-defined-name>
Namespace override:           <none-or-namespace>
Package source:               <local-path-url-or-oci-reference>
Package architecture/version: <architecture-and-version>
```

Use an immutable source digest when the source supports one. Confirm that only one deployment alias is moving to the package-defined name and namespace.

Verify the flags supported by the installed UDS CLI. The examples use Legacy `uds create` and `uds deploy` or direct `uds zarf` commands, depending on the path; available options can change between versions.

```shell
uds create --help
uds deploy --help
uds zarf package deploy --help
uds zarf package inspect manifests --help
```

Use `terraform` in place of `tofu` when the resource is managed by Terraform.

## 1. Back up the deployment

Back up all four state layers and retain the original Legacy bundle artifact, `uds-bundle.yaml`, deployment configuration, and referenced values files before changing the deployment. These files can contain credentials, package variables, values, and other sensitive data. Set `<secure-backup-directory>` to restricted encrypted storage outside the configuration repository. The timestamp creates a new directory so a later attempt does not overwrite earlier evidence.

```shell
# Create a private, unique directory outside the configuration repository.
umask 077
BACKUP_DIR="<secure-backup-directory>/alias-migration-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -m 700 "$BACKUP_DIR"

# Capture the complete infrastructure state before changing resource state.
tofu state pull > "$BACKUP_DIR/tofu-state.json"

# Capture the deployed-package records, Helm release inventory, and namespaces.
uds zarf package list --output-format yaml > "$BACKUP_DIR/zarf-packages.yaml"
helm list --all-namespaces > "$BACKUP_DIR/helm-releases.txt"
kubectl get namespaces -o yaml > "$BACKUP_DIR/namespaces.yaml"
```

If an attempted import created provisional resource state, capture that resource separately:

```shell
# Capture the imported resource entry before removing provisional state.
tofu state show <resource-address> > "$BACKUP_DIR/tofu-resource.txt"
```

Locate the Zarf deployed-package Secret by its alias label. Discover the Secret rather than constructing its name:

```shell
kubectl --namespace zarf get secrets \
  --selector 'package-deploy-info=<deployment-alias>' \
  -o name
```

Decode each candidate and compare its deployed package name, namespace override, and embedded deployed metadata name with the alias you recorded. For an aliased UDS CLI deployment, both `.name` and `.data.metadata.name` contain the deployment alias:

```shell
kubectl --namespace zarf get <candidate-secret> \
  -o jsonpath='{.data.data}' \
  | base64 --decode \
  > "$BACKUP_DIR/candidate-deployed-package.json"

jq '{name, namespaceOverride, metadataName: .data.metadata.name, deployedComponents}' \
  "$BACKUP_DIR/candidate-deployed-package.json"
```

Identify exactly one matching Secret and record its full Kubernetes name as `<verified-alias-secret>`. Back up that Secret:

```shell
kubectl --namespace zarf get <verified-alias-secret> -o yaml \
  > "$BACKUP_DIR/verified-alias-secret.yaml"
```

The decoded `.deployedComponents[].installedCharts[]` entries identify the installed Helm releases and namespaces. Back up the values, rendered manifest, status, and history for every release:

```shell
helm --namespace <release-namespace> get values <release-name> --all \
  > "$BACKUP_DIR/<release-name>-values.yaml"
helm --namespace <release-namespace> get manifest <release-name> \
  > "$BACKUP_DIR/<release-name>-manifest.yaml"
helm --namespace <release-namespace> status <release-name> \
  > "$BACKUP_DIR/<release-name>-status.txt"
helm --namespace <release-namespace> history <release-name> \
  > "$BACKUP_DIR/<release-name>-history.txt"
```

Capture representative live YAML and application-specific evidence for the resources whose identity, ownership, storage, or health is important to the handoff. For example, save a critical namespaced object with the following command; omit `--namespace` when capturing a cluster-scoped object:

```shell
kubectl --namespace <object-namespace> get <kind> <object-name> -o yaml \
  > "$BACKUP_DIR/<kind>-<object-name>.yaml"
```

## 2. Reconstruct and assess the deployment

Recover the inputs used for the alias deployment:

- exact unmodified Zarf package source, version, architecture, and verification settings (a bundle artifact alone may not provide a directly deployable source reference);
- original Legacy bundle definition and artifact;
- selected optional components;
- package values and values files, including sensitive values;
- Legacy bundle per-chart `overrides` (static values, variable-backed values, values files, and chart namespaces);
- resolved shared and per-package variables from `uds-config.yaml`, `UDS_` environment variables, `--set`, and bundle imports/exports;
- package namespace override;
- connectivity, registry, and package-specific deployment flags;
- expected package actions and Helm hooks.

Do not continue if an input cannot be reconstructed or represented in the eventual `uds_package` configuration. Different components, values, variables, chart overrides, or namespace behavior can produce a different workload even when the package source is unchanged. Renaming a bundle entry also changes the name used by package-specific configuration, `--set` selectors, and imports or exports; update those references in a rebuilt bundle. A `--packages` deployment selects only that package; if it depends on exports from another package that are unavailable when deployed alone, stop and design a package-specific migration rather than redeploying the entire bundle by default. Package actions and Helm hooks can also run again during deployment.

Inspect the unmodified Zarf package definition. For the direct Zarf takeover path, render it using the reconstructed **package-level** values and variables:

```shell
uds zarf package inspect definition <package-source> \
  > "$BACKUP_DIR/package-definition.yaml"

uds zarf package inspect manifests <package-source> \
  --components <component-list> \
  --values <values-file> \
  --set-values <key=value> \
  --set-variables <KEY=value> \
  > "$BACKUP_DIR/package-manifests.yaml"
```

Omit flags that were not used and repeat supported flags where needed. The `inspect manifests` command does not apply Legacy bundle chart overrides; do not use its output alone to claim equivalence for the bundle path. Test the rebuilt bundle with its resolved overrides in a representative environment and compare the result against stored Helm manifests and the live workload. For either path, the intended object kind, API identity, name, and namespace must remain compatible; account for expected Zarf and Helm metadata changes.

Classify every installed release, then select one deployment path for the **whole package**:

| Path | Evidence required | Expected handoff |
| --- | --- | --- |
| Ordinary chart only | All release names, release namespaces, and intended object identities remain the same; provider configuration can reproduce the required inputs | Redeploy the rebuilt Legacy bundle entry; existing Helm releases are upgraded or reused |
| Raw manifest or mixed package | Generated raw-manifest releases change, intended object identities remain the same, and **all** inputs can be represented by direct Zarf options and provider configuration | Deploy the unmodified Zarf package directly; new raw-manifest releases take ownership of those exact objects |
| Needs custom migration | Required inputs cannot be reproduced through the selected path, or any eligibility condition fails | Stop this procedure and design a package-specific migration |

Review selectors, immutable fields, owner references, persistent storage, cluster-scoped objects, custom resources, admission behavior, package actions, and Helm hooks before approving the handoff.

## 3. Begin the handoff

Once the package has passed assessment, begin the exclusive handoff window. Leave the workload running and ensure that the `uds_package` resource is not active in either configuration or state.

If you have not attempted an import, do not add the resource configuration yet. Confirm that the intended address is absent from `tofu state list`, then continue to the deployment step.

If an attempted import created provisional alias state, temporarily remove the resource block or module instance from the active configuration. Then remove only its state entry:

```shell
tofu state rm <resource-address>
```

Do not run a plan or apply between removing the configuration and removing its state entry because the plan could propose destroying the package. After both are absent, confirm that the address is absent from `tofu state list`. Run `tofu plan` and verify that it does not propose creating, replacing, or destroying the package. Confirm that the Zarf package, Helm releases, Kubernetes objects, and application remain present and healthy.

Keep the resource configuration inactive until the import step. If the configuration is restored while state remains absent, OpenTofu can plan to deploy the package directly from the source. You can use the same paired configuration-and-state removal before the maintenance window when the provisional import would otherwise block unrelated operations in the root module.

!> Do not use `tofu destroy`, `uds zarf package remove`, `zarf package remove`, or `helm uninstall` to clear provisional state. Those commands can remove the workload or shared resources.

## 4. Deploy under the package-defined name

Choose **one** path for the whole package, based on the classification in Step 2. Both paths create an additional Zarf package record under the package-defined name. An ordinary chart reuses its existing Helm release; a raw manifest needs its new generated release to take ownership of matching live objects.

### Ordinary-chart-only package: redeploy through the Legacy bundle

Use this path only when the package has no raw-manifest releases. Prepare a new Legacy bundle definition with the affected entry's `name` changed from the deployment alias to the package-defined name, leaving its package source, component selection, overrides, and namespace behavior equivalent. Update package-keyed `uds-config.yaml` entries, `--set` selectors, imports, and exports to use the new name. Rebuild the bundle from the correct package artifact; merely passing the old bundle artifact to `uds deploy` cannot change the name embedded in it. When the bundle includes a local package, ensure the staging filename and package entry match the new name as required by the installed Legacy CLI.

Verify that this rebuilt bundle preserves the required inputs and does not unexpectedly redeploy other packages. Deploy only the renamed entry with the original configuration and CLI inputs:

```shell
uds create <rebuilt-bundle-directory> --output <output-directory>
uds deploy <rebuilt-bundle-artifact> --packages <package-defined-name>
```

Use the same architecture, signing, verification, `uds-config.yaml`, environment variables, and `--set` values established during assessment; adjust package-specific references to the new name. Review the interactive deployment summary before confirming. Legacy `uds deploy` retains bundle-level chart overrides, but has no `--take-ownership` option. Do not use `--force-conflicts` as a replacement for Helm release takeover.

### Package with raw manifests: deploy the unmodified Zarf package directly

Use this path only if **every** required input can be supplied without the Legacy bundle's chart-specific overrides, imports/exports, or other unavailable bundle processing. The unmodified source Zarf package must have `metadata.name` equal to the package-defined name. A bundle artifact is not a direct substitute for that package source. Pass the matching package-level values, variables, components, and namespace with ownership takeover enabled:

```shell
uds zarf package deploy <package-source> \
  --namespace <namespace-override> \
  --components <component-list> \
  --values <values-file> \
  --set-values <key=value> \
  --set-variables <KEY=value> \
  --take-ownership
```

Omit `--namespace` when the alias had no namespace override. Omit other unused flags, and include all verification, connectivity, registry, value, variable, and component options established during assessment. `--values` and `--set-values` supply Zarf package values, **not** arbitrary Legacy UDS per-chart overrides. Review the deployment summary and confirm interactively only after verifying the package and reconstructed inputs.

For either path, Helm should reuse or upgrade existing ordinary-chart releases. At this point, both Zarf package records refer to those releases. On the direct Zarf path, raw manifests should create the expected releases derived from the package-defined name and transfer the exact matching objects to them. Their old release histories remain but no longer own those objects.

If deployment reports an error or an unexpected create, replacement, deletion, hook, action, or ownership change, stop. Keep both Zarf records and all Helm histories while you investigate.

## 5. Verify the migrated deployment

Do not retire the alias Zarf package record until every applicable check passes:

- `uds zarf package list --output-format yaml` shows the package-defined name as the new deployed package name, with the expected namespace override;
- the new deployed-package Secret contains the package-defined name in both `.name` and `.data.metadata.name`, together with the expected namespace, components, and installed releases;
- every expected Helm release has the intended status, manifest, values, history, and ownership metadata;
- every expected Kubernetes object retains the intended name, namespace, selectors, labels, ownership, storage, and health;
- application readiness, connectivity, persistence, and package-specific behavior pass their established checks;
- no unexpected duplicate or missing resource exists.

For raw manifests, verify that live objects now identify the new generated Helm release. Confirm that the old release history still exists, and record that it describes objects now owned by the new release and must not be uninstalled.

For each object transferred to a new raw-manifest release, inspect the live YAML and verify these Helm ownership values against the new release:

```text
metadata.labels["app.kubernetes.io/managed-by"]: Helm
metadata.annotations["meta.helm.sh/release-name"]: <new-release-name>
metadata.annotations["meta.helm.sh/release-namespace"]: <new-release-namespace>
```

Capture the post-handoff state in a separate restricted backup before cleanup.

## 6. Retire the alias package record

After the new package identity and workload are fully verified, discover the alias Secret again:

```shell
kubectl --namespace zarf get secrets \
  --selector 'package-deploy-info=<deployment-alias>' \
  -o name
```

Decode the candidate and compare its deployed package name, namespace override, and embedded deployed metadata name with the recorded alias. Confirm that it is not the Secret for the new deployment. Back up the exact current Secret immediately before deleting it:

```shell
kubectl --namespace zarf get <verified-alias-secret> -o yaml \
  > "$BACKUP_DIR/verified-alias-secret-before-delete.yaml"

kubectl --namespace zarf delete <verified-alias-secret>
```

Deleting this Secret retires the stale Zarf record. It does not uninstall the workload, but it also removes Zarf's normal removal and recovery path for the alias.

!> Do not run `uds zarf package remove` or `zarf package remove` for the alias, and do not uninstall an old release that rendered adopted objects. Do not delete or edit Helm release storage. Preserve stale raw-manifest release history and document why it remains.

Verify once more that the new Zarf record, Helm releases, Kubernetes objects, and application remain healthy.

## 7. Import the package-defined deployment

Restore the assessed `uds_package` resource configuration at `<resource-address>`, including the exact package source, components, values, variables, namespace override, and any supported component chart-value overrides needed to preserve the deployed workload. The provider cannot infer Legacy bundle overrides from Zarf state. Do not apply the configuration before import, and do not import if the provider cannot reproduce the required inputs during a subsequent Terraform or OpenTofu apply after import.

Import the package-defined name that the new Zarf record now uses as its deployed package name. Run exactly one of the following commands.

When the alias deployment did not use a namespace override:

```shell
tofu import <resource-address> <package-defined-name>
```

Or, when the alias deployment used a namespace override:

```shell
tofu import <resource-address> <namespace-override>:<package-defined-name>
```

Review the resulting state and plan:

```shell
tofu state show <resource-address>
tofu plan
```

Confirm that state contains the package-defined name and expected namespace. The first plan can show an in-place update, marked with `~`, because Zarf's deployed state does not reconstruct every provider configuration input. An in-place update may redeploy the package, but it does not first uninstall the imported package. Review every proposed change and the first post-import deployment rather than assuming the migration produces an empty plan.

!> A replacement plan is different from an in-place update. If the plan says the resource must be replaced and shows `-/+`, the normal destroy-before-create lifecycle fully uninstalls the imported package and its workload before deploying a fresh package. Investigate why replacement is planned and decide whether that behavior is acceptable before applying. Confirm that the resource address, import identity, source package, namespace, and configuration all describe the verified deployment.

## If something does not match

Stop when a command fails or the observed package, release, object, or application state differs from the expected result. Record all four state layers before taking another action.

- Before deployment under the package-defined name, keep the alias workload unmanaged while you resolve missing inputs or eligibility questions.
- During or after a failed deployment, preserve both Zarf records and all Helm histories until you understand the package-specific outcome.
- After successful deployment but before alias cleanup, do not remove the alias package; it can still reference releases or objects now used by the new deployment.
- After verified alias cleanup but before import, leave the migrated workload intact and import only after reconfirming its identity and health.

Do not assume that restoring OpenTofu state, restoring a Zarf Secret, uninstalling a Helm release, or rerunning deployment will reverse changes in the other layers. Use the backups, cluster events, Helm histories, package logs, and application recovery plan to determine the next package-specific action.
